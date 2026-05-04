package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type backgroundJobsPg struct{ db *pgxpool.Pool }

func NewBackgroundJobsRepository(db *pgxpool.Pool) BackgroundJobsRepository {
	return &backgroundJobsPg{db: db}
}

func (r *backgroundJobsPg) ClaimNext(ctx context.Context, queue, workerID string, lockTTL time.Duration) (*domain.BackgroundJob, error) {
	row := r.db.QueryRow(ctx, `
		WITH picked AS (
			SELECT id
			FROM background_jobs
			WHERE queue=$1
			  AND (
			    (status IN ($3,$4) AND run_at <= NOW())
			    OR (status=$5 AND lock_expires_at < NOW() AND attempts < max_attempts)
			  )
			ORDER BY priority DESC, run_at ASC, created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE background_jobs j
		SET status=$5,
		    attempts=attempts+1,
		    locked_at=NOW(),
		    locked_by=$2,
		    lock_expires_at=NOW()+($6 * INTERVAL '1 second'),
		    started_at=COALESCE(started_at, NOW()),
		    updated_at=NOW()
		FROM picked
		WHERE j.id=picked.id
		RETURNING j.id, j.job_type, j.status::text, j.queue, j.priority,
		          j.payload, j.result, j.attempts, j.max_attempts, j.run_at,
		          j.locked_at, j.locked_by, j.lock_expires_at, j.last_error,
		          j.created_at, j.updated_at, j.started_at, j.finished_at, j.canceled_at`,
		queue, workerID, domain.BackgroundJobStatusPending, domain.BackgroundJobStatusRetrying,
		domain.BackgroundJobStatusRunning, int(lockTTL.Seconds()),
	)
	return scanBackgroundJob(row)
}

func (r *backgroundJobsPg) Succeed(ctx context.Context, id uuid.UUID, result []byte) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE background_jobs
		SET status=$2, result=$3, locked_at=NULL, locked_by=NULL, lock_expires_at=NULL,
		    finished_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND status=$4`,
		id, domain.BackgroundJobStatusSucceeded, result, domain.BackgroundJobStatusRunning,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *backgroundJobsPg) Retry(ctx context.Context, id uuid.UUID, runAt time.Time, lastError string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE background_jobs
		SET status=$2, run_at=$3, last_error=$4,
		    locked_at=NULL, locked_by=NULL, lock_expires_at=NULL, updated_at=NOW()
		WHERE id=$1 AND status=$5`,
		id, domain.BackgroundJobStatusRetrying, runAt, lastError, domain.BackgroundJobStatusRunning,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *backgroundJobsPg) Fail(ctx context.Context, id uuid.UUID, lastError string, result []byte) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE background_jobs
		SET status=$2, last_error=$3, result=$4,
		    locked_at=NULL, locked_by=NULL, lock_expires_at=NULL,
		    finished_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND status=$5`,
		id, domain.BackgroundJobStatusFailed, lastError, result, domain.BackgroundJobStatusRunning,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanBackgroundJob(row scannable) (*domain.BackgroundJob, error) {
	var job domain.BackgroundJob
	err := row.Scan(
		&job.ID, &job.JobType, &job.Status, &job.Queue, &job.Priority,
		&job.Payload, &job.Result, &job.Attempts, &job.MaxAttempts, &job.RunAt,
		&job.LockedAt, &job.LockedBy, &job.LockExpiresAt, &job.LastError,
		&job.CreatedAt, &job.UpdatedAt, &job.StartedAt, &job.FinishedAt, &job.CanceledAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &job, nil
}
