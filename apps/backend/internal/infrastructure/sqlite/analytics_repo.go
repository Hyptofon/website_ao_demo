package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"university-chatbot/backend/internal/domain"
)

const analyticsSchema = `
CREATE TABLE IF NOT EXISTS queries (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	query_hash  TEXT    NOT NULL,
	language    TEXT    CHECK(language IN ('uk', 'en')) DEFAULT 'uk',
	response_ms INTEGER NOT NULL DEFAULT 0,
	sources_cnt INTEGER DEFAULT 0,
	feedback    INTEGER DEFAULT 0,
	is_blocked  INTEGER DEFAULT 0,
	created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_created_at ON queries(created_at);
CREATE INDEX IF NOT EXISTS idx_query_hash  ON queries(query_hash);
CREATE INDEX IF NOT EXISTS idx_feedback    ON queries(feedback);
`

// AnalyticsRepo is the SQLite-backed implementation of domain.AnalyticsRepo.
type AnalyticsRepo struct {
	db *sql.DB
}

// NewAnalyticsRepo creates the analytics repo and runs migrations.
func NewAnalyticsRepo(db *sql.DB) (*AnalyticsRepo, error) {
	if _, err := db.Exec(analyticsSchema); err != nil {
		return nil, fmt.Errorf("sqlite: migrate analytics: %w", err)
	}

	// Add query_text if it doesn't exist
	_, _ = db.Exec(`ALTER TABLE queries ADD COLUMN query_text TEXT NOT NULL DEFAULT ""`)

	return &AnalyticsRepo{db: db}, nil
}

// Record persists a completed query event.
func (r *AnalyticsRepo) Record(ctx context.Context, rec domain.QueryRecord) error {
	lang := string(rec.Language)
	if lang == "" {
		lang = string(domain.LangUk)
	}
	blocked := 0
	if rec.IsBlocked {
		blocked = 1
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO queries (query_hash, query_text, language, response_ms, sources_cnt, feedback, is_blocked)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.QueryHash, rec.QueryText, lang, rec.ResponseMs, rec.SourcesCnt, int(rec.Feedback), blocked,
	)
	return err
}

// UpdateFeedback updates the feedback column for rows with the given hash.
func (r *AnalyticsRepo) UpdateFeedback(ctx context.Context, queryHash string, fb domain.Feedback) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE queries SET feedback = ? WHERE query_hash = ?`,
		int(fb), queryHash,
	)
	return err
}

// Summary returns aggregated statistics over the last `days` days.
func (r *AnalyticsRepo) Summary(ctx context.Context, days int) (*domain.AnalyticsSummary, error) {
	since := time.Now().AddDate(0, 0, -days).Format(time.RFC3339)

	row := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			SUM(CASE WHEN is_blocked = 1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN feedback =  1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN feedback = -1 THEN 1 ELSE 0 END),
			AVG(response_ms)
		FROM queries
		WHERE created_at >= ?
	`, since)

	s := &domain.AnalyticsSummary{}
	var avgMs sql.NullFloat64
	if err := row.Scan(&s.TotalQueries, &s.BlockedQueries, &s.PositiveFeedback, &s.NegativeFeedback, &avgMs); err != nil {
		return nil, fmt.Errorf("sqlite: summary: %w", err)
	}
	if avgMs.Valid {
		s.AvgResponseMs = avgMs.Float64
	}
	return s, nil
}

// TopQueries returns the most frequent query hashes in the last N days.
func (r *AnalyticsRepo) TopQueries(ctx context.Context, days, limit int) ([]domain.TopQuery, error) {
	if limit <= 0 {
		limit = 20
	}
	since := time.Now().AddDate(0, 0, -days).Format(time.RFC3339)

	rows, err := r.db.QueryContext(ctx, `
		SELECT query_text as display_name, COUNT(*) as cnt, language, MAX(created_at) as last_seen
		FROM queries
		WHERE created_at >= ? AND is_blocked = 0 AND query_text IS NOT NULL AND query_text != ''
		GROUP BY display_name
		ORDER BY cnt DESC
		LIMIT ?
	`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: top queries: %w", err)
	}
	defer rows.Close()

	var results []domain.TopQuery
	for rows.Next() {
		var q domain.TopQuery
		if err := rows.Scan(&q.QueryText, &q.Count, &q.Language, &q.LastSeen); err != nil {
			return nil, fmt.Errorf("sqlite: scan top query: %w", err)
		}
		results = append(results, q)
	}
	return results, rows.Err()
}

