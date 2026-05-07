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

type productCompetitorsPg struct{ db *pgxpool.Pool }

func NewProductCompetitorsRepository(db *pgxpool.Pool) ProductCompetitorsRepository {
	return &productCompetitorsPg{db: db}
}

func (r *productCompetitorsPg) Create(ctx context.Context, userID uuid.UUID, input CompetitorCreateInput) (*domain.ProductCompetitor, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO product_competitors
			(product_id, marketplace, source, competitor_url, normalized_competitor_url,
			 ozon_public_product_id, ozon_sku, created_at, updated_at)
		SELECT $2, $3, $4, $5, $6, $7, $8, NOW(), NOW()
		FROM products p
		JOIN shops s ON s.id = p.shop_id
		WHERE p.id = $2 AND s.user_id = $1
		RETURNING id, product_id, marketplace::text, source, competitor_url,
		          normalized_competitor_url, ozon_public_product_id, ozon_sku,
		          last_price::float8, last_availability, last_checked_at,
		          last_error_code, last_status, created_at, updated_at`,
		userID, input.ProductID, input.Marketplace, input.Source, input.CompetitorURL,
		input.NormalizedCompetitorURL, input.OzonPublicProductID, input.OzonSKU,
	)
	competitor, err := scanProductCompetitor(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if isUniqueViolation(err) {
		return nil, ErrDuplicate
	}
	return competitor, err
}

func (r *productCompetitorsPg) ListByProduct(ctx context.Context, userID, productID uuid.UUID) ([]*domain.ProductCompetitor, error) {
	rows, err := r.db.Query(ctx, `
		SELECT pc.id, pc.product_id, pc.marketplace::text, pc.source, pc.competitor_url,
		       pc.normalized_competitor_url, pc.ozon_public_product_id, pc.ozon_sku,
		       pc.last_price::float8, pc.last_availability, pc.last_checked_at,
		       pc.last_error_code, pc.last_status, pc.created_at, pc.updated_at
		FROM product_competitors pc
		JOIN products p ON p.id = pc.product_id
		JOIN shops s ON s.id = p.shop_id
		WHERE pc.product_id = $1 AND s.user_id = $2
		ORDER BY pc.created_at DESC, pc.id DESC`,
		productID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*domain.ProductCompetitor
	for rows.Next() {
		item, err := scanProductCompetitor(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *productCompetitorsPg) GetByIDForUser(ctx context.Context, userID, competitorID uuid.UUID) (*domain.ProductCompetitor, error) {
	row := r.db.QueryRow(ctx, `
		SELECT pc.id, pc.product_id, pc.marketplace::text, pc.source, pc.competitor_url,
		       pc.normalized_competitor_url, pc.ozon_public_product_id, pc.ozon_sku,
		       pc.last_price::float8, pc.last_availability, pc.last_checked_at,
		       pc.last_error_code, pc.last_status, pc.created_at, pc.updated_at
		FROM product_competitors pc
		JOIN products p ON p.id = pc.product_id
		JOIN shops s ON s.id = p.shop_id
		WHERE pc.id = $1 AND s.user_id = $2`,
		competitorID, userID,
	)
	return scanProductCompetitor(row)
}

func (r *productCompetitorsPg) Update(ctx context.Context, userID, competitorID uuid.UUID, input CompetitorUpdateInput) (*domain.ProductCompetitor, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE product_competitors pc
		SET competitor_url = $3,
		    normalized_competitor_url = $4,
		    ozon_public_product_id = $5,
		    ozon_sku = $6,
		    last_price = NULL,
		    last_availability = 'unknown',
		    last_checked_at = NULL,
		    last_error_code = '',
		    last_status = 'pending',
		    updated_at = NOW()
		FROM products p
		JOIN shops s ON s.id = p.shop_id
		WHERE pc.product_id = p.id
		  AND pc.id = $1
		  AND s.user_id = $2
		RETURNING pc.id, pc.product_id, pc.marketplace::text, pc.source, pc.competitor_url,
		          pc.normalized_competitor_url, pc.ozon_public_product_id, pc.ozon_sku,
		          pc.last_price::float8, pc.last_availability, pc.last_checked_at,
		          pc.last_error_code, pc.last_status, pc.created_at, pc.updated_at`,
		competitorID, userID, input.CompetitorURL, input.NormalizedCompetitorURL,
		input.OzonPublicProductID, input.OzonSKU,
	)
	competitor, err := scanProductCompetitor(row)
	if isUniqueViolation(err) {
		return nil, ErrDuplicate
	}
	return competitor, err
}

func (r *productCompetitorsPg) Delete(ctx context.Context, userID, competitorID uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM product_competitors pc
		USING products p, shops s
		WHERE pc.product_id = p.id
		  AND p.shop_id = s.id
		  AND pc.id = $1
		  AND s.user_id = $2`,
		competitorID, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *productCompetitorsPg) SaveCheckResult(ctx context.Context, competitorID uuid.UUID, result CompetitorCheckResult) (*domain.ProductCompetitor, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now().UTC()
	}

	row := tx.QueryRow(ctx, `
		UPDATE product_competitors
		SET last_price = $2,
		    last_availability = $3,
		    last_checked_at = $4,
		    last_error_code = $5,
		    last_status = $6,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, product_id, marketplace::text, source, competitor_url,
		          normalized_competitor_url, ozon_public_product_id, ozon_sku,
		          last_price::float8, last_availability, last_checked_at,
		          last_error_code, last_status, created_at, updated_at`,
		competitorID, result.Price, result.Availability, result.CheckedAt,
		result.ErrorCode, result.Status,
	)
	competitor, err := scanProductCompetitor(row)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO competitor_price_snapshots
			(competitor_id, price, availability, checked_at, status, error_code, raw_source)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		competitorID, result.Price, result.Availability, result.CheckedAt,
		result.Status, result.ErrorCode, result.RawSource,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return competitor, nil
}

func (r *productCompetitorsPg) LatestFreshPrice(ctx context.Context, userID, productID uuid.UUID, maxAge time.Duration) (*float64, error) {
	row := r.db.QueryRow(ctx, `
		SELECT pc.last_price::float8
		FROM product_competitors pc
		JOIN products p ON p.id = pc.product_id
		JOIN shops s ON s.id = p.shop_id
		WHERE pc.product_id = $1
		  AND s.user_id = $2
		  AND pc.last_status = 'ok'
		  AND pc.last_price IS NOT NULL
		  AND pc.last_checked_at >= NOW() - ($3 * INTERVAL '1 second')
		ORDER BY pc.last_price ASC, pc.last_checked_at DESC
		LIMIT 1`,
		productID, userID, int(maxAge.Seconds()),
	)
	var price float64
	if err := row.Scan(&price); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &price, nil
}

func scanProductCompetitor(row scannable) (*domain.ProductCompetitor, error) {
	var item domain.ProductCompetitor
	if err := row.Scan(
		&item.ID, &item.ProductID, &item.Marketplace, &item.Source,
		&item.CompetitorURL, &item.NormalizedCompetitorURL,
		&item.OzonPublicProductID, &item.OzonSKU,
		&item.LastPrice, &item.LastAvailability, &item.LastCheckedAt,
		&item.LastErrorCode, &item.LastStatus, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}
