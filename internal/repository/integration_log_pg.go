package repository

import (
	"context"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type integrationLogPg struct{ db *pgxpool.Pool }

func NewIntegrationLogRepository(db *pgxpool.Pool) IntegrationLogRepository {
	return &integrationLogPg{db: db}
}

func (r *integrationLogPg) Create(ctx context.Context, e *domain.IntegrationLogEntry) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO integration_log
			(id, shop_id, operation, http_status, error_text, correlation_id, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		e.ID, e.ShopID, e.Operation, e.HTTPStatus, e.ErrorText, e.CorrelationID, e.CreatedAt,
	)
	return err
}

func (r *integrationLogPg) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM integration_log WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
