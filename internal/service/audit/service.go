package audit

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

// ErrInvalidFilter — невалидные параметры фильтра/пагинации.
// HTTP-слой мапит её в 400 bad_request.
var ErrInvalidFilter = errors.New("invalid filter")

const (
	defaultPerPage = 50
	maxPerPage     = 200
	listHorizon    = 180 * 24 * time.Hour // ретеншн журнала
	summaryWindow  = 30 * 24 * time.Hour  // дефолтное окно для summary
)

// ListFilter — параметры запросов журнала на сервисном слое.
// HTTP-слой парсит query, заполняет эту структуру и передаёт в Service.
// Сервис задаёт дефолты (горизонт 180д для списка/экспорта, 30д для summary)
// и валидирует, после чего проксирует в repository.PriceChangeFilter.
type ListFilter struct {
	ShopID      *uuid.UUID
	ProductID   *uuid.UUID
	ExternalSKU string
	Status      string
	From        time.Time
	Until       time.Time
	Page        int
	PerPage     int
	SortDir     string
}

type Service struct {
	changes repository.PriceChangesRepository
	now     func() time.Time
}

func New(changes repository.PriceChangesRepository) *Service {
	return &Service{changes: changes, now: time.Now}
}

// ListChanges возвращает страницу журнала и общее число подходящих записей.
func (s *Service) ListChanges(ctx context.Context, userID uuid.UUID, f ListFilter) ([]*domain.PriceChange, int, error) {
	rf, err := s.toRepoFilter(f, listHorizon)
	if err != nil {
		return nil, 0, err
	}
	return s.changes.ListForUser(ctx, userID, rf)
}

// Summary считает агрегированные метрики за параметризуемый период
// (по умолчанию — последние 30 дней).
func (s *Service) Summary(ctx context.Context, userID uuid.UUID, f ListFilter) (*domain.PriceChangeSummary, error) {
	rf, err := s.toRepoFilter(f, summaryWindow)
	if err != nil {
		return nil, err
	}
	// Page/PerPage в summary не имеют смысла — обнуляем.
	rf.Page, rf.PerPage = 0, 0
	return s.changes.SummaryForUser(ctx, userID, rf)
}

// ExportCSV возвращает CSV-выгрузку, применяя те же фильтры, что и список.
// Безопасность: значения с ведущими =,+,-,@ префиксируются `'` от формула-инъекций.
func (s *Service) ExportCSV(ctx context.Context, userID uuid.UUID, f ListFilter) ([]byte, error) {
	rf, err := s.toRepoFilter(f, listHorizon)
	if err != nil {
		return nil, err
	}
	rf.Page, rf.PerPage = 0, 0
	changes, err := s.changes.ExportForUser(ctx, userID, rf)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"id", "created_at", "product", "old_price", "new_price", "target_price", "status", "constraint_hit", "reason"})
	for _, c := range changes {
		constraint := ""
		if c.ConstraintHit != nil {
			constraint = *c.ConstraintHit
		}
		_ = w.Write([]string{
			c.ID.String(),
			c.CreatedAt.Format(time.RFC3339),
			csvSafeText(c.ProductName),
			fmt.Sprintf("%.2f", c.OldPrice),
			fmt.Sprintf("%.2f", c.NewPrice),
			fmt.Sprintf("%.2f", c.TargetPrice),
			csvSafeText(c.Status),
			csvSafeText(constraint),
			csvSafeText(c.Reason),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// toRepoFilter валидирует входные параметры и подставляет дефолты.
// defaultWindow — длительность окна, если From/Until не заданы.
func (s *Service) toRepoFilter(f ListFilter, defaultWindow time.Duration) (repository.PriceChangeFilter, error) {
	rf := repository.PriceChangeFilter{
		ShopID:      f.ShopID,
		ProductID:   f.ProductID,
		ExternalSKU: strings.TrimSpace(f.ExternalSKU),
		Status:      f.Status,
		From:        f.From,
		Until:       f.Until,
		Page:        f.Page,
		PerPage:     f.PerPage,
		SortDir:     f.SortDir,
	}

	if f.Status != "" {
		switch f.Status {
		case "success", "failed", "skipped":
		default:
			return repository.PriceChangeFilter{}, fmt.Errorf("%w: status", ErrInvalidFilter)
		}
	}

	now := s.now().UTC()
	if rf.Until.IsZero() {
		rf.Until = now
	}
	if rf.From.IsZero() {
		rf.From = rf.Until.Add(-defaultWindow)
	}
	if rf.From.After(rf.Until) {
		return repository.PriceChangeFilter{}, fmt.Errorf("%w: from > to", ErrInvalidFilter)
	}

	if rf.Page < 1 {
		rf.Page = 1
	}
	if rf.PerPage < 1 {
		rf.PerPage = defaultPerPage
	}
	if rf.PerPage > maxPerPage {
		rf.PerPage = maxPerPage
	}
	if rf.SortDir != "asc" && rf.SortDir != "desc" {
		rf.SortDir = "desc"
	}
	return rf, nil
}

// csvSafeText — защита от формула-инъекций в Excel/Sheets:
// значения с ведущими =,+,-,@ (после ведущих whitespace) префиксируются одинарной кавычкой.
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
