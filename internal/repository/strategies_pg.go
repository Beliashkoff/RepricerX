package repository

import (
	"context"
	"errors"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type strategiesPg struct{ db *pgxpool.Pool }

func NewStrategiesRepository(db *pgxpool.Pool) StrategiesRepository {
	return &strategiesPg{db: db}
}

func (r *strategiesPg) ListByUser(ctx context.Context, userID uuid.UUID) ([]*domain.Strategy, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, name, type::text, params, constraints, fallback_policy::text,
		       priority, enabled, created_at, updated_at
		FROM strategies
		WHERE user_id=$1
		ORDER BY priority ASC, created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*domain.Strategy
	for rows.Next() {
		item, err := scanStrategy(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *strategiesPg) GetByIDForUser(ctx context.Context, userID, strategyID uuid.UUID) (*domain.Strategy, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, user_id, name, type::text, params, constraints, fallback_policy::text,
		       priority, enabled, created_at, updated_at
		FROM strategies
		WHERE id=$1 AND user_id=$2`, strategyID, userID)
	return scanStrategy(row)
}

func (r *strategiesPg) Create(ctx context.Context, userID uuid.UUID, input StrategyCreateInput) (*domain.Strategy, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO strategies
			(user_id, name, type, params, constraints, fallback_policy, priority, enabled, created_at, updated_at)
		VALUES ($1,$2,$3,$4::jsonb,$5::jsonb,$6,$7,$8,NOW(),NOW())
		RETURNING id, user_id, name, type::text, params, constraints, fallback_policy::text,
		          priority, enabled, created_at, updated_at`,
		userID, input.Name, input.Type, input.Params, input.Constraints,
		input.FallbackPolicy, input.Priority, input.Enabled,
	)
	return scanStrategy(row)
}

func (r *strategiesPg) Update(ctx context.Context, userID, strategyID uuid.UUID, input StrategyUpdateInput) (*domain.Strategy, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE strategies SET
			name            = CASE WHEN $3::text IS NOT NULL THEN $3 ELSE name END,
			type            = CASE WHEN $4::text IS NOT NULL THEN $4::strategy_type ELSE type END,
			params          = CASE WHEN $5::jsonb IS NOT NULL THEN $5 ELSE params END,
			constraints     = CASE WHEN $6::jsonb IS NOT NULL THEN $6 ELSE constraints END,
			fallback_policy = CASE WHEN $7::text IS NOT NULL THEN $7::fallback_policy ELSE fallback_policy END,
			priority        = CASE WHEN $8::int IS NOT NULL THEN $8 ELSE priority END,
			enabled         = CASE WHEN $9::boolean IS NOT NULL THEN $9 ELSE enabled END,
			updated_at      = NOW()
		WHERE id=$1 AND user_id=$2
		RETURNING id, user_id, name, type::text, params, constraints, fallback_policy::text,
		          priority, enabled, created_at, updated_at`,
		strategyID, userID,
		input.Name, input.Type,
		input.Params, input.Constraints,
		input.FallbackPolicy, input.Priority, input.Enabled,
	)
	s, err := scanStrategy(row)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *strategiesPg) CountAssignments(ctx context.Context, strategyID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM strategy_assignments WHERE strategy_id=$1`, strategyID,
	).Scan(&n)
	return n, err
}

func (r *strategiesPg) Delete(ctx context.Context, userID, strategyID uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM strategies WHERE id=$1 AND user_id=$2`, strategyID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanStrategy(row scannable) (*domain.Strategy, error) {
	var s domain.Strategy
	if err := row.Scan(
		&s.ID, &s.UserID, &s.Name, &s.Type, &s.Params, &s.Constraints,
		&s.FallbackPolicy, &s.Priority, &s.Enabled, &s.CreatedAt, &s.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

