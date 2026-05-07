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

type priceChangesPg struct{ db *pgxpool.Pool }

func NewPriceChangesRepository(db *pgxpool.Pool) PriceChangesRepository {
	return &priceChangesPg{db: db}
}

func (r *priceChangesPg) ListForUser(ctx context.Context, userID uuid.UUID, limit int) ([]*domain.PriceChange, error) {
	if limit < 1 || limit > 500 {
		limit = 200
	}
	return r.list(ctx, userID, limit)
}

func (r *priceChangesPg) ExportForUser(ctx context.Context, userID uuid.UUID) ([]*domain.PriceChange, error) {
	return r.list(ctx, userID, 10000)
}

func (r *priceChangesPg) SummaryForUser(ctx context.Context, userID uuid.UUID, since time.Time, until time.Time) (*domain.PriceChangeSummary, error) {
	row := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE pcl.status='applied')::int,
			COUNT(*) FILTER (WHERE pcl.status='failed')::int,
			COALESCE(AVG(((pcl.new_price - pcl.old_price) / NULLIF(pcl.old_price, 0)) * 100), 0)::float8
		FROM price_change_log pcl
		JOIN shops sh ON sh.id=pcl.shop_id
		WHERE sh.user_id=$1 AND pcl.created_at >= $2 AND pcl.created_at <= $3`,
		userID, since, until,
	)

	summary := &domain.PriceChangeSummary{PeriodStart: since, PeriodEnd: until}
	if err := row.Scan(&summary.TotalUpdates, &summary.SuccessfulUpdates, &summary.FailedUpdates, &summary.AvgChangePct); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return summary, nil
		}
		return nil, err
	}
	return summary, nil
}

func (r *priceChangesPg) list(ctx context.Context, userID uuid.UUID, limit int) ([]*domain.PriceChange, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			pcl.id, pcl.shop_id, pcl.product_id, COALESCE(p.name, '') AS product_name,
			pcl.strategy_id, pcl.old_price::float8, pcl.new_price::float8, pcl.target_price::float8,
			pcl.reason, NULLIF(pcl.constraint_hit, '') AS constraint_hit,
			CASE pcl.status
				WHEN 'applied' THEN 'success'
				WHEN 'failed' THEN 'failed'
				ELSE 'skipped'
			END AS status,
			pcl.created_at
		FROM price_change_log pcl
		JOIN shops sh ON sh.id=pcl.shop_id
		LEFT JOIN products p ON p.id=pcl.product_id
		WHERE sh.user_id=$1
		ORDER BY pcl.created_at DESC, pcl.id DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*domain.PriceChange
	for rows.Next() {
		item, err := scanPriceChange(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanPriceChange(row scannable) (*domain.PriceChange, error) {
	var c domain.PriceChange
	if err := row.Scan(
		&c.ID, &c.ShopID, &c.ProductID, &c.ProductName, &c.StrategyID,
		&c.OldPrice, &c.NewPrice, &c.TargetPrice, &c.Reason, &c.ConstraintHit,
		&c.Status, &c.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}