// DailyStats returns per-day aggregated analytics for the last N days.
func (r *AnalyticsRepo) DailyStats(ctx context.Context, days int) ([]domain.DailyStat, error) {
	since := time.Now().AddDate(0, 0, -days).Format(time.RFC3339)

	rows, err := r.db.QueryContext(ctx, `
		SELECT 
			DATE(created_at) as day,
			COUNT(*) as total,
			SUM(CASE WHEN is_blocked = 1 THEN 1 ELSE 0 END) as blocked,
			AVG(response_ms) as avg_ms,
			SUM(CASE WHEN feedback = 1 THEN 1 ELSE 0 END) as pos,
			SUM(CASE WHEN feedback = -1 THEN 1 ELSE 0 END) as neg
		FROM queries
		WHERE created_at >= ?
		GROUP BY DATE(created_at)
		ORDER BY day ASC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("sqlite: daily stats: %w", err)
	}
	defer rows.Close()

	var stats []domain.DailyStat
	for rows.Next() {
		var s domain.DailyStat
		var avgMs sql.NullFloat64
		if err := rows.Scan(&s.Date, &s.TotalQueries, &s.BlockedQueries, &avgMs, &s.PositiveFeedback, &s.NegativeFeedback); err != nil {
			return nil, fmt.Errorf("sqlite: scan daily stat: %w", err)
		}
		if avgMs.Valid {
			s.AvgResponseMs = avgMs.Float64
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// FeedbackStats returns aggregated feedback counts and positivity ratio.
func (r *AnalyticsRepo) FeedbackStats(ctx context.Context, days int) (*domain.FeedbackStat, error) {
	since := time.Now().AddDate(0, 0, -days).Format(time.RFC3339)

	row := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE feedback != 0) as total,
			SUM(CASE WHEN feedback = 1 THEN 1 ELSE 0 END) as pos,
			SUM(CASE WHEN feedback = -1 THEN 1 ELSE 0 END) as neg
		FROM queries
		WHERE created_at >= ?
	`, since)

	fs := &domain.FeedbackStat{}
	if err := row.Scan(&fs.Total, &fs.Positive, &fs.Negative); err != nil {
		// SQLite doesn't support FILTER, use fallback
		row2 := r.db.QueryRowContext(ctx, `
			SELECT
				SUM(CASE WHEN feedback != 0 THEN 1 ELSE 0 END) as total,
				SUM(CASE WHEN feedback = 1 THEN 1 ELSE 0 END) as pos,
				SUM(CASE WHEN feedback = -1 THEN 1 ELSE 0 END) as neg
			FROM queries
			WHERE created_at >= ?
		`, since)
		if err := row2.Scan(&fs.Total, &fs.Positive, &fs.Negative); err != nil {
			return nil, fmt.Errorf("sqlite: feedback stats: %w", err)
		}
	}

	if fs.Total > 0 {
		fs.Ratio = float64(fs.Positive) / float64(fs.Total)
	}
	return fs, nil
}

// RecentQueries returns individual query rows for admin inspection.
func (r *AnalyticsRepo) RecentQueries(ctx context.Context, days, limit int) ([]domain.QueryRow, error) {
	if limit <= 0 {
		limit = 50
	}
	since := time.Now().AddDate(0, 0, -days).Format(time.RFC3339)

	rows, err := r.db.QueryContext(ctx, `
		SELECT query_hash, COALESCE(NULLIF(query_text, ''), '[Текст не збережено]') as query_text, language, response_ms, sources_cnt, feedback, is_blocked, created_at
		FROM queries
		WHERE created_at >= ?
		ORDER BY created_at DESC
		LIMIT ?
	`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: recent queries: %w", err)
	}
	defer rows.Close()

	var results []domain.QueryRow
	for rows.Next() {
		var q domain.QueryRow
		if err := rows.Scan(&q.QueryHash, &q.QueryText, &q.Language, &q.ResponseMs, &q.SourcesCnt, &q.Feedback, &q.IsBlocked, &q.CreatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan query row: %w", err)
		}
		results = append(results, q)
	}
	return results, rows.Err()
}
