package competitor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrProductNotFound     = errors.New("product not found")
	ErrCompetitorNotFound  = errors.New("competitor not found")
	ErrInvalidTarget       = errors.New("invalid competitor target")
	ErrDuplicateCompetitor = errors.New("duplicate competitor")
	ErrRefreshFailed       = errors.New("competitor refresh failed")
)

type OzonPriceLookup interface {
	Lookup(ctx context.Context, target OzonTarget) (LookupResult, error)
}

type OzonTarget struct {
	PublicProductID string
	URL             string
}

type LookupResult struct {
	Price        *float64
	Availability string
	Source       string
}

type Service struct {
	repo       repository.ProductCompetitorsRepository
	ozon       OzonPriceLookup
	now        func() time.Time
	maxURLSize int
}

func New(repo repository.ProductCompetitorsRepository, ozon OzonPriceLookup) *Service {
	if ozon == nil {
		ozon = NewHTTPBasedOzonLookup()
	}
	return &Service{
		repo:       repo,
		ozon:       ozon,
		now:        func() time.Time { return time.Now().UTC() },
		maxURLSize: 2048,
	}
}

type CreateInput struct {
	ProductID uuid.UUID
	Target    string
}

type UpdateInput struct {
	Target string
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, input CreateInput) (*domain.ProductCompetitor, error) {
	target, err := normalizeOzonTarget(input.Target, s.maxURLSize)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.Create(ctx, userID, repository.CompetitorCreateInput{
		ProductID:               input.ProductID,
		Marketplace:             domain.MarketplaceOzon,
		Source:                  "public_ozon",
		CompetitorURL:           target.URL,
		NormalizedCompetitorURL: target.normalized,
		OzonPublicProductID:     &target.PublicProductID,
	})
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrProductNotFound
	}
	if errors.Is(err, repository.ErrDuplicate) {
		return nil, ErrDuplicateCompetitor
	}
	if err != nil {
		return nil, fmt.Errorf("competitor create: %w", err)
	}
	return item, nil
}

func (s *Service) List(ctx context.Context, userID, productID uuid.UUID) ([]*domain.ProductCompetitor, error) {
	items, err := s.repo.ListByProduct(ctx, userID, productID)
	if err != nil {
		return nil, fmt.Errorf("competitor list: %w", err)
	}
	return items, nil
}

func (s *Service) Update(ctx context.Context, userID, competitorID uuid.UUID, input UpdateInput) (*domain.ProductCompetitor, error) {
	target, err := normalizeOzonTarget(input.Target, s.maxURLSize)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.Update(ctx, userID, competitorID, repository.CompetitorUpdateInput{
		CompetitorURL:           target.URL,
		NormalizedCompetitorURL: target.normalized,
		OzonPublicProductID:     &target.PublicProductID,
	})
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrCompetitorNotFound
	}
	if errors.Is(err, repository.ErrDuplicate) {
		return nil, ErrDuplicateCompetitor
	}
	if err != nil {
		return nil, fmt.Errorf("competitor update: %w", err)
	}
	return item, nil
}

func (s *Service) Delete(ctx context.Context, userID, competitorID uuid.UUID) error {
	err := s.repo.Delete(ctx, userID, competitorID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrCompetitorNotFound
	}
	if err != nil {
		return fmt.Errorf("competitor delete: %w", err)
	}
	return nil
}

func (s *Service) Refresh(ctx context.Context, userID, competitorID uuid.UUID) (*domain.ProductCompetitor, error) {
	item, err := s.repo.GetByIDForUser(ctx, userID, competitorID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrCompetitorNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("competitor get: %w", err)
	}
	target := OzonTarget{URL: item.CompetitorURL}
	if item.OzonPublicProductID != nil {
		target.PublicProductID = *item.OzonPublicProductID
	}
	result, lookupErr := s.ozon.Lookup(ctx, target)
	check := repository.CompetitorCheckResult{
		Availability: domain.CompetitorAvailabilityUnknown,
		Status:       domain.CompetitorStatusFailed,
		ErrorCode:    safeLookupErrorCode(lookupErr),
		RawSource:    "public_ozon",
		CheckedAt:    s.now(),
	}
	if lookupErr == nil {
		check.Price = result.Price
		check.Availability = normalizeAvailability(result.Availability)
		check.Status = domain.CompetitorStatusOK
		check.ErrorCode = ""
		if result.Source != "" {
			check.RawSource = result.Source
		}
	}
	updated, saveErr := s.repo.SaveCheckResult(ctx, competitorID, check)
	if saveErr != nil {
		return nil, fmt.Errorf("competitor save refresh: %w", saveErr)
	}
	if lookupErr != nil {
		return updated, ErrRefreshFailed
	}
	return updated, nil
}

type normalizedTarget struct {
	OzonTarget
	normalized string
}

var productIDPattern = regexp.MustCompile(`\d{6,}`)

func normalizeOzonTarget(raw string, maxLen int) (normalizedTarget, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > maxLen {
		return normalizedTarget{}, ErrInvalidTarget
	}
	id := ""
	targetURL := ""
	if productIDPattern.MatchString(value) && !strings.Contains(value, "://") {
		id = productIDPattern.FindString(value)
		targetURL = "https://www.ozon.ru/product/" + id + "/"
	} else {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" {
			return normalizedTarget{}, ErrInvalidTarget
		}
		host := strings.ToLower(parsed.Host)
		if host != "ozon.ru" && host != "www.ozon.ru" {
			return normalizedTarget{}, ErrInvalidTarget
		}
		id = lastID(parsed.Path)
		if id == "" {
			return normalizedTarget{}, ErrInvalidTarget
		}
		parsed.Scheme = "https"
		parsed.Host = "www.ozon.ru"
		parsed.RawQuery = ""
		parsed.Fragment = ""
		targetURL = parsed.String()
	}
	return normalizedTarget{
		OzonTarget: OzonTarget{PublicProductID: id, URL: targetURL},
		normalized: "ozon:" + id,
	}, nil
}

func lastID(path string) string {
	matches := productIDPattern.FindAllString(path, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

func normalizeAvailability(value string) string {
	switch value {
	case domain.CompetitorAvailabilityAvailable, domain.CompetitorAvailabilityOutOfStock, domain.CompetitorAvailabilityNotFound:
		return value
	default:
		return domain.CompetitorAvailabilityUnknown
	}
}

func safeLookupErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrInvalidTarget) {
		return domain.CompetitorErrorInvalidTarget
	}
	return domain.CompetitorErrorUnavailable
}

func validPrice(price *float64) bool {
	return price != nil && !math.IsNaN(*price) && !math.IsInf(*price, 0) && *price >= 0
}
