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

type sessionsPg struct {
	db *pgxpool.Pool
}

func NewSessionsRepository(db *pgxpool.Pool) SessionsRepository {
	return &sessionsPg{db: db}
}

func (r *sessionsPg) Create(ctx context.Context, s *domain.Session) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO sessions
			(id, user_id, token_hash, created_at, last_seen_at,
			 idle_expires_at, absolute_expires_at, user_agent, ip_prefix)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, s.ID, s.UserID, s.TokenHash, s.CreatedAt, s.LastSeenAt,
		s.IdleExpiresAt, s.AbsoluteExpiresAt, s.UserAgent, s.IPPrefix)
	return err
}

func (r *sessionsPg) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.Session, error) {
	s := &domain.Session{}
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, token_hash, created_at, last_seen_at,
		       idle_expires_at, absolute_expires_at, user_agent, ip_prefix
		FROM sessions
		WHERE token_hash = $1
		  AND idle_expires_at > now()
		  AND absolute_expires_at > now()
	`, tokenHash).Scan(
		&s.ID, &s.UserID, &s.TokenHash, &s.CreatedAt, &s.LastSeenAt,
		&s.IdleExpiresAt, &s.AbsoluteExpiresAt, &s.UserAgent, &s.IPPrefix,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// TouchIdleIfNeeded продлевает idle TTL только если до истечения < 12 ч.
// Новый idle ограничен absolute_expires_at — чтобы не выйти за абсолютный кэп.
// Возвращает nil если обновления не было (клиент не получает Set-Cookie).
func (r *sessionsPg) TouchIdleIfNeeded(ctx context.Context, id uuid.UUID, candidateIdle time.Time) (*time.Time, error) {
	var newIdle *time.Time
	err := r.db.QueryRow(ctx, `
		UPDATE sessions
		SET idle_expires_at = LEAST($2, absolute_expires_at)
		WHERE id = $1
		  AND idle_expires_at - now() < interval '12 hours'
		RETURNING idle_expires_at
	`, id, candidateIdle).Scan(&newIdle)
	if errors.Is(err, pgx.ErrNoRows) {
		// Условие не выполнено — обновления не было, это нормально
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return newIdle, nil
}

func (r *sessionsPg) TouchLastSeen(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE sessions SET last_seen_at = $1 WHERE id = $2`, at, id)
	return err
}

func (r *sessionsPg) DeleteByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (r *sessionsPg) DeleteByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *sessionsPg) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM sessions
		WHERE idle_expires_at < now() OR absolute_expires_at < now()
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
