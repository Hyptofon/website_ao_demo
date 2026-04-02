package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
	"university-chatbot/backend/internal/domain"
)

const schema = `
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

// NewAnalyticsRepo opens (or creates) the SQLite database and runs migrations.
func NewAnalyticsRepo(dbPath string) (*AnalyticsRepo, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", dbPath, err)
	}

	// SQLite is single-writer: use WAL mode for better concurrency.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		return nil, fmt.Errorf("sqlite: pragma: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}

	return &AnalyticsRepo{db: db}, nil
}

// Close closes the underlying DB connection.
func (r *AnalyticsRepo) Close() error { return r.db.Close() }

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
		`INSERT INTO queries (query_hash, language, response_ms, sources_cnt, feedback, is_blocked)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		rec.QueryHash, lang, rec.ResponseMs, rec.SourcesCnt, int(rec.Feedback), blocked,
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
