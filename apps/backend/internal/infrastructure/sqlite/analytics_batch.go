package sqlite

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"university-chatbot/backend/internal/domain"
)

// ─── Batch Analytics Writer ────────────────────────────────────────────────────
// Problem: Each chat request fires `go func() { analyticsRepo.Record(...) }()`
// resulting in up to 100 concurrent goroutines all hammering the same SQLite
// connection (SetMaxOpenConns=1). Under load this causes latency spikes as
// goroutines queue behind the single writer connection.
//
// Solution: A channel-based batch writer that:
//   1. Accepts records non-blocking via a buffered channel (decouples hot path)
//   2. Flushes them in a single INSERT ... VALUES (…),(…) batch on a ticker
//      (every 500ms) or when the buffer fills up (>= batchSize)
//   3. Stops cleanly when the context is cancelled (graceful shutdown)
//
// The writer implements domain.AnalyticsRepo so it is a drop-in replacement.

const (
	// analyticsBufferSize: how many pending records we buffer before the
	// channel blocks callers. At 100 req/s this covers 2 seconds of burst.
	analyticsBufferSize = 200

	// analyticsBatchSize: max records per INSERT batch.
	analyticsBatchSize = 50

	// analyticsFlushInterval: how often we flush even if batch is not full.
	analyticsFlushInterval = 500 * time.Millisecond
)

// BatchAnalyticsWriter wraps AnalyticsRepo and batches Record() calls.
// All read methods (Summary, TopQueries, etc.) delegate directly to the
// underlying repo — only writes are batched.
type BatchAnalyticsWriter struct {
	repo    *AnalyticsRepo
	ch      chan domain.QueryRecord
	db      *sql.DB
}

// NewBatchAnalyticsWriter creates a BatchAnalyticsWriter and starts its
// background flush goroutine. The goroutine stops when ctx is cancelled.
//
// Usage in main():
//
//	rawRepo, _ := sqlite.NewAnalyticsRepo(db)
//	batchWriter := sqlite.NewBatchAnalyticsWriter(ctx, rawRepo, db)
//	// use batchWriter wherever analyticsRepo is needed
func NewBatchAnalyticsWriter(ctx context.Context, repo *AnalyticsRepo, db *sql.DB) *BatchAnalyticsWriter {
	w := &BatchAnalyticsWriter{
		repo: repo,
		ch:   make(chan domain.QueryRecord, analyticsBufferSize),
		db:   db,
	}
	go w.flushLoop(ctx)
	return w
}

// Record enqueues a query record for async batch writing.
// Non-blocking: if the channel is full, the record is dropped with a warning
// (prefer losing one analytics point over blocking the chat response).
func (w *BatchAnalyticsWriter) Record(_ context.Context, rec domain.QueryRecord) error {
	select {
	case w.ch <- rec:
	default:
		slog.Warn("analytics batch channel full — dropping record",
			"query_hash", rec.QueryHash)
	}
	return nil
}

// ─── Read methods delegate to underlying repo ─────────────────────────────────

func (w *BatchAnalyticsWriter) UpdateFeedback(ctx context.Context, queryHash string, fb domain.Feedback) error {
	return w.repo.UpdateFeedback(ctx, queryHash, fb)
}

func (w *BatchAnalyticsWriter) Summary(ctx context.Context, days int) (*domain.AnalyticsSummary, error) {
	return w.repo.Summary(ctx, days)
}

func (w *BatchAnalyticsWriter) TopQueries(ctx context.Context, days, limit int) ([]domain.TopQuery, error) {
	return w.repo.TopQueries(ctx, days, limit)
}

func (w *BatchAnalyticsWriter) DailyStats(ctx context.Context, days int) ([]domain.DailyStat, error) {
	return w.repo.DailyStats(ctx, days)
}

func (w *BatchAnalyticsWriter) FeedbackStats(ctx context.Context, days int) (*domain.FeedbackStat, error) {
	return w.repo.FeedbackStats(ctx, days)
}

func (w *BatchAnalyticsWriter) RecentQueries(ctx context.Context, days, limit int) ([]domain.QueryRow, error) {
	return w.repo.RecentQueries(ctx, days, limit)
}

// ─── Background flush goroutine ───────────────────────────────────────────────

// flushLoop drains the channel and writes records in batches.
// Exits cleanly when ctx is cancelled (after flushing any remaining records).
func (w *BatchAnalyticsWriter) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(analyticsFlushInterval)
	defer ticker.Stop()

	buf := make([]domain.QueryRecord, 0, analyticsBatchSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := w.writeBatch(buf); err != nil {
			slog.Error("analytics batch write failed", "count", len(buf), "error", err)
		}
		buf = buf[:0] // reset slice, keep allocated memory
	}

	for {
		select {
		case rec := <-w.ch:
			buf = append(buf, rec)
			if len(buf) >= analyticsBatchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-ctx.Done():
			// Drain any remaining records in the channel before exiting.
		drain:
			for {
				select {
				case rec := <-w.ch:
					buf = append(buf, rec)
				default:
					break drain
				}
			}
			flush()
			slog.Info("analytics batch writer stopped", "flushed_on_shutdown", len(buf))
			return
		}
	}
}

// writeBatch inserts a slice of QueryRecords in a single transaction.
// Uses a multi-value INSERT for efficiency.
func (w *BatchAnalyticsWriter) writeBatch(records []domain.QueryRecord) error {
	if len(records) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO queries (query_hash, query_text, language, response_ms, sources_cnt, feedback, is_blocked)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, rec := range records {
		lang := string(rec.Language)
		if lang == "" {
			lang = string(domain.LangUk)
		}
		blocked := 0
		if rec.IsBlocked {
			blocked = 1
		}
		if _, err = stmt.ExecContext(ctx, rec.QueryHash, "", lang,
			rec.ResponseMs, rec.SourcesCnt, int(rec.Feedback), blocked); err != nil {
			return err
		}
	}

	return tx.Commit()
}
