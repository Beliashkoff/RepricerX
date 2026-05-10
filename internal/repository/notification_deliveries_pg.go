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

type notificationDeliveriesPg struct{ db *pgxpool.Pool }

func NewNotificationDeliveriesRepository(db *pgxpool.Pool) NotificationDeliveriesRepository {
	return &notificationDeliveriesPg{db: db}
}

func (r *notificationDeliveriesPg) Create(ctx context.Context, tx pgx.Tx, in NotificationDeliveryCreate) (*domain.NotificationDelivery, error) {
	q := `
		INSERT INTO notification_deliveries (notification_id, channel, status)
		VALUES ($1, $2, $3)
		ON CONFLICT (notification_id, channel) DO UPDATE SET updated_at = NOW()
		RETURNING id, notification_id, channel, status, attempts, last_error, job_id,
		          sent_at, created_at, updated_at`
	row := r.queryRow(ctx, tx, q, in.NotificationID, in.Channel, in.Status)
	return scanDelivery(row)
}

func (r *notificationDeliveriesPg) ListByNotification(ctx context.Context, notificationID uuid.UUID) ([]*domain.NotificationDelivery, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, notification_id, channel, status, attempts, last_error, job_id,
		       sent_at, created_at, updated_at
		FROM notification_deliveries
		WHERE notification_id = $1
		ORDER BY channel`, notificationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.NotificationDelivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *notificationDeliveriesPg) UpdateStatus(ctx context.Context, id uuid.UUID, status, lastError string, sentAt *time.Time) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE notification_deliveries
		SET status = $2, last_error = $3, sent_at = COALESCE($4, sent_at), updated_at = NOW()
		WHERE id = $1`, id, status, lastError, sentAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *notificationDeliveriesPg) AttachJob(ctx context.Context, id, jobID uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `UPDATE notification_deliveries SET job_id = $2, updated_at = NOW() WHERE id = $1`, id, jobID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *notificationDeliveriesPg) IncrementAttempts(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `UPDATE notification_deliveries SET attempts = attempts + 1, updated_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *notificationDeliveriesPg) ListPendingDigestPairs(ctx context.Context, channel string) ([]PendingDigestRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT n.user_id, d.channel
		FROM notification_deliveries d
		JOIN notifications n ON n.id = d.notification_id
		WHERE d.status = 'pending_digest' AND d.channel = $1`, channel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingDigestRow
	for rows.Next() {
		var p PendingDigestRow
		if err := rows.Scan(&p.UserID, &p.Channel); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *notificationDeliveriesPg) LockPendingDigestForUser(ctx context.Context, userID uuid.UUID, channel string) ([]uuid.UUID, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx, `
		SELECT d.id
		FROM notification_deliveries d
		JOIN notifications n ON n.id = d.notification_id
		WHERE d.status = 'pending_digest' AND d.channel = $1 AND n.user_id = $2
		ORDER BY d.created_at
		FOR UPDATE OF d SKIP LOCKED`, channel, userID)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE notification_deliveries
		SET status = 'queued_digest', updated_at = NOW()
		WHERE id = ANY($1)`, ids); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *notificationDeliveriesPg) LoadByIDs(ctx context.Context, deliveryIDs []uuid.UUID) ([]*domain.NotificationDelivery, []*domain.Notification, error) {
	if len(deliveryIDs) == 0 {
		return nil, nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT d.id, d.notification_id, d.channel, d.status, d.attempts, d.last_error, d.job_id,
		       d.sent_at, d.created_at, d.updated_at,
		       n.id, n.user_id, n.event_type, n.severity, n.title, n.body, n.data,
		       n.shop_id, n.plan_id, n.correlation_id, n.read_at, n.created_at
		FROM notification_deliveries d
		JOIN notifications n ON n.id = d.notification_id
		WHERE d.id = ANY($1)`, deliveryIDs)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var deliveries []*domain.NotificationDelivery
	var notifications []*domain.Notification
	for rows.Next() {
		var d domain.NotificationDelivery
		var n domain.Notification
		if err := rows.Scan(
			&d.ID, &d.NotificationID, &d.Channel, &d.Status, &d.Attempts, &d.LastError, &d.JobID,
			&d.SentAt, &d.CreatedAt, &d.UpdatedAt,
			&n.ID, &n.UserID, &n.EventType, &n.Severity, &n.Title, &n.Body, &n.Data,
			&n.ShopID, &n.PlanID, &n.CorrelationID, &n.ReadAt, &n.CreatedAt,
		); err != nil {
			return nil, nil, err
		}
		deliveries = append(deliveries, &d)
		notifications = append(notifications, &n)
	}
	return deliveries, notifications, rows.Err()
}

func (r *notificationDeliveriesPg) queryRow(ctx context.Context, tx pgx.Tx, q string, args ...any) pgx.Row {
	if tx != nil {
		return tx.QueryRow(ctx, q, args...)
	}
	return r.db.QueryRow(ctx, q, args...)
}

func scanDelivery(row scannable) (*domain.NotificationDelivery, error) {
	var d domain.NotificationDelivery
	err := row.Scan(
		&d.ID, &d.NotificationID, &d.Channel, &d.Status, &d.Attempts, &d.LastError, &d.JobID,
		&d.SentAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}
