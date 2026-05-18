package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type productsPg struct{ db *pgxpool.Pool }

func NewProductsRepository(db *pgxpool.Pool) ProductsRepository { return &productsPg{db: db} }

const productColumns = `p.id, p.shop_id, p.external_sku, p.vendor_code, p.name, p.current_price::float8,
	       p.currency, p.status, p.min_price::float8, p.max_price::float8,
	       p.cost_price::float8, p.stock_count, p.rating::float8,
	       p.reviews_count, p.last_synced_at,
	       EXISTS (
	         SELECT 1 FROM strategy_assignments sa WHERE sa.product_id=p.id
	       ) AS has_strategy,
	       p.created_at, p.updated_at`

func (r *productsPg) Create(ctx context.Context, userID uuid.UUID, input ProductCreateInput) (*domain.Product, error) {
	row := r.db.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO products
				(shop_id, external_sku, name, current_price, currency, status,
				 min_price, max_price, cost_price, created_at, updated_at)
			SELECT $2,$3,$4,$5,$6,$7,$8,$9,$10,NOW(),NOW()
			FROM shops
			WHERE id=$2 AND user_id=$1
			RETURNING id
		)
		SELECT `+productColumns+`
		FROM products p
		WHERE p.id = (SELECT id FROM ins)`,
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
		SELECT `+productColumns+`
		FROM products p
		JOIN shops s ON s.id=p.shop_id
		`+where+`
		`+productOrderBy(filter)+`
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
		SELECT `+productColumns+`
		FROM products p
		JOIN shops s ON s.id=p.shop_id
		WHERE p.id=$1 AND s.user_id=$2`,
		productID, userID,
	)
	return scanProduct(row)
}

func (r *productsPg) PatchPrices(ctx context.Context, userID, productID uuid.UUID, patch ProductPricePatch) (*domain.Product, error) {
	row := r.db.QueryRow(ctx, `
		WITH upd AS (
			UPDATE products p
			SET min_price=CASE WHEN $3 THEN $4 ELSE min_price END,
			    max_price=CASE WHEN $5 THEN $6 ELSE max_price END,
			    cost_price=CASE WHEN $7 THEN $8 ELSE cost_price END,
			    updated_at=NOW()
			FROM shops s
			WHERE p.shop_id=s.id AND p.id=$1 AND s.user_id=$2
			RETURNING p.id
		)
		SELECT `+productColumns+`
		FROM products p
		WHERE p.id = (SELECT id FROM upd)`,
		productID, userID,
		patch.MinPrice.Set, patch.MinPrice.Value,
		patch.MaxPrice.Set, patch.MaxPrice.Value,
		patch.CostPrice.Set, patch.CostPrice.Value,
	)
	return scanProduct(row)
}

func (r *productsPg) UpsertImported(ctx context.Context, shopID uuid.UUID, rows []ProductImportRow) (ImportUpsertResult, error) {
	if len(rows) == 0 {
		return ImportUpsertResult{}, nil
	}

	externalSKUs := make([]string, len(rows))
	vendorCodes := make([]*string, len(rows))
	names := make([]string, len(rows))
	prices := make([]float64, len(rows))
	currencies := make([]string, len(rows))
	statuses := make([]string, len(rows))
	stockCounts := make([]int32, len(rows))

	for i, row := range rows {
		externalSKUs[i] = row.ExternalSKU
		vendorCodes[i] = row.VendorCode
		names[i] = row.Name
		prices[i] = row.CurrentPrice
		currencies[i] = row.Currency
		statuses[i] = row.Status
		stockCounts[i] = int32(row.StockCount)
	}

	var added, updated int
	err := r.db.QueryRow(ctx, `
		WITH upsert AS (
			INSERT INTO products
				(shop_id, external_sku, vendor_code, name, current_price, currency, status,
				 stock_count, last_synced_at, created_at, updated_at)
			SELECT
				$1::uuid,
				unnest($2::text[]),
				unnest($3::text[]),
				unnest($4::text[]),
				unnest($5::float8[]),
				unnest($6::text[]),
				unnest($7::text[])::product_status,
				unnest($8::int[]),
				NOW(), NOW(), NOW()
			ON CONFLICT (shop_id, external_sku) DO UPDATE SET
				vendor_code    = COALESCE(EXCLUDED.vendor_code, products.vendor_code),
				name           = EXCLUDED.name,
				current_price  = EXCLUDED.current_price,
				currency       = EXCLUDED.currency,
				status         = EXCLUDED.status,
				stock_count    = EXCLUDED.stock_count,
				last_synced_at = NOW(),
				updated_at     = NOW()
			RETURNING (xmax = 0) AS is_insert
		)
		SELECT
			COUNT(*) FILTER (WHERE is_insert)     AS added,
			COUNT(*) FILTER (WHERE NOT is_insert) AS updated
		FROM upsert`,
		shopID, externalSKUs, vendorCodes, names, prices, currencies, statuses, stockCounts,
	).Scan(&added, &updated)
	if err != nil {
		return ImportUpsertResult{}, err
	}
	return ImportUpsertResult{Added: added, Updated: updated}, nil
}

