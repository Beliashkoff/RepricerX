package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type passwordResetTokensPg struct {
	db *pgxpool.Pool
}

func NewPasswordResetTokensRepository(db *pgxpool.Pool) PasswordResetTokensRepository {
	return &passwordResetTokensPg{db: db}
}

func (r *passwordResetTokensPg) Issue(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `
		UPDATE password_reset_tokens SET used_at = now()
		WHERE user_id = $1 AND used_at IS NULL
	`, userID); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *passwordResetTokensPg) Consume(ctx context.Context, tokenHash string, newPasswordHash string) (uuid.UUID, int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, 0, err
	}
	defer tx.Rollback(ctx)

	var userID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE password_reset_tokens
		SET used_at = now()
		FROM users
		WHERE token_hash = $1
		  AND password_reset_tokens.user_id = users.id
		  AND users.status = 'active'
		  AND expires_at > now()
		  AND used_at IS NULL
		RETURNING password_reset_tokens.user_id
	`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, 0, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, 0, err
	}

	if _, err = tx.Exec(ctx, `
		UPDATE users
		SET password_hash = $1, failed_login_count = 0, lockout_until = NULL
		WHERE id = $2
	`, newPasswordHash, userID); err != nil {
		return uuid.Nil, 0, err
	}

	tag, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		return uuid.Nil, 0, err
	}

	if err = tx.Commit(ctx); err != nil {
		return uuid.Nil, 0, err
	}
	return userID, tag.RowsAffected(), nil
}

func (r *passwordResetTokensPg) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM password_reset_tokens WHERE expires_at < now()
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
