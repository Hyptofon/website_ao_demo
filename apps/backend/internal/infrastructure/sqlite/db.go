package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
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

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}

	return db, nil
}

// ─── Versioned Migration System ─────────────────────────────────────────────
// Each migration runs once, tracked by schema_version table.
// New migrations are appended to the list — never modify existing ones.

type migration struct {
	Version     int
	Description string
	SQL         string
}

var migrations = []migration{
	{
		Version:     1,
		Description: "upload_jobs table",
		SQL: `
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
		);`,
	},
	{
		Version:     2,
		Description: "documents table for knowledge base management",
		SQL: `
		CREATE TABLE IF NOT EXISTS documents (
			id TEXT PRIMARY KEY,
			filename TEXT NOT NULL,
			doc_type TEXT NOT NULL DEFAULT 'general',
			language TEXT CHECK(language IN ('uk', 'en')) DEFAULT 'uk',
			chunk_count INTEGER NOT NULL DEFAULT 0,
			summary TEXT DEFAULT '',
			uploaded_by TEXT NOT NULL DEFAULT 'system',
			uploaded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_documents_uploaded_at ON documents(uploaded_at);`,
	},
	{
		Version:     3,
		Description: "audit_log table for admin action tracking",
		SQL: `
		CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			admin_email TEXT NOT NULL,
			action TEXT NOT NULL,
			target TEXT DEFAULT '',
			details TEXT DEFAULT '',
			ip TEXT DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at);
		CREATE INDEX IF NOT EXISTS idx_audit_admin ON audit_log(admin_email);`,
	},
	{
		Version:     4,
		Description: "prompt_variants table for A/B testing",
		SQL: `
		CREATE TABLE IF NOT EXISTS prompt_variants (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			language TEXT CHECK(language IN ('uk', 'en')) DEFAULT 'uk',
			prompt_text TEXT NOT NULL,
			is_active INTEGER NOT NULL DEFAULT 1,
			usage_count INTEGER NOT NULL DEFAULT 0,
			total_score REAL NOT NULL DEFAULT 0,
			score_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
	},
	{
		Version:     5,
		Description: "suggested_questions table",
		SQL: `
		CREATE TABLE IF NOT EXISTS suggested_questions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			question TEXT NOT NULL,
			language TEXT CHECK(language IN ('uk', 'en')) DEFAULT 'uk',
			is_auto INTEGER NOT NULL DEFAULT 0,
			priority INTEGER NOT NULL DEFAULT 100,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_suggestions_lang ON suggested_questions(language, priority);`,
	},
}

func runMigrations(db *sql.DB) error {
	// Ensure schema_version table exists
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		description TEXT NOT NULL,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	for _, m := range migrations {
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM schema_version WHERE version = ?", m.Version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check schema version %d: %w", m.Version, err)
		}
		if exists > 0 {
			continue // already applied
		}

		slog.Info("Applying migration", "version", m.Version, "description", m.Description)

		if _, err := db.Exec(m.SQL); err != nil {
			return fmt.Errorf("migration v%d (%s): %w", m.Version, m.Description, err)
		}

		if _, err := db.Exec("INSERT INTO schema_version (version, description) VALUES (?, ?)", m.Version, m.Description); err != nil {
			return fmt.Errorf("record migration v%d: %w", m.Version, err)
		}
	}

	return nil
}
