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
	"github.com/jackc/pgx/v5/pgxpool"
)

// exportHardCap — защитный потолок строк, отдаваемых наружу в CSV.
// При большем объёме клиент должен сужать фильтр (по магазину/датам/статусу).
const exportHardCap = 10000

type priceChangesPg struct{ db *pgxpool.Pool }

func NewPriceChangesRepository(db *pgxpool.Pool) PriceChangesRepository {
	return &priceChangesPg{db: db}
}

func (r *priceChangesPg) Create(ctx context.Context, c PriceChangeCreate) error {
	// Маппим item.status (plan_item_status) → price_change_log.status (тоже plan_item_status):
	// 'dispatched' → 'applied' (исторический статус для read-API)
	// 'failed'/'skipped' остаются как есть.
	dbStatus := c.Status
	if dbStatus == domain.PlanItemStatusDispatched {
		dbStatus = domain.PlanItemStatusApplied
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO price_change_log (
			shop_id, product_id, strategy_id,
			old_price, new_price, target_price,
			reason, constraint_hit, status, correlation_id
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7, $8, $9::plan_item_status, $10
		)`,
		c.ShopID, c.ProductID, c.StrategyID,
		c.OldPrice, c.NewPrice, c.TargetPrice,
		c.Reason, c.ConstraintHit, dbStatus, c.CorrelationID,
	)
	return err
}

func (r *priceChangesPg) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM price_change_log WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *priceChangesPg) ListForUser(ctx context.Context, userID uuid.UUID, f PriceChangeFilter) ([]*domain.PriceChange, int, error) {
	where, args := buildAuditWhere(userID, f)

	// total — отдельный COUNT по тому же WHERE, без JOIN на products.
	countSQL := "SELECT COUNT(*) FROM price_change_log pcl JOIN shops sh ON sh.id=pcl.shop_id " + where
	var total int
	if err := r.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []*domain.PriceChange{}, 0, nil
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	perPage := f.PerPage
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 500 {
		perPage = 500
	}
	offset := (page - 1) * perPage

	dir := normalizeSortDir(f.SortDir)
	limitPos := len(args) + 1
	offsetPos := len(args) + 2
	listSQL := fmt.Sprintf(`
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
		%s
		ORDER BY pcl.created_at %s, pcl.id %s
		LIMIT $%d OFFSET $%d`, where, dir, dir, limitPos, offsetPos)

	queryArgs := append(append([]any{}, args...), perPage, offset)
	rows, err := r.db.Query(ctx, listSQL, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]*domain.PriceChange, 0, perPage)
	for rows.Next() {
		item, err := scanPriceChange(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *priceChangesPg) ExportForUser(ctx context.Context, userID uuid.UUID, f PriceChangeFilter) ([]*domain.PriceChange, error) {
	where, args := buildAuditWhere(userID, f)
	dir := normalizeSortDir(f.SortDir)

	limitPos := len(args) + 1
	listSQL := fmt.Sprintf(`
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
		%s
		ORDER BY pcl.created_at %s, pcl.id %s
		LIMIT $%d`, where, dir, dir, limitPos)

	queryArgs := append(append([]any{}, args...), exportHardCap)
	rows, err := r.db.Query(ctx, listSQL, queryArgs...)
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

func (r *priceChangesPg) SummaryForUser(ctx context.Context, userID uuid.UUID, f PriceChangeFilter) (*domain.PriceChangeSummary, error) {
	where, args := buildAuditWhere(userID, f)
	sql := `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE pcl.status='applied')::int,
			COUNT(*) FILTER (WHERE pcl.status='failed')::int,
			COALESCE(AVG(((pcl.new_price - pcl.old_price) / NULLIF(pcl.old_price, 0)) * 100), 0)::float8
		FROM price_change_log pcl
		JOIN shops sh ON sh.id=pcl.shop_id ` + where

	row := r.db.QueryRow(ctx, sql, args...)
	summary := &domain.PriceChangeSummary{PeriodStart: f.From, PeriodEnd: f.Until}
	if err := row.Scan(&summary.TotalUpdates, &summary.SuccessfulUpdates, &summary.FailedUpdates, &summary.AvgChangePct); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return summary, nil
		}
		return nil, err
	}
	return summary, nil
}

// buildAuditWhere собирает WHERE-блок и список args на основе фильтра.
// Первый плейсхолдер всегда $1 = userID; остальные добавляются по мере необходимости.
func buildAuditWhere(userID uuid.UUID, f PriceChangeFilter) (string, []any) {
	conds := []string{"sh.user_id=$1"}
	args := []any{userID}

	if !f.From.IsZero() {
		args = append(args, f.From)
		conds = append(conds, fmt.Sprintf("pcl.created_at >= $%d", len(args)))
	}
	if !f.Until.IsZero() {
		args = append(args, f.Until)
		conds = append(conds, fmt.Sprintf("pcl.created_at <= $%d", len(args)))
	}
	if f.ShopID != nil {
		args = append(args, *f.ShopID)
		conds = append(conds, fmt.Sprintf("pcl.shop_id=$%d", len(args)))
	}
	if f.ProductID != nil {
		args = append(args, *f.ProductID)
		conds = append(conds, fmt.Sprintf("pcl.product_id=$%d", len(args)))
	}
	if sku := strings.TrimSpace(f.ExternalSKU); sku != "" {
		args = append(args, escapeLike(sku))
		conds = append(conds, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM products p2 WHERE p2.id=pcl.product_id AND p2.shop_id=pcl.shop_id AND p2.external_sku ILIKE $%d ESCAPE '\\')",
			len(args),
		))
	}
	if status := mapPublicStatusToDB(f.Status); status != "" {
		args = append(args, status)
		conds = append(conds, fmt.Sprintf("pcl.status=$%d::plan_item_status", len(args)))
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// mapPublicStatusToDB переводит публичный статус (success/failed/skipped) в БД-значение
// (applied/failed/skipped). Пустая строка → пустая (фильтр не применяется).
func mapPublicStatusToDB(s string) string {
	switch s {
	case "success":
		return "applied"
	case "failed":
		return "failed"
	case "skipped":
		return "skipped"
	default:
		return ""
	}
}

// escapeLike оборачивает строку для безопасного использования в LIKE/ILIKE
// с экранированием спецсимволов LIKE (% _ \) и обрамляет % с обеих сторон
// для подстрочного поиска. Backslash используется как ESCAPE-символ в SQL.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}

func normalizeSortDir(dir string) string {
	if strings.EqualFold(dir, "asc") {
		return "ASC"
	}
	return "DESC"
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
