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
	query := `INSERT INTO upload_jobs (id, filename, status, error, progress, current_step, chunks_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query, job.ID, job.Filename, job.Status, job.Error,
		job.Progress, job.CurrentStep, job.ChunksCount, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	return nil
}

// UpsertJob inserts or replaces a job record (idempotent).
// Used by ReindexAll so that re-running reindex resets the job state
// even if a job with the same ID already exists from a previous upload.
func (r *JobRepository) UpsertJob(ctx context.Context, job *domain.UploadJob) error {
	query := `INSERT OR REPLACE INTO upload_jobs (id, filename, status, error, progress, current_step, chunks_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query, job.ID, job.Filename, job.Status, job.Error,
		job.Progress, job.CurrentStep, job.ChunksCount, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert job: %w", err)
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

	// When completing or failing, set progress to 100 or keep current
	progress := 0
	if status == domain.JobStatusCompleted {
		progress = 100
	}

	query := `UPDATE upload_jobs SET status = ?, error = ?, progress = CASE WHEN ? > 0 THEN ? ELSE progress END, updated_at = ? WHERE id = ?`
	now := time.Now().UTC()

	res, err := r.db.ExecContext(ctx, query, status, errStr, progress, progress, now, id)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrJobNotFound
	}
	return nil
}

// UpdateProgress updates the progress percentage and current step description.
// Inspired by python_service-dev/worker.py publish_callback pattern.
func (r *JobRepository) UpdateProgress(ctx context.Context, id string, progress int, step string) error {
	query := `UPDATE upload_jobs SET progress = ?, current_step = ?, updated_at = ? WHERE id = ?`
	now := time.Now().UTC()

	res, err := r.db.ExecContext(ctx, query, progress, step, now, id)
	if err != nil {
		return fmt.Errorf("update progress: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrJobNotFound
	}
	return nil
}

// UpdateChunksCount sets the final chunks_count after indexing is complete.
func (r *JobRepository) UpdateChunksCount(ctx context.Context, id string, count int) error {
	query := `UPDATE upload_jobs SET chunks_count = ?, updated_at = ? WHERE id = ?`
	now := time.Now().UTC()

	_, err := r.db.ExecContext(ctx, query, count, now, id)
	return err
}

// GetJob retrieves a job by ID. Now includes progress tracking fields.
func (r *JobRepository) GetJob(ctx context.Context, id string) (*domain.UploadJob, error) {
	query := `SELECT filename, status, error, progress, current_step, chunks_count, created_at, updated_at FROM upload_jobs WHERE id = ?`

	var job domain.UploadJob
	job.ID = id
	var errStr sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&job.Filename,
		&job.Status,
		&errStr,
		&job.Progress,
		&job.CurrentStep,
		&job.ChunksCount,
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
