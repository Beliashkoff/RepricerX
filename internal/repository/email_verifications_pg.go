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

// ConsumeAndActivate атомарно (в одной CTE-инструкции) помечает токен использованным
// и переводит пользователя в 'active', если его статус 'pending_verification'.
// Если токен уже использован или пользователь не pending — возвращает ErrNotFound.
func (r *emailVerificationsPg) ConsumeAndActivate(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.db.QueryRow(ctx, `
		WITH consumed AS (
			UPDATE email_verifications
			SET used_at = now()
			WHERE token_hash = $1
			  AND expires_at > now()
			  AND used_at IS NULL
			RETURNING user_id
		)
		UPDATE users
		SET status = 'active'
		WHERE id = (SELECT user_id FROM consumed)
		  AND status = 'pending_verification'
		RETURNING id
	`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return userID, err
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
