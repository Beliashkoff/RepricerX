package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type strategyAssignmentsPg struct{ db *pgxpool.Pool }

func NewStrategyAssignmentsRepository(db *pgxpool.Pool) StrategyAssignmentsRepository {
	return &strategyAssignmentsPg{db: db}
}

// AssignToProducts назначает стратегию на список товаров пользователя.
// Если товар уже имеет другую стратегию — перезаписывает (ON CONFLICT DO UPDATE).
// Ownership проверяется через JOIN shops — чужие товары молча игнорируются.
func (r *strategyAssignmentsPg) AssignToProducts(ctx context.Context, userID, strategyID uuid.UUID, productIDs []uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO strategy_assignments (strategy_id, product_id)
		SELECT $1, p.id
		FROM products p
		JOIN shops s ON s.id = p.shop_id
		WHERE p.id = ANY($2::uuid[])
		  AND s.user_id = $3
		  AND EXISTS (SELECT 1 FROM strategies WHERE id=$1 AND user_id=$3)
		ON CONFLICT (product_id) WHERE product_id IS NOT NULL
		DO UPDATE SET strategy_id = EXCLUDED.strategy_id`,
		strategyID, productIDs, userID,
	)
	return err
}

// UnassignFromProducts снимает стратегию с указанных товаров пользователя.
func (r *strategyAssignmentsPg) UnassignFromProducts(ctx context.Context, userID, strategyID uuid.UUID, productIDs []uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM strategy_assignments sa
		USING products p, shops s
		WHERE sa.product_id = p.id
		  AND p.shop_id = s.id
		  AND sa.strategy_id = $1
		  AND sa.product_id = ANY($2::uuid[])
		  AND s.user_id = $3`,
		strategyID, productIDs, userID,
	)
	return err
}

// ListProductIDsByStrategy возвращает список product_id, назначенных на стратегию.
func (r *strategyAssignmentsPg) ListProductIDsByStrategy(ctx context.Context, userID, strategyID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `
		SELECT sa.product_id
		FROM strategy_assignments sa
		JOIN strategies st ON st.id = sa.strategy_id
		WHERE sa.strategy_id = $1 AND st.user_id = $2`,
		strategyID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
