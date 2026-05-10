package repository

import (
	"context"
	"errors"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type webhooksPg struct{ db *pgxpool.Pool }

func NewWebhooksRepository(db *pgxpool.Pool) WebhooksRepository {
	return &webhooksPg{db: db}
}

func (r *webhooksPg) List(ctx context.Context, userID uuid.UUID) ([]*domain.Webhook, error) {
	rows, err := r.db.Query(ctx, baseWebhookSelect+" WHERE user_id = $1 ORDER BY created_at", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectWebhooks(rows)
}

func (r *webhooksPg) GetByIDForUser(ctx context.Context, userID, id uuid.UUID) (*domain.Webhook, error) {
	row := r.db.QueryRow(ctx, baseWebhookSelect+" WHERE id = $1 AND user_id = $2", id, userID)
	return scanWebhook(row)
}

func (r *webhooksPg) Create(ctx context.Context, userID uuid.UUID, in WebhookCreate) (*domain.Webhook, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO user_webhooks (user_id, url, secret, description)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, url, secret, enabled, description, created_at, updated_at`,
		userID, in.URL, in.Secret, in.Description)
	return scanWebhook(row)
}

func (r *webhooksPg) SetEnabled(ctx context.Context, userID, id uuid.UUID, enabled bool) error {
	tag, err := r.db.Exec(ctx, `UPDATE user_webhooks SET enabled = $3, updated_at = NOW() WHERE id = $1 AND user_id = $2`, id, userID, enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *webhooksPg) Delete(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM user_webhooks WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *webhooksPg) ListEnabledForUser(ctx context.Context, userID uuid.UUID) ([]*domain.Webhook, error) {
	rows, err := r.db.Query(ctx, baseWebhookSelect+" WHERE user_id = $1 AND enabled = TRUE", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectWebhooks(rows)
}

const baseWebhookSelect = `
	SELECT id, user_id, url, secret, enabled, description, created_at, updated_at
	FROM user_webhooks`

func collectWebhooks(rows pgx.Rows) ([]*domain.Webhook, error) {
	var out []*domain.Webhook
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func scanWebhook(row scannable) (*domain.Webhook, error) {
	var w domain.Webhook
	err := row.Scan(
		&w.ID, &w.UserID, &w.URL, &w.Secret, &w.Enabled, &w.Description,
		&w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &w, nil
}
