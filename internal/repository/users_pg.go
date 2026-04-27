package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type usersPg struct {
	db *pgxpool.Pool
}

func NewUsersRepository(db *pgxpool.Pool) UsersRepository {
	return &usersPg{db: db}
}

func (r *usersPg) Create(ctx context.Context, u *domain.User) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, status)
		VALUES ($1, $2, $3, $4, $5)
	`, u.ID, u.Email, u.PasswordHash, u.DisplayName, u.Status)
	if err != nil {
		// Postgres unique-violation code 23505 → ErrEmailTaken
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrEmailTaken
		}
		return err
	}
	return nil
}

func (r *usersPg) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.scanUser(ctx, `
		SELECT id, email, password_hash, display_name, status,
		       failed_login_count, lockout_until, created_at
		FROM users WHERE email = $1
	`, email)
}

func (r *usersPg) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return r.scanUser(ctx, `
		SELECT id, email, password_hash, display_name, status,
		       failed_login_count, lockout_until, created_at
		FROM users WHERE id = $1
	`, id)
}

func (r *usersPg) UpdateDisplayName(ctx context.Context, id uuid.UUID, name string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET display_name = $1 WHERE id = $2`, name, id)
	return err
}

func (r *usersPg) UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, hash, id)
	return err
}

func (r *usersPg) RegisterFailedLogin(ctx context.Context, id uuid.UUID, newCount int, lockoutUntil *time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users SET failed_login_count = $1, lockout_until = $2 WHERE id = $3
	`, newCount, lockoutUntil, id)
	return err
}

func (r *usersPg) ResetFailedLogin(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users SET failed_login_count = 0, lockout_until = NULL WHERE id = $1
	`, id)
	return err
}

func (r *usersPg) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET status = $1 WHERE id = $2`, status, id)
	return err
}

func (r *usersPg) scanUser(ctx context.Context, query string, args ...any) (*domain.User, error) {
	row := r.db.QueryRow(ctx, query, args...)
	u := &domain.User{}
	err := row.Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Status,
		&u.FailedLoginCount, &u.LockoutUntil, &u.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}
