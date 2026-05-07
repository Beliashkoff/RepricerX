package audit

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

type Service struct {
	changes repository.PriceChangesRepository
	now     func() time.Time
}

func New(changes repository.PriceChangesRepository) *Service {
	return &Service{changes: changes, now: time.Now}
}

func (s *Service) ListChanges(ctx context.Context, userID uuid.UUID) ([]*domain.PriceChange, error) {
	return s.changes.ListForUser(ctx, userID, 200)
}

func (s *Service) Summary(ctx context.Context, userID uuid.UUID) (*domain.PriceChangeSummary, error) {
	until := s.now().UTC()
	since := until.AddDate(0, 0, -30)
	return s.changes.SummaryForUser(ctx, userID, since, until)
}

func (s *Service) ExportCSV(ctx context.Context, userID uuid.UUID) ([]byte, error) {
	changes, err := s.changes.ExportForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"id", "created_at", "product", "old_price", "new_price", "target_price", "status", "reason"})
	for _, c := range changes {
		_ = w.Write([]string{
			c.ID.String(),
			c.CreatedAt.Format(time.RFC3339),
			csvSafeText(c.ProductName),
			fmt.Sprintf("%.2f", c.OldPrice),
			fmt.Sprintf("%.2f", c.NewPrice),
			fmt.Sprintf("%.2f", c.TargetPrice),
			csvSafeText(c.Status),
			csvSafeText(c.Reason),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func csvSafeText(value string) string {
	if value == "" {
		return value
	}
	if value[0] == '\t' || value[0] == '\r' || value[0] == '\n' {
		return "'" + value
	}
	trimmed := value
	for len(trimmed) > 0 {
		switch trimmed[0] {
		case ' ', '\t', '\r', '\n':
			trimmed = trimmed[1:]
		default:
			goto check
		}
	}
check:
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}
