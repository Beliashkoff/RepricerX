package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type importLogPg struct{ db *pgxpool.Pool }

func NewImportLogRepository(db *pgxpool.Pool) ImportLogRepository { return &importLogPg{db: db} }

func (r *importLogPg) HasRunning(ctx context.Context, shopID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM import_log
			WHERE shop_id=$1 AND status IN ($2,$3)
		)`,
		shopID, domain.ImportStatusPending, domain.ImportStatusRunning,
	).Scan(&exists)
	return exists, err
}

func (r *importLogPg) Create(ctx context.Context, entry *domain.ImportLogEntry) error {
	errorsJSON, err := json.Marshal(entry.Errors)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO import_log
			(id, shop_id, job_id, user_id, status, started_at, requested_at, finished_at,
			 total, added, updated, skipped, failed, errors)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		entry.ID, entry.ShopID, entry.JobID, entry.UserID, entry.Status,
		entry.StartedAt, entry.RequestedAt, entry.FinishedAt,
		entry.Total, entry.Added, entry.Updated, entry.Skipped, entry.Failed, errorsJSON,
	)
	return err
}

func (r *importLogPg) GetByID(ctx context.Context, id uuid.UUID) (*domain.ImportLogEntry, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, shop_id, status, started_at, finished_at,
		       total, added, updated, skipped, failed, errors,
		       job_id, user_id, requested_at, '' AS job_status
		FROM import_log
		WHERE id=$1`, id)
	return scanImportLog(row)
}

func (r *importLogPg) GetForUser(ctx context.Context, userID, importID uuid.UUID) (*domain.ImportLogEntry, error) {
	row := r.db.QueryRow(ctx, `
		SELECT il.id, il.shop_id, il.status, il.started_at, il.finished_at,
		       il.total, il.added, il.updated, il.skipped, il.failed, il.errors,
		       il.job_id, il.user_id, il.requested_at, COALESCE(bj.status::text, '') AS job_status
		FROM import_log il
		JOIN shops s ON s.id=il.shop_id
		LEFT JOIN background_jobs bj ON bj.id=il.job_id
		WHERE il.id=$1 AND s.user_id=$2`, importID, userID)
	return scanImportLog(row)
}

func (r *importLogPg) EnqueueProductImport(ctx context.Context, userID, shopID uuid.UUID, maxAttempts int, cooldown time.Duration) (*domain.ImportLogEntry, *domain.BackgroundJob, time.Duration, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if cooldown > 0 {
		var lastRequested time.Time
		err := tx.QueryRow(ctx, `
			SELECT requested_at
			FROM import_log
			WHERE shop_id=$1
			  AND status IN ($2,$3,$4,$5)
			ORDER BY requested_at DESC
			LIMIT 1`,
			shopID, domain.ImportStatusSucceeded, domain.ImportStatusPartial,
			domain.ImportStatusFailed, domain.ImportStatusCanceled,
		).Scan(&lastRequested)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, 0, err
		}
		if err == nil {
			remaining := time.Until(lastRequested.Add(cooldown))
			if remaining > 0 {
				return nil, nil, remaining, ErrCooldownActive
			}
		}
	}

	now := time.Now().UTC()
	importID := uuid.New()
	jobID := uuid.New()
	payload, err := json.Marshal(domain.SKUImportJobPayload{
		ImportID: importID, ShopID: shopID, RequestedByUserID: userID,
	})
	if err != nil {
		return nil, nil, 0, err
	}

	jobRow := tx.QueryRow(ctx, `
		INSERT INTO background_jobs
			(id, job_type, status, queue, priority, payload, result,
			 attempts, max_attempts, run_at, created_at, updated_at)
		VALUES ($1,$2,$3,'default',0,$4,'{}'::jsonb,0,$5,$6,$6,$6)
		RETURNING id, job_type, status::text, queue, priority, payload, result,
		          attempts, max_attempts, run_at, locked_at, locked_by,
		          lock_expires_at, last_error, created_at, updated_at,
		          started_at, finished_at, canceled_at`,
		jobID, domain.BackgroundJobTypeSKUImport, domain.BackgroundJobStatusPending,
		payload, maxAttempts, now,
	)
	job, err := scanBackgroundJob(jobRow)
	if err != nil {
		return nil, nil, 0, err
	}

	entry := &domain.ImportLogEntry{
		ID: importID, ShopID: shopID, JobID: &jobID, UserID: &userID,
		Status: domain.ImportStatusPending, StartedAt: now, RequestedAt: now,
		JobStatus: domain.BackgroundJobStatusPending, Errors: []domain.ImportLogError{},
	}
	errorsJSON, err := json.Marshal(entry.Errors)
	if err != nil {
		return nil, nil, 0, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO import_log
			(id, shop_id, job_id, user_id, status, started_at, requested_at,
			 total, added, updated, skipped, failed, errors)
		VALUES ($1,$2,$3,$4,$5,$6,$7,0,0,0,0,0,$8)`,
		entry.ID, entry.ShopID, entry.JobID, entry.UserID, entry.Status,
		entry.StartedAt, entry.RequestedAt, errorsJSON,
	)
	if err != nil {
		if isPgUniqueViolation(err) {
			return nil, nil, 0, ErrDuplicate
		}
		return nil, nil, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, 0, err
	}
	return entry, job, 0, nil
}

func (r *importLogPg) MarkRunning(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE import_log
		SET status=$2
		WHERE id=$1 AND status IN ($3,$4)`,
		id, domain.ImportStatusRunning, domain.ImportStatusPending, domain.ImportStatusRunning,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *importLogPg) Finish(ctx context.Context, id uuid.UUID, status string, total, added, updated, skipped, failed int, errs []domain.ImportLogError, finishedAt time.Time) error {
	errorsJSON, err := json.Marshal(errs)
	if err != nil {
		return err
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE import_log
		SET status=$2, total=$3, added=$4, updated=$5, skipped=$6,
		    failed=$7, errors=$8, finished_at=$9
		WHERE id=$1 AND status IN ($10,$11)`,
		id, status, total, added, updated, skipped, failed, errorsJSON, finishedAt,
		domain.ImportStatusPending, domain.ImportStatusRunning,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanImportLog(row scannable) (*domain.ImportLogEntry, error) {
	var entry domain.ImportLogEntry
	var errorsJSON []byte
	err := row.Scan(
		&entry.ID, &entry.ShopID, &entry.Status, &entry.StartedAt, &entry.FinishedAt,
		&entry.Total, &entry.Added, &entry.Updated, &entry.Skipped, &entry.Failed, &errorsJSON,
		&entry.JobID, &entry.UserID, &entry.RequestedAt, &entry.JobStatus,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if len(errorsJSON) > 0 {
		if err := json.Unmarshal(errorsJSON, &entry.Errors); err != nil {
			return nil, err
		}
	}
	return &entry, nil
}

func isPgUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
