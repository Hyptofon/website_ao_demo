package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
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

// ─── Refresh Token Revocation ─────────────────────────────────────────────────
// Stateless revocation without a blacklist table: we store the "revoke timestamp"
// per email in admin_settings. A refresh token is valid only if its iat (issued-at)
// is strictly after the latest revoke timestamp.
// This invalidates ALL refresh tokens for the user on logout — clean and simple.

// RevokeRefreshTokens records the current time as the revocation timestamp for email.
// All refresh tokens issued before (and at) this moment become invalid.
func (r *AdminSettingsRepo) RevokeRefreshTokens(ctx context.Context, email string) error {
	return r.Set(ctx, revokeKey(email), fmt.Sprintf("%d", time.Now().Unix()))
}

// IsRefreshTokenValid returns true if the token's IssuedAt is after the last
// revocation timestamp for the email. Returns true if no revocation has been recorded.
func (r *AdminSettingsRepo) IsRefreshTokenValid(ctx context.Context, email string, issuedAt int64) (bool, error) {
	val, err := r.Get(ctx, revokeKey(email))
	if err != nil {
		return false, err
	}
	if val == "" {
		return true, nil // never revoked
	}
	var revokeUnix int64
	if _, err := fmt.Sscanf(val, "%d", &revokeUnix); err != nil {
		return false, fmt.Errorf("corrupt revoke timestamp for %q: %w", email, err)
	}
	// Token must have been issued strictly AFTER the revocation time.
	return issuedAt > revokeUnix, nil
}

// RevokeJTI records a specific JWT ID as revoked.
// This provides granular session revocation.
func (r *AdminSettingsRepo) RevokeJTI(ctx context.Context, jti string, exp int64) error {
	// We store the JTI as the key, and the expiration time as the value.
	// A cleanup job could later remove expired JTIs to save space.
	return r.Set(ctx, "revoked_jti_"+jti, fmt.Sprintf("%d", exp))
}

// IsJTIRevoked checks if a specific JWT ID is in the blacklist.
func (r *AdminSettingsRepo) IsJTIRevoked(ctx context.Context, jti string) (bool, error) {
	if jti == "" {
		return false, nil // older tokens without JTI are not individually blacklisted
	}
	val, err := r.Get(ctx, "revoked_jti_"+jti)
	if err != nil {
		return false, err
	}
	return val != "", nil
}

// CleanupExpiredJTIs removes JWT IDs from the blacklist that have passed their expiration time.
func (r *AdminSettingsRepo) CleanupExpiredJTIs(ctx context.Context) error {
	now := time.Now().Unix()
	
	// Use a direct query because our generic Set/Get doesn't easily support range deletes
	// Note: We need to filter by key prefix "revoked_jti_" and CAST the value to integer.
	query := `
		DELETE FROM admin_settings 
		WHERE key LIKE 'revoked_jti_%' 
		AND CAST(value AS INTEGER) < ?
	`
	_, err := r.db.ExecContext(ctx, query, now)
	return err
}

// revokeKey returns the admin_settings key for the refresh revocation timestamp.
func revokeKey(email string) string {
	return "refresh_revoke:" + email
}
