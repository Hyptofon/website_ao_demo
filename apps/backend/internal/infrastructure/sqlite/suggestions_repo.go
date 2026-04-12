package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"university-chatbot/backend/internal/domain"
)

// SuggestionsRepo is the SQLite-backed implementation of domain.SuggestionsRepo.
type SuggestionsRepo struct {
	db *sql.DB
}

// NewSuggestionsRepo creates a new suggestions repository.
func NewSuggestionsRepo(db *sql.DB) *SuggestionsRepo {
	return &SuggestionsRepo{db: db}
}

// List returns active suggestions for a language, ordered by priority.
func (r *SuggestionsRepo) List(ctx context.Context, lang domain.Language, limit int) ([]domain.SuggestedQuestion, error) {
	if limit <= 0 {
		limit = 5
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, question, language, is_auto, priority 
		 FROM suggested_questions 
		 WHERE language = ? 
		 ORDER BY priority ASC 
		 LIMIT ?`, string(lang), limit)
	if err != nil {
		return nil, fmt.Errorf("suggestions list: %w", err)
	}
	defer rows.Close()

	var questions []domain.SuggestedQuestion
	for rows.Next() {
		var q domain.SuggestedQuestion
		var lang string
		var isAuto int
		if err := rows.Scan(&q.ID, &q.Question, &lang, &isAuto, &q.Priority); err != nil {
			return nil, fmt.Errorf("suggestions scan: %w", err)
		}
		q.Language = domain.Language(lang)
		q.IsAuto = isAuto == 1
		questions = append(questions, q)
	}
	return questions, rows.Err()
}

// Upsert creates or updates a suggested question. If ID > 0, updates; otherwise inserts.
func (r *SuggestionsRepo) Upsert(ctx context.Context, q *domain.SuggestedQuestion) error {
	if q.ID > 0 {
		_, err := r.db.ExecContext(ctx,
			`UPDATE suggested_questions SET question = ?, language = ?, is_auto = ?, priority = ? WHERE id = ?`,
			q.Question, string(q.Language), boolToInt(q.IsAuto), q.Priority, q.ID)
		return err
	}

	res, err := r.db.ExecContext(ctx,
		`INSERT INTO suggested_questions (question, language, is_auto, priority) VALUES (?, ?, ?, ?)`,
		q.Question, string(q.Language), boolToInt(q.IsAuto), q.Priority)
	if err != nil {
		return fmt.Errorf("suggestions upsert: %w", err)
	}
	id, _ := res.LastInsertId()
	q.ID = id
	return nil
}

// DeleteAuto removes all auto-generated suggestions for a language (for refresh cycle).
func (r *SuggestionsRepo) DeleteAuto(ctx context.Context, lang domain.Language) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM suggested_questions WHERE is_auto = 1 AND language = ?", string(lang))
	return err
}
