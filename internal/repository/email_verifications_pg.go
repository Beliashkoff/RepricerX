package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type emailVerificationsPg struct {
	db *pgxpool.Pool
}

func NewEmailVerificationsRepository(db *pgxpool.Pool) EmailVerificationsRepository {
	return &emailVerificationsPg{db: db}
}

func (r *emailVerificationsPg) Create(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO email_verifications (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt)
	return err
}

// GetUnusedValid ищет валидный неиспользованный токен — анти-replay через used_at IS NULL.
func (r *emailVerificationsPg) GetUnusedValid(ctx context.Context, tokenHash string) (id uuid.UUID, userID uuid.UUID, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT id, user_id FROM email_verifications
		WHERE token_hash = $1
		  AND expires_at > now()
		  AND used_at IS NULL
	`, tokenHash).Scan(&id, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	return id, userID, err
}

func (r *emailVerificationsPg) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE email_verifications SET used_at = now() WHERE id = $1
	`, id)
	return err
}

// InvalidatePending помечает все незакрытые токены юзера как использованные перед resend.
func (r *emailVerificationsPg) InvalidatePending(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE email_verifications SET used_at = now()
		WHERE user_id = $1 AND used_at IS NULL
	`, userID)
	return err
}

func (r *emailVerificationsPg) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM email_verifications WHERE expires_at < now()
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
