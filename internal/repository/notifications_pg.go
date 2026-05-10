package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type notificationsPg struct{ db *pgxpool.Pool }

func NewNotificationsRepository(db *pgxpool.Pool) NotificationsRepository {
	return &notificationsPg{db: db}
}

func (r *notificationsPg) Create(ctx context.Context, tx pgx.Tx, in NotificationCreate) (*domain.Notification, error) {
	q := `
		INSERT INTO notifications
			(user_id, event_type, severity, title, body, data, shop_id, plan_id, correlation_id)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6::jsonb, '{}'::jsonb), $7, $8, $9)
		RETURNING id, user_id, event_type, severity, title, body, data,
		          shop_id, plan_id, correlation_id, read_at, created_at`
	args := []any{
		in.UserID, in.EventType, in.Severity, in.Title, in.Body, in.Data,
		in.ShopID, in.PlanID, in.CorrelationID,
	}
	row := r.queryRow(ctx, tx, q, args...)
	return scanNotification(row)
}

func (r *notificationsPg) GetByIDForUser(ctx context.Context, userID, id uuid.UUID) (*domain.Notification, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, user_id, event_type, severity, title, body, data,
		       shop_id, plan_id, correlation_id, read_at, created_at
		FROM notifications
		WHERE id = $1 AND user_id = $2`, id, userID)
	return scanNotification(row)
}

func (r *notificationsPg) ListForUser(ctx context.Context, userID uuid.UUID, f NotificationListFilter) ([]*domain.Notification, int, error) {
	page := f.Page
	if page < 1 {
		page = 1
	}
	perPage := f.PerPage
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	conds := []string{"user_id = $1"}
	args := []any{userID}
	idx := 2
	if f.EventType != "" {
		conds = append(conds, fmt.Sprintf("event_type = $%d", idx))
		args = append(args, f.EventType)
		idx++
	}
	if f.Severity != "" {
		conds = append(conds, fmt.Sprintf("severity = $%d", idx))
		args = append(args, f.Severity)
		idx++
	}
	if f.UnreadOnly {
		conds = append(conds, "read_at IS NULL")
	}
	if !f.From.IsZero() {
		conds = append(conds, fmt.Sprintf("created_at >= $%d", idx))
		args = append(args, f.From)
		idx++
	}
	if !f.Until.IsZero() {
		conds = append(conds, fmt.Sprintf("created_at <= $%d", idx))
		args = append(args, f.Until)
		idx++
	}
	if f.ShopID != nil {
		conds = append(conds, fmt.Sprintf("shop_id = $%d", idx))
		args = append(args, *f.ShopID)
		idx++
	}

	where := strings.Join(conds, " AND ")

	var total int
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := fmt.Sprintf(`
		SELECT id, user_id, event_type, severity, title, body, data,
		       shop_id, plan_id, correlation_id, read_at, created_at
		FROM notifications
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, idx, idx+1)
	args = append(args, perPage, offset)

	rows, err := r.db.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*domain.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, n)
	}
	return items, total, rows.Err()
}

func (r *notificationsPg) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

func (r *notificationsPg) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE notifications SET read_at = NOW()
		WHERE id = $1 AND user_id = $2 AND read_at IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Может быть либо чужая запись, либо уже прочитанная — клиенту
		// одинаково неинтересно различать; ErrNotFound будет уловлен только
		// если действительно нет записи.
		var exists bool
		if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notifications WHERE id = $1 AND user_id = $2)`, id, userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (r *notificationsPg) MarkAllRead(ctx context.Context, userID uuid.UUID) (int64, error) {
	tag, err := r.db.Exec(ctx, `UPDATE notifications SET read_at = NOW() WHERE user_id = $1 AND read_at IS NULL`, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *notificationsPg) Delete(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM notifications WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *notificationsPg) ExistsRecentByDedupe(ctx context.Context, userID uuid.UUID, eventType string, shopID *uuid.UUID, since time.Time) (bool, error) {
	var exists bool
	if shopID == nil {
		err := r.db.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM notifications
				WHERE user_id = $1 AND event_type = $2 AND created_at >= $3
			)`, userID, eventType, since).Scan(&exists)
		return exists, err
	}
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM notifications
			WHERE user_id = $1 AND event_type = $2
			  AND shop_id = $3 AND created_at >= $4
		)`, userID, eventType, *shopID, since).Scan(&exists)
	return exists, err
}

func (r *notificationsPg) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM notifications WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *notificationsPg) queryRow(ctx context.Context, tx pgx.Tx, q string, args ...any) pgx.Row {
	if tx != nil {
		return tx.QueryRow(ctx, q, args...)
	}
	return r.db.QueryRow(ctx, q, args...)
}

func scanNotification(row scannable) (*domain.Notification, error) {
	var n domain.Notification
	err := row.Scan(
		&n.ID, &n.UserID, &n.EventType, &n.Severity, &n.Title, &n.Body, &n.Data,
		&n.ShopID, &n.PlanID, &n.CorrelationID, &n.ReadAt, &n.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &n, nil
}
