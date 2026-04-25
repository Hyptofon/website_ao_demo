package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS queries (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		query_hash   TEXT    NOT NULL,
		query_text   TEXT    NOT NULL DEFAULT '',
		language     TEXT    NOT NULL DEFAULT 'uk',
		response_ms  INTEGER NOT NULL DEFAULT 0,
		sources_cnt  INTEGER NOT NULL DEFAULT 0,
		feedback     INTEGER NOT NULL DEFAULT 0,
		is_blocked   INTEGER NOT NULL DEFAULT 0,
		created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

// TestBatchAnalyticsWriter_FlushesOnTicker verifies that records sent through
// Record() are eventually persisted after the flush interval elapses.
func TestBatchAnalyticsWriter_FlushesOnTicker(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	repo, err := sqlite.NewAnalyticsRepo(db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := sqlite.NewBatchAnalyticsWriter(ctx, repo, db)

	// Enqueue 5 records.
	for i := 0; i < 5; i++ {
		_ = writer.Record(ctx, domain.QueryRecord{
			QueryHash:  "testhash",
			Language:   domain.LangUk,
			ResponseMs: int64(i * 100),
		})
	}

	// Wait for the flush ticker (500ms) + margin.
	time.Sleep(800 * time.Millisecond)

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM queries`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 records after flush, got %d", count)
	}
}

// TestBatchAnalyticsWriter_FlushesOnShutdown verifies records are not lost
// when the context is cancelled (graceful shutdown path).
func TestBatchAnalyticsWriter_FlushesOnShutdown(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	repo, err := sqlite.NewAnalyticsRepo(db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	writer := sqlite.NewBatchAnalyticsWriter(ctx, repo, db)

	// Enqueue 3 records then immediately cancel the context.
	for i := 0; i < 3; i++ {
		_ = writer.Record(ctx, domain.QueryRecord{
			QueryHash: "shutdown_test",
			Language:  domain.LangEn,
		})
	}

	// Cancel → triggers drain + flush in flushLoop.
	cancel()

	// Give the goroutine time to exit.
	time.Sleep(200 * time.Millisecond)

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM queries WHERE query_hash = 'shutdown_test'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 records flushed on shutdown, got %d", count)
	}
}

// TestBatchAnalyticsWriter_ReadsDelegateToRepo verifies that read methods
// (Summary, etc.) still work correctly through the batch writer.
func TestBatchAnalyticsWriter_ReadsDelegateToRepo(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	repo, err := sqlite.NewAnalyticsRepo(db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := sqlite.NewBatchAnalyticsWriter(ctx, repo, db)

	// Summary on empty DB should return zeros, not an error.
	summary, err := writer.Summary(ctx, 30)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.TotalQueries != 0 {
		t.Errorf("expected 0 queries, got %d", summary.TotalQueries)
	}
}
