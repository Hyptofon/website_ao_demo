package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// InitDB creates a connection to the SQLite database and initializes tables.
func InitDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	// Important pragmas for concurrent SQLite access
	db.Exec("PRAGMA journal_mode=WAL;")
	db.Exec("PRAGMA synchronous=NORMAL;")
	db.Exec("PRAGMA foreign_keys=ON;")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS upload_jobs (
		id TEXT PRIMARY KEY,
		filename TEXT NOT NULL,
		status TEXT NOT NULL,
		error TEXT,
		progress INTEGER NOT NULL DEFAULT 0,
		current_step TEXT NOT NULL DEFAULT '',
		chunks_count INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`
	_, err := db.Exec(schema)
	return err
}
