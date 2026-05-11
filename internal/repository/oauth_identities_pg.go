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

type oauthIdentitiesPg struct {
	db *pgxpool.Pool
}

func NewOAuthIdentitiesRepository(db *pgxpool.Pool) OAuthIdentitiesRepository {
	return &oauthIdentitiesPg{db: db}
}

func (r *oauthIdentitiesPg) Create(ctx context.Context, identity *domain.OAuthIdentity) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO oauth_identities (id, user_id, provider, external_id, email, created_at, last_login_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, identity.ID, identity.UserID, string(identity.Provider),
		identity.ExternalID, identity.Email, identity.CreatedAt, identity.LastLoginAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

func (r *oauthIdentitiesPg) GetByProviderAndExternalID(ctx context.Context, provider domain.OAuthProvider, externalID string) (*domain.OAuthIdentity, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, user_id, provider, external_id, email, created_at, last_login_at
		FROM oauth_identities
		WHERE provider = $1 AND external_id = $2
	`, string(provider), externalID)

	identity := &domain.OAuthIdentity{}
	var prov string
	err := row.Scan(&identity.ID, &identity.UserID, &prov, &identity.ExternalID,
		&identity.Email, &identity.CreatedAt, &identity.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	identity.Provider = domain.OAuthProvider(prov)
	return identity, nil
}

func (r *oauthIdentitiesPg) TouchLastLogin(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE oauth_identities SET last_login_at = $1 WHERE id = $2`,
		time.Now(), id,
	)
	return err
}

func (r *oauthIdentitiesPg) ListByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.OAuthIdentity, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, provider, external_id, email, created_at, last_login_at
		FROM oauth_identities
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.OAuthIdentity
	for rows.Next() {
		identity := &domain.OAuthIdentity{}
		var prov string
		if err := rows.Scan(&identity.ID, &identity.UserID, &prov, &identity.ExternalID,
			&identity.Email, &identity.CreatedAt, &identity.LastLoginAt); err != nil {
			return nil, err
		}
		identity.Provider = domain.OAuthProvider(prov)
		out = append(out, identity)
	}
	return out, rows.Err()
}
