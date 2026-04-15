package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"university-chatbot/backend/internal/domain"
)

// PromptRepo is the SQLite-backed implementation of domain.PromptRepo.
// Manages A/B testing prompt variants with usage tracking and scoring.
type PromptRepo struct {
	db *sql.DB
}

// NewPromptRepo creates a new prompt variant repository.
func NewPromptRepo(db *sql.DB) *PromptRepo {
	return &PromptRepo{db: db}
}

// ActiveVariants returns all active prompt variants for a given language.
func (r *PromptRepo) ActiveVariants(ctx context.Context, lang domain.Language) ([]domain.PromptVariant, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, language, prompt_text, is_active, usage_count,
		        CASE WHEN score_count > 0 THEN total_score / score_count ELSE 0 END as avg_score
		 FROM prompt_variants WHERE is_active = 1 AND language = ?`, string(lang))
	if err != nil {
		return nil, fmt.Errorf("prompt active variants: %w", err)
	}
	defer rows.Close()

	return scanVariants(rows)
}

// IncrementUsage atomically increments usage_count for a variant.
func (r *PromptRepo) IncrementUsage(ctx context.Context, variantID int64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE prompt_variants SET usage_count = usage_count + 1 WHERE id = ?", variantID)
	return err
}

// RecordScore records a feedback score for a variant (running average via total/count).
func (r *PromptRepo) RecordScore(ctx context.Context, variantID int64, score float64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE prompt_variants SET total_score = total_score + ?, score_count = score_count + 1 WHERE id = ?",
		score, variantID)
	return err
}

// List returns all prompt variants (for admin panel overview).
func (r *PromptRepo) List(ctx context.Context) ([]domain.PromptVariant, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, language, prompt_text, is_active, usage_count,
		        CASE WHEN score_count > 0 THEN total_score / score_count ELSE 0 END as avg_score
		 FROM prompt_variants ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("prompt list: %w", err)
	}
	defer rows.Close()

	return scanVariants(rows)
}

// Create inserts a new prompt variant.
func (r *PromptRepo) Create(ctx context.Context, variant *domain.PromptVariant) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO prompt_variants (name, language, prompt_text, is_active) VALUES (?, ?, ?, ?)`,
		variant.Name, string(variant.Language), variant.PromptText, boolToInt(variant.IsActive))
	if err != nil {
		return fmt.Errorf("prompt create: %w", err)
	}
	id, _ := res.LastInsertId()
	variant.ID = id
	return nil
}

// SetActive toggles the is_active flag.
func (r *PromptRepo) SetActive(ctx context.Context, id int64, active bool) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE prompt_variants SET is_active = ? WHERE id = ?", boolToInt(active), id)
	return err
}

// Update modifies the text of a prompt variant.
func (r *PromptRepo) Update(ctx context.Context, id int64, text string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE prompt_variants SET prompt_text = ? WHERE id = ?", text, id)
	return err
}

// Delete removes a prompt variant.
func (r *PromptRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM prompt_variants WHERE id = ?", id)
	return err
}

func scanVariants(rows *sql.Rows) ([]domain.PromptVariant, error) {
	var variants []domain.PromptVariant
	for rows.Next() {
		var v domain.PromptVariant
		var lang string
		var isActive int
		if err := rows.Scan(&v.ID, &v.Name, &lang, &v.PromptText, &isActive, &v.UsageCount, &v.AvgScore); err != nil {
			return nil, fmt.Errorf("prompt scan: %w", err)
		}
		v.Language = domain.Language(lang)
		v.IsActive = isActive == 1
		variants = append(variants, v)
	}
	return variants, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
