package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type productsPg struct{ db *pgxpool.Pool }

func NewProductsRepository(db *pgxpool.Pool) ProductsRepository { return &productsPg{db: db} }

func (r *productsPg) Create(ctx context.Context, userID uuid.UUID, input ProductCreateInput) (*domain.Product, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO products
			(shop_id, external_sku, name, current_price, currency, status,
			 min_price, max_price, cost_price, created_at, updated_at)
		SELECT $2,$3,$4,$5,$6,$7,$8,$9,$10,NOW(),NOW()
		FROM shops
		WHERE id=$2 AND user_id=$1
		RETURNING id, shop_id, external_sku, name, current_price::float8, currency, status,
		          min_price::float8, max_price::float8, co
				  st_price::float8,
		          stock_count, rating::float8, reviews_count, last_synced_at,
		          false, created_at, updated_at`,
		userID, input.ShopID, input.ExternalSKU, input.Name, input.CurrentPrice, input.Currency,
		input.Status, input.MinPrice, input.MaxPrice, input.CostPrice,
	)
	product, err := scanProduct(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return product, nil
}

func (r *productsPg) List(ctx context.Context, userID uuid.UUID, filter ProductListFilter) (*ProductListResult, error) {
	where, args := productWhere(userID, filter)

	countSQL := "SELECT COUNT(*) FROM products p JOIN shops s ON s.id=p.shop_id " + where
	var total int
	if err := r.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, err
	}

	offset := (filter.Page - 1) * filter.PerPage
	args = append(args, filter.PerPage, offset)
	limitPos := len(args) - 1
	offsetPos := len(args)

	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.shop_id, p.external_sku, p.name, p.current_price::float8,
		       p.currency, p.status, p.min_price::float8, p.max_price::float8,
		       p.cost_price::float8, p.stock_count, p.rating::float8,
		       p.reviews_count, p.last_synced_at,
		       EXISTS (
		         SELECT 1 FROM strategy_assignments sa WHERE sa.product_id=p.id
		       ) AS has_strategy,
		       p.created_at, p.updated_at
		FROM products p
		JOIN shops s ON s.id=p.shop_id
		`+where+`
		ORDER BY p.updated_at DESC, p.id DESC
		LIMIT $`+fmt.Sprint(limitPos)+` OFFSET $`+fmt.Sprint(offsetPos),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*domain.Product, 0, filter.PerPage)
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, product)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &ProductListResult{Items: items, Total: total, Page: filter.Page, PerPage: filter.PerPage}, nil
}

func (r *productsPg) GetByIDForUser(ctx context.Context, userID, productID uuid.UUID) (*domain.Product, error) {
	row := r.db.QueryRow(ctx, `
		SELECT p.id, p.shop_id, p.external_sku, p.name, p.current_price::float8,
		       p.currency, p.status, p.min_price::float8, p.max_price::float8,
		       p.cost_price::float8, p.stock_count, p.rating::float8,
		       p.reviews_count, p.last_synced_at,
		       EXISTS (
		         SELECT 1 FROM strategy_assignments sa WHERE sa.product_id=p.id
		       ) AS has_strategy,
		       p.created_at, p.updated_at
		FROM products p
		JOIN shops s ON s.id=p.shop_id
		WHERE p.id=$1 AND s.user_id=$2`,
		productID, userID,
	)
	return scanProduct(row)
}

func (r *productsPg) PatchPrices(ctx context.Context, userID, productID uuid.UUID, patch ProductPricePatch) (*domain.Product, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE products p
		SET min_price=CASE WHEN $3 THEN $4 ELSE min_price END,
		    max_price=CASE WHEN $5 THEN $6 ELSE max_price END,
		    cost_price=CASE WHEN $7 THEN $8 ELSE cost_price END,
		    updated_at=NOW()
		FROM shops s
		WHERE p.shop_id=s.id AND p.id=$1 AND s.user_id=$2
		RETURNING p.id, p.shop_id, p.external_sku, p.name, p.current_price::float8,
		          p.currency, p.status, p.min_price::float8, p.max_price::float8,
		          p.cost_price::float8, p.stock_count, p.rating::float8,
		          p.reviews_count, p.last_synced_at,
		          EXISTS (
		            SELECT 1 FROM strategy_assignments sa WHERE sa.product_id=p.id
		          ) AS has_strategy,
		          p.created_at, p.updated_at`,
		productID, userID,
		patch.MinPrice.Set, patch.MinPrice.Value,
		patch.MaxPrice.Set, patch.MaxPrice.Value,
		patch.CostPrice.Set, patch.CostPrice.Value,
	)
	return scanProduct(row)
}

func (r *productsPg) UpsertImported(ctx context.Context, shopID uuid.UUID, rows []ProductImportRow) (ImportUpsertResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return ImportUpsertResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var result ImportUpsertResult
	now := time.Now().UTC()
	for _, item := range rows {
		var inserted bool
		err := tx.QueryRow(ctx, `
			INSERT INTO products
				(shop_id, external_sku, name, current_price, currency, status,
				 stock_count, last_synced_at, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8,$8)
			ON CONFLICT (shop_id, external_sku) DO UPDATE SET
				name=EXCLUDED.name,
				current_price=EXCLUDED.current_price,
				currency=EXCLUDED.currency,
				status=EXCLUDED.status,
				stock_count=EXCLUDED.stock_count,
				last_synced_at=EXCLUDED.last_synced_at,
				updated_at=EXCLUDED.updated_at
			RETURNING xmax = 0`,
			shopID, item.ExternalSKU, item.Name, item.CurrentPrice, item.Currency,
			item.Status, item.StockCount, now,
		).Scan(&inserted)
		if err != nil {
			return ImportUpsertResult{}, err
		}
		if inserted {
			result.Added++
		} else {
			result.Updated++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ImportUpsertResult{}, err
	}
	return result, nil
}

func productWhere(userID uuid.UUID, filter ProductListFilter) (string, []any) {
	clauses := []string{"s.user_id=$1"}
	args := []any{userID}
	next := 2
	if filter.ShopID != nil {
		clauses = append(clauses, fmt.Sprintf("p.shop_id=$%d", next))
		args = append(args, *filter.ShopID)
		next++
	}
	if filter.Status != "" {
		clauses = append(clauses, fmt.Sprintf("p.status=$%d", next))
		args = append(args, filter.Status)
		next++
	}
	if filter.Query != "" {
		clauses = append(clauses, fmt.Sprintf("(p.name ILIKE $%d OR p.external_sku ILIKE $%d)", next, next))
		args = append(args, "%"+filter.Query+"%")
		next++
	}
	if filter.HasStrategy != nil {
		exists := "EXISTS (SELECT 1 FROM strategy_assignments sa WHERE sa.product_id=p.id)"
		if *filter.HasStrategy {
			clauses = append(clauses, exists)
		} else {
			clauses = append(clauses, "NOT "+exists)
		}
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func scanProduct(row scannable) (*domain.Product, error) {
	var p domain.Product
	err := row.Scan(
		&p.ID, &p.ShopID, &p.ExternalSKU, &p.Name, &p.CurrentPrice,
		&p.Currency, &p.Status, &p.MinPrice, &p.MaxPrice, &p.CostPrice,
		&p.StockCount, &p.Rating, &p.ReviewsCount, &p.LastSyncedAt,
		&p.HasStrategy, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
