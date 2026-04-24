package sqlite

import (
	"context"
	"database/sql"
	"time"

	"university-chatbot/backend/internal/domain"
)

// AdminUsersRepo manages the list of authorized administrators.
// Implements domain.AdminUsersRepo.
type AdminUsersRepo struct {
	db *sql.DB
}

// NewAdminUsersRepo creates a new AdminUsersRepo.
func NewAdminUsersRepo(db *sql.DB) *AdminUsersRepo {
	return &AdminUsersRepo{db: db}
}

// List returns all registered admin users, newest first.
func (r *AdminUsersRepo) List(ctx context.Context) ([]domain.AdminUser, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, email, added_by, added_at FROM admin_users ORDER BY added_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var admins []domain.AdminUser
	for rows.Next() {
		var a domain.AdminUser
		if err := rows.Scan(&a.ID, &a.Email, &a.AddedBy, &a.AddedAt); err != nil {
			return nil, err
		}
		admins = append(admins, a)
	}
	return admins, rows.Err()
}

// Add inserts a new admin user. Returns ErrAdminAlreadyExists if the email is
// already registered.
func (r *AdminUsersRepo) Add(ctx context.Context, email, addedBy string) (*domain.AdminUser, error) {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO admin_users (email, added_by, added_at) VALUES (?, ?, ?)`,
		email, addedBy, now,
	)
	if err != nil {
		// SQLite UNIQUE constraint violation
		if isUniqueConstraintError(err) {
			return nil, domain.ErrAdminAlreadyExists
		}
		return nil, err
	}
	return &domain.AdminUser{
		Email:   email,
		AddedBy: addedBy,
		AddedAt: now,
	}, nil
}

// Delete removes an admin user by email.
// Returns domain.ErrAdminNotFound if the email is not in the table.
func (r *AdminUsersRepo) Delete(ctx context.Context, email string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM admin_users WHERE email = ?`, email,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrAdminNotFound
	}
	return nil
}

// Exists checks if an email is registered as an admin.
func (r *AdminUsersRepo) Exists(ctx context.Context, email string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM admin_users WHERE email = ?`, email,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// isUniqueConstraintError detects SQLite UNIQUE constraint violation.
// We avoid importing the sqlite driver's error type directly to keep this
// package driver-agnostic.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return len(msg) > 0 && (contains(msg, "UNIQUE constraint failed") || contains(msg, "constraint failed"))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