func (r *productsPg) SoftDelete(ctx context.Context, userID, productID uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE products
		SET status = 'archived'::product_status, updated_at = NOW()
		WHERE id = $1
		  AND shop_id IN (SELECT id FROM shops WHERE user_id = $2)
		  AND status != 'archived'::product_status`,
		productID, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *productsPg) BulkPatch(ctx context.Context, userID uuid.UUID, patches []BulkPricePatch) (int, error) {
	if len(patches) == 0 {
		return 0, nil
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	batch := &pgx.Batch{}
	for _, p := range patches {
		batch.Queue(`
			UPDATE products
			SET min_price  = CASE WHEN $3 THEN $4 ELSE min_price  END,
			    max_price  = CASE WHEN $5 THEN $6 ELSE max_price  END,
			    cost_price = CASE WHEN $7 THEN $8 ELSE cost_price END,
			    updated_at = NOW()
			FROM shops s
			WHERE products.shop_id = s.id
			  AND products.id = $1
			  AND s.user_id = $2`,
			p.ProductID, userID,
			p.MinPrice.Set, p.MinPrice.Value,
			p.MaxPrice.Set, p.MaxPrice.Value,
			p.CostPrice.Set, p.CostPrice.Value,
		)
	}
	br := tx.SendBatch(ctx, batch)

	updated := 0
	for range patches {
		tag, err := br.Exec()
		if err != nil {
			_ = br.Close()
			if isCheckViolation(err) {
				return 0, ErrConstraintViolation
			}
			return 0, err
		}
		updated += int(tag.RowsAffected())
	}
	if err := br.Close(); err != nil {
		if isCheckViolation(err) {
			return 0, ErrConstraintViolation
		}
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		if isCheckViolation(err) {
			return 0, ErrConstraintViolation
		}
		return 0, err
	}
	return updated, nil
}

func (r *productsPg) ExportForUser(ctx context.Context, userID uuid.UUID, filter ProductListFilter) ([]*domain.Product, error) {
	where, args := productWhere(userID, filter)
	rows, err := r.db.Query(ctx, `
		SELECT `+productColumns+`
		FROM products p
		JOIN shops s ON s.id=p.shop_id
		`+where+`
		`+productOrderBy(filter)+`
		LIMIT 10000`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*domain.Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
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
		clauses = append(clauses, fmt.Sprintf("(p.name ILIKE $%d OR p.external_sku ILIKE $%d OR p.vendor_code ILIKE $%d)", next, next, next))
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
	if filter.PriceFrom != nil {
		clauses = append(clauses, fmt.Sprintf("p.current_price >= $%d", next))
		args = append(args, *filter.PriceFrom)
		next++
	}
	if filter.PriceTo != nil {
		clauses = append(clauses, fmt.Sprintf("p.current_price <= $%d", next))
		args = append(args, *filter.PriceTo)
		next++
	}
	_ = next
	return "WHERE " + strings.Join(clauses, " AND "), args
}

// productOrderBy строит безопасный ORDER BY по whitelist полей.
func productOrderBy(filter ProductListFilter) string {
	dir := "DESC"
	if strings.ToLower(filter.SortDir) == "asc" {
		dir = "ASC"
	}
	switch filter.SortBy {
	case "name":
		return "ORDER BY p.name " + dir + ", p.id DESC"
	case "current_price":
		return "ORDER BY p.current_price " + dir + ", p.id DESC"
	default:
		return "ORDER BY p.updated_at " + dir + ", p.id DESC"
	}
}

func scanProduct(row scannable) (*domain.Product, error) {
	var p domain.Product
	err := row.Scan(
		&p.ID, &p.ShopID, &p.ExternalSKU, &p.VendorCode, &p.Name, &p.CurrentPrice,
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

func isCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514"
}
