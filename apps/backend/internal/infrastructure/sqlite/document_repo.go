package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"university-chatbot/backend/internal/domain"
)

// DocumentRepo is the SQLite-backed implementation of domain.DocumentRepo.
type DocumentRepo struct {
	db *sql.DB
}

// NewDocumentRepo creates the document metadata repository.
func NewDocumentRepo(db *sql.DB) *DocumentRepo {
	return &DocumentRepo{db: db}
}

// Create inserts a new document record.
func (r *DocumentRepo) Create(ctx context.Context, doc *domain.DocumentRecord) error {
	if doc.UploadedAt.IsZero() {
		doc.UploadedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO documents (id, filename, doc_type, language, chunk_count, summary, uploaded_by, uploaded_at) 
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		doc.ID, doc.Filename, doc.DocType, string(doc.Language),
		doc.ChunkCount, doc.Summary, doc.UploadedBy, doc.UploadedAt,
	)
	if err != nil {
		return fmt.Errorf("document create: %w", err)
	}
	return nil
}

// List returns all document records (newest first).
func (r *DocumentRepo) List(ctx context.Context) ([]domain.DocumentRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, filename, doc_type, language, chunk_count, summary, uploaded_by, uploaded_at 
		 FROM documents ORDER BY uploaded_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("document list: %w", err)
	}
	defer rows.Close()

	var docs []domain.DocumentRecord
	for rows.Next() {
		var d domain.DocumentRecord
		var lang string
		if err := rows.Scan(&d.ID, &d.Filename, &d.DocType, &lang, &d.ChunkCount, &d.Summary, &d.UploadedBy, &d.UploadedAt); err != nil {
			return nil, fmt.Errorf("document scan: %w", err)
		}
		d.Language = domain.Language(lang)
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// GetByID returns a single document by ID.
func (r *DocumentRepo) GetByID(ctx context.Context, id string) (*domain.DocumentRecord, error) {
	var d domain.DocumentRecord
	var lang string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, filename, doc_type, language, chunk_count, summary, uploaded_by, uploaded_at 
		 FROM documents WHERE id = ?`, id,
	).Scan(&d.ID, &d.Filename, &d.DocType, &lang, &d.ChunkCount, &d.Summary, &d.UploadedBy, &d.UploadedAt)

	if err == sql.ErrNoRows {
		return nil, domain.ErrDocumentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("document get: %w", err)
	}
	d.Language = domain.Language(lang)
	return &d, nil
}

// Delete removes a document record by ID.
func (r *DocumentRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM documents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("document delete: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return domain.ErrDocumentNotFound
	}
	return nil
}

// UpdateChunkCount sets the final chunk count after indexing.
func (r *DocumentRepo) UpdateChunkCount(ctx context.Context, id string, count int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE documents SET chunk_count = ? WHERE id = ?", count, id)
	return err
}
