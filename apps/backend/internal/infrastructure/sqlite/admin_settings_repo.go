package sqlite

import (
	"context"
	"database/sql"
	"log/slog"
)

// AdminSettingsRepo manages system-level admin settings in SQLite.
// Used for auto-admin first-user functionality.
type AdminSettingsRepo struct {
	db *sql.DB
}

// NewAdminSettingsRepo creates a new AdminSettingsRepo.
func NewAdminSettingsRepo(db *sql.DB) *AdminSettingsRepo {
	return &AdminSettingsRepo{db: db}
}

// Get retrieves a setting value by key. Returns ("", nil) if not found.
func (r *AdminSettingsRepo) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM admin_settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// Set inserts or updates a setting value.
func (r *AdminSettingsRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO admin_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}

// IsFirstAdmin checks if any admin has been registered yet.
// Returns true if no admin exists (the caller should auto-promote).
func (r *AdminSettingsRepo) IsFirstAdmin(ctx context.Context) bool {
	val, err := r.Get(ctx, "first_admin_email")
	if err != nil {
		slog.Error("Failed to check first admin", "error", err)
		return false
	}
	return val == ""
}

// SetFirstAdmin records the first admin email (one-time operation).
func (r *AdminSettingsRepo) SetFirstAdmin(ctx context.Context, email string) error {
	return r.Set(ctx, "first_admin_email", email)
}

// SetFirstAdminAtomic atomically promotes the first authenticated user to admin.
//
// C-3 Fix: The naive Get()+Set() pattern has a TOCTOU race — two concurrent
// goroutines can both read firstAdmin=="" and both become admin. This method
// uses INSERT OR IGNORE inside a serialized SQLite transaction so that exactly
// one caller wins, regardless of concurrency. After the INSERT the actual
// stored value is re-read and returned so the caller can verify outcome.
//
// Returns (wonRace=true, nil) if this caller just set the first admin.
// Returns (wonRace=false, nil) if another caller already set a first admin.
func (r *AdminSettingsRepo) SetFirstAdminAtomic(ctx context.Context, email string) (wonRace bool, actualAdmin string, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, "", err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// INSERT OR IGNORE: only inserts if the key does not yet exist.
	// Under SQLite's exclusive write lock this is truly atomic.
	_, err = tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO admin_settings (key, value, updated_at)
		 VALUES ('first_admin_email', ?, CURRENT_TIMESTAMP)`,
		email,
	)
	if err != nil {
		return false, "", err
	}

	// Re-read the canonical value within the same transaction.
	var stored string
	err = tx.QueryRowContext(ctx,
		`SELECT value FROM admin_settings WHERE key = 'first_admin_email'`,
	).Scan(&stored)
	if err != nil {
		return false, "", err
	}

	if err = tx.Commit(); err != nil {
		return false, "", err
	}

	// If stored == email then our INSERT won the race.
	won := stored == email
	return won, stored, nil
}
