package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pricePlansPg struct{ db *pgxpool.Pool }

func NewPricePlansRepository(db *pgxpool.Pool) PricePlansRepository {
	return &pricePlansPg{db: db}
}

func (r *pricePlansPg) Create(ctx context.Context, shopID uuid.UUID) (*domain.PricePlan, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO price_plans (shop_id, status)
		VALUES ($1, 'pending'::plan_status)
		RETURNING id, shop_id, status::text, created_at, updated_at`, shopID)

	var p domain.PricePlan
	if err := row.Scan(&p.ID, &p.ShopID, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, fmt.Errorf("price_plans create: %w", err)
	}
	return &p, nil
}

func (r *pricePlansPg) GetByIDForUser(ctx context.Context, userID, planID uuid.UUID) (*domain.PricePlan, []*domain.PricePlanItem, error) {
	row := r.db.QueryRow(ctx, `
		SELECT pp.id, pp.shop_id, pp.status::text, pp.created_at, pp.updated_at,
		       (SELECT COUNT(*) FROM price_plan_items WHERE plan_id = pp.id) AS total
		FROM price_plans pp
		JOIN shops s ON s.id = pp.shop_id
		WHERE pp.id = $1 AND s.user_id = $2`, planID, userID)

	var p domain.PricePlan
	if err := row.Scan(&p.ID, &p.ShopID, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.Total); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("price_plans get: %w", err)
	}

	rows, err := r.db.Query(ctx, `
		SELECT ppi.id, ppi.plan_id, ppi.product_id, p.name, ppi.strategy_id,
		       ppi.current_price::float8, ppi.target_price::float8, ppi.final_price::float8,
		       ppi.constraint_hit, ppi.status::text, ppi.error, ppi.correlation_id,
		       ppi.created_at, ppi.updated_at
		FROM price_plan_items ppi
		JOIN products p ON p.id = ppi.product_id
		WHERE ppi.plan_id = $1
		ORDER BY ppi.created_at`, planID)
	if err != nil {
		return nil, nil, fmt.Errorf("price_plan_items list: %w", err)
	}
	defer rows.Close()

	var items []*domain.PricePlanItem
	for rows.Next() {
		var it domain.PricePlanItem
		if err := rows.Scan(&it.ID, &it.PlanID, &it.ProductID, &it.ProductName, &it.StrategyID,
			&it.CurrentPrice, &it.TargetPrice, &it.FinalPrice,
			&it.ConstraintHit, &it.Status, &it.Error, &it.CorrelationID,
			&it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, nil, fmt.Errorf("price_plan_items scan: %w", err)
		}
		items = append(items, &it)
	}
	return &p, items, rows.Err()
}

func (r *pricePlansPg) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.PricePlan, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM price_plans pp
		JOIN shops s ON s.id = pp.shop_id
		WHERE s.user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("price_plans count: %w", err)
	}

	rows, err := r.db.Query(ctx, `
		SELECT pp.id, pp.shop_id, pp.status::text, pp.created_at, pp.updated_at,
		       (SELECT COUNT(*) FROM price_plan_items WHERE plan_id = pp.id) AS items_total
		FROM price_plans pp
		JOIN shops s ON s.id = pp.shop_id
		WHERE s.user_id = $1
		ORDER BY pp.created_at DESC
		LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("price_plans list: %w", err)
	}
	defer rows.Close()

	var plans []*domain.PricePlan
	for rows.Next() {
		var p domain.PricePlan
		if err := rows.Scan(&p.ID, &p.ShopID, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.Total); err != nil {
			return nil, 0, fmt.Errorf("price_plans list scan: %w", err)
		}
		plans = append(plans, &p)
	}
	return plans, total, rows.Err()
}

func (r *pricePlansPg) UpdateStatus(ctx context.Context, planID uuid.UUID, status string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE price_plans
		SET status = $2::plan_status, updated_at = NOW()
		WHERE id = $1`, planID, status)
	if err != nil {
		return fmt.Errorf("price_plans update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pricePlansPg) LatestItemCreatedAt(ctx context.Context, productID uuid.UUID) (*time.Time, error) {
	var t time.Time
	err := r.db.QueryRow(ctx, `
		SELECT created_at FROM price_plan_items
		WHERE product_id = $1
		  AND status IN ('pending'::plan_item_status, 'applied'::plan_item_status)
		ORDER BY created_at DESC
		LIMIT 1`, productID).Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("price_plan_items latest: %w", err)
	}
	return &t, nil
}

func (r *pricePlansPg) InsertItems(ctx context.Context, planID uuid.UUID, items []*domain.PricePlanItem) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("price_plan_items tx begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	batch := &pgx.Batch{}
	for _, it := range items {
		batch.Queue(`
			INSERT INTO price_plan_items (
				plan_id, product_id, strategy_id, current_price, target_price,
				final_price, constraint_hit, status, error
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::plan_item_status, $9)`,
			planID, it.ProductID, it.StrategyID, it.CurrentPrice, it.TargetPrice,
			it.FinalPrice, it.ConstraintHit, it.Status, it.Error,
		)
	}
	br := tx.SendBatch(ctx, batch)
	for range items {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return fmt.Errorf("price_plan_items insert: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("price_plan_items batch close: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("price_plan_items commit: %w", err)
	}
	return nil
}
