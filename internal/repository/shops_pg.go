package repository

import (
	"context"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type shopsPg struct{ db *pgxpool.Pool }

func NewShopsRepository(db *pgxpool.Pool) ShopsRepository { return &shopsPg{db: db} }

func (r *shopsPg) Create(ctx context.Context, shop *domain.Shop) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO shops
			(id, user_id, marketplace, name, credentials_encrypted,
			 status, auto_update_enabled, schedule_cron, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		shop.ID, shop.UserID, shop.Marketplace, shop.Name, shop.CredentialsEncrypted,
		shop.Status, shop.AutoUpdateEnabled, shop.ScheduleCron,
		shop.CreatedAt, shop.UpdatedAt,
	)
	return err
}

func (r *shopsPg) GetByID(ctx context.Context, id, userID uuid.UUID) (*domain.Shop, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, user_id, marketplace, name, credentials_encrypted,
		       status, auto_update_enabled, schedule_cron,
		       last_checked_at, created_at, updated_at
		FROM shops
		WHERE id=$1 AND user_id=$2`, id, userID)
	return scanShop(row)
}

func (r *shopsPg) ListByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Shop, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, marketplace, name, credentials_encrypted,
		       status, auto_update_enabled, schedule_cron,
		       last_checked_at, created_at, updated_at
		FROM shops
		WHERE user_id=$1
		ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shops []*domain.Shop
	for rows.Next() {
		shop, err := scanShop(rows)
		if err != nil {
			return nil, err
		}
		shops = append(shops, shop)
	}
	return shops, rows.Err()
}

func (r *shopsPg) Update(ctx context.Context, shop *domain.Shop) error {
	_, err := r.db.Exec(ctx, `
		UPDATE shops SET
			name                  = $3,
			credentials_encrypted = $4,
			auto_update_enabled   = $5,
			schedule_cron         = $6,
			updated_at            = $7
		WHERE id=$1 AND user_id=$2`,
		shop.ID, shop.UserID, shop.Name, shop.CredentialsEncrypted,
		shop.AutoUpdateEnabled, shop.ScheduleCron, shop.UpdatedAt,
	)
	return err
}

func (r *shopsPg) Delete(ctx context.Context, id, userID uuid.UUID) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM shops WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *shopsPg) UpdateStatus(ctx context.Context, id uuid.UUID, status string, checkedAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE shops SET status=$2, last_checked_at=$3, updated_at=$3 WHERE id=$1`,
		id, status, checkedAt)
	return err
}

// scanShop читает одну строку из любого источника (Row или Rows).
type scannable interface {
	Scan(dest ...any) error
}

func scanShop(row scannable) (*domain.Shop, error) {
	var s domain.Shop
	err := row.Scan(
		&s.ID, &s.UserID, &s.Marketplace, &s.Name, &s.CredentialsEncrypted,
		&s.Status, &s.AutoUpdateEnabled, &s.ScheduleCron,
		&s.LastCheckedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}
