package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"university-chatbot/backend/internal/domain"
)

var ErrJobNotFound = errors.New("job not found")

// JobRepository persists upload jobs via SQLite.
type JobRepository struct {
	db *sql.DB
}

// NewJobRepository creates a new JobRepository.
func NewJobRepository(db *sql.DB) *JobRepository {
	return &JobRepository{db: db}
}

// CreateJob inserts a new job record.
func (r *JobRepository) CreateJob(ctx context.Context, job *domain.UploadJob) error {
	query := `INSERT INTO upload_jobs (id, filename, status, error, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`
	
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query, job.ID, job.Filename, job.Status, job.Error, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	return nil
}

// UpdateJobStatus modifies the status and error of an existing job.
func (r *JobRepository) UpdateJobStatus(ctx context.Context, id string, status domain.JobStatus, jobErr error) error {
	var errStr *string
	if jobErr != nil {
		s := jobErr.Error()
		errStr = &s
	}

	query := `UPDATE upload_jobs SET status = ?, error = ?, updated_at = ? WHERE id = ?`
	now := time.Now().UTC()

	res, err := r.db.ExecContext(ctx, query, status, errStr, now, id)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrJobNotFound
	}
	return nil
}

// GetJob retrieves a job by ID.
func (r *JobRepository) GetJob(ctx context.Context, id string) (*domain.UploadJob, error) {
	query := `SELECT filename, status, error, created_at, updated_at FROM upload_jobs WHERE id = ?`
	
	var job domain.UploadJob
	job.ID = id
	var errStr sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&job.Filename,
		&job.Status,
		&errStr,
		&job.CreatedAt,
		&job.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrJobNotFound
	} else if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}

	if errStr.Valid {
		job.Error = errStr.String
	}

	return &job, nil
}
