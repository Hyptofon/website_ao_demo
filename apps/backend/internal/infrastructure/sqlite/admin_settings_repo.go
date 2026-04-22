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
