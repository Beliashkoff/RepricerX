package competitor

import (
	"context"
	"encoding/json"
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
	wb         WBPriceLookup
	notifier   NotifierEmitter
	now        func() time.Time
	maxURLSize int
}

type Option func(*Service)

type NotifierEmitter interface {
	NotifyCompetitorPriceDropped(ctx context.Context, userID, productID uuid.UUID, externalSKU string, oldPrice, newPrice float64)
	NotifyCompetitorAppeared(ctx context.Context, userID, productID uuid.UUID, externalSKU, competitorURL string, price float64)
	NotifyMedianShifted(ctx context.Context, userID, productID uuid.UUID, externalSKU string, oldMedian, newMedian float64)
}

func WithNotifier(n NotifierEmitter) Option {
	return func(s *Service) { s.notifier = n }
}

func WithWBLookup(wb WBPriceLookup) Option {
	return func(s *Service) { s.wb = wb }
}

// WithOzonLookup явно задаёт реализацию OzonPriceLookup.
// Используется в cmd/* для выбора источника данных по конфигу.
func WithOzonLookup(ozon OzonPriceLookup) Option {
	return func(s *Service) { s.ozon = ozon }
}

// SelectOzonLookup создаёт реализацию OzonPriceLookup по имени источника.
// Это единственная точка, где cmd/* зависит от конкретных реализаций.
//
//   - "bff" (default) — Ozon BFF JSON API, стабильнее HTML-парсинга
//   - "html"          — legacy HTML-парсинг (ненадёжен на SPA, оставлен как fallback)
//   - "mpstats"       — платный агрегатор; требует непустого mpstatsKey
//
// Для будущего перехода на MPStats: изменить env OZON_PRICE_SOURCE=mpstats,
// добавить MPSTATS_API_KEY — и реализовать NewMPStatsOzonLookup (skeleton уже есть).
func SelectOzonLookup(source, mpstatsKey string) OzonPriceLookup {
	switch source {
	case "mpstats":
		if mpstatsKey != "" {
			return NewMPStatsOzonLookup(mpstatsKey)
		}
		// Ключ не задан — деградируем до BFF с предупреждением (логируется в cmd).
		fallthrough
	case "html":
		return NewHTTPBasedOzonLookup()
	default: // "bff" и любой неизвестный источник
		return NewBFFBasedOzonLookup()
	}
}

func New(repo repository.ProductCompetitorsRepository, ozon OzonPriceLookup, opts ...Option) *Service {
	if ozon == nil {
		// По умолчанию используем BFF API — надёжнее HTML-парсинга.
		ozon = NewBFFBasedOzonLookup()
	}
	s := &Service{
		repo:       repo,
		ozon:       ozon,
		wb:         NewHTTPBasedWBLookup(),
		now:        func() time.Time { return time.Now().UTC() },
		maxURLSize: 2048,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type CreateInput struct {
	ProductID uuid.UUID
	Target    string
}

type UpdateInput struct {
	Target string
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, input CreateInput) (*domain.ProductCompetitor, error) {
	raw := strings.TrimSpace(input.Target)

	// Определяем маркетплейс по содержимому таргета.
	if isWBTarget(raw) {
		return s.createWB(ctx, userID, input.ProductID, raw)
	}
	return s.createOzon(ctx, userID, input.ProductID, raw)
}

func (s *Service) createOzon(ctx context.Context, userID, productID uuid.UUID, raw string) (*domain.ProductCompetitor, error) {
	target, err := normalizeOzonTarget(raw, s.maxURLSize)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.Create(ctx, userID, repository.CompetitorCreateInput{
		ProductID:               productID,
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
		return nil, fmt.Errorf("competitor create ozon: %w", err)
	}
	return item, nil
}

func (s *Service) createWB(ctx context.Context, userID, productID uuid.UUID, raw string) (*domain.ProductCompetitor, error) {
	target, err := normalizeWBTarget(raw, s.maxURLSize)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.Create(ctx, userID, repository.CompetitorCreateInput{
		ProductID:               productID,
		Marketplace:             domain.MarketplaceWB,
		Source:                  "public_wb",
		CompetitorURL:           target.URL,
		NormalizedCompetitorURL: target.normalized,
		OzonPublicProductID:     nil, // не Ozon
	})
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrProductNotFound
	}
	if errors.Is(err, repository.ErrDuplicate) {
		return nil, ErrDuplicateCompetitor
	}
	if err != nil {
		return nil, fmt.Errorf("competitor create wb: %w", err)
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
	raw := strings.TrimSpace(input.Target)

	var compURL, normalizedURL string
	var ozonProductID *string

	if isWBTarget(raw) {
		target, err := normalizeWBTarget(raw, s.maxURLSize)
		if err != nil {
			return nil, err
		}
		compURL = target.URL
		normalizedURL = target.normalized
	} else {
		target, err := normalizeOzonTarget(raw, s.maxURLSize)
		if err != nil {
			return nil, err
		}
		compURL = target.URL
		normalizedURL = target.normalized
		ozonProductID = &target.PublicProductID
	}

	item, err := s.repo.Update(ctx, userID, competitorID, repository.CompetitorUpdateInput{
		CompetitorURL:           compURL,
		NormalizedCompetitorURL: normalizedURL,
		OzonPublicProductID:     ozonProductID,
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

// RefreshFromJob — Этап 7. Обработчик для worker switch на BackgroundJobTypeCompetitorRefresh.
// Парсит payload и делегирует в Refresh. ErrRefreshFailed → retryable error для worker;
// ErrCompetitorNotFound → терминальная ошибка (не retry-ить).
func (s *Service) RefreshFromJob(ctx context.Context, job *domain.BackgroundJob) error {
	var payload domain.CompetitorRefreshJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("competitor refresh: parse payload: %w", err)
	}
	if _, err := s.Refresh(ctx, payload.UserID, payload.CompetitorID); err != nil {
		return err
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

	var result LookupResult
	var lookupErr error

	if item.Marketplace == domain.MarketplaceWB {
		// WB: извлекаем NmID из NormalizedCompetitorURL ("wb:{nmID}") или URL
		nmID := wbNmIDFromNormalized(item.NormalizedCompetitorURL)
		if nmID == "" {
			nmID = WBNmIDFromURL(item.CompetitorURL)
		}
		if nmID == "" {
			lookupErr = fmt.Errorf("%w: cannot extract WB nmID", ErrInvalidTarget)
		} else {
			result, lookupErr = s.wb.Lookup(ctx, nmID)
		}
	} else {
		// Ozon (default)
		target := OzonTarget{URL: item.CompetitorURL}
		if item.OzonPublicProductID != nil {
			target.PublicProductID = *item.OzonPublicProductID
		}
		result, lookupErr = s.ozon.Lookup(ctx, target)
	}
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
	var before repository.CompetitorPriceStats
	if lookupErr == nil && result.Price != nil && s.notifier != nil {
		if stats, err := s.repo.StatsBefore(ctx, item.ProductID, check.CheckedAt); err == nil {
			before = stats
		}
	}
	updated, saveErr := s.repo.SaveCheckResult(ctx, competitorID, check)
	if saveErr != nil {
		return nil, fmt.Errorf("competitor save refresh: %w", saveErr)
	}
	if lookupErr != nil {
		return updated, ErrRefreshFailed
	}
	if result.Price != nil {
		s.emitSignals(ctx, userID, item, updated, before)
	}
	return updated, nil
}

func (s *Service) emitSignals(ctx context.Context, userID uuid.UUID, beforeItem, updated *domain.ProductCompetitor, before repository.CompetitorPriceStats) {
	if s.notifier == nil || updated == nil || updated.LastPrice == nil {
		return
	}
	info, err := s.repo.SignalContext(ctx, userID, updated.ProductID)
	if err != nil {
		return
	}
	after, err := s.repo.CurrentStats(ctx, updated.ProductID)
	if err != nil || after.Min == nil {
		return
	}
	if before.Min == nil {
		if info.CurrentPrice > 0 && *after.Min < info.CurrentPrice {
			s.notifier.NotifyCompetitorAppeared(ctx, userID, updated.ProductID, info.ExternalSKU, updated.CompetitorURL, *after.Min)
		}
		return
	}
	if *after.Min < *before.Min*0.95 {
		s.notifier.NotifyCompetitorPriceDropped(ctx, userID, updated.ProductID, info.ExternalSKU, *before.Min, *after.Min)
	}
	if beforeItem.LastCheckedAt == nil && *updated.LastPrice < info.CurrentPrice {
		s.notifier.NotifyCompetitorAppeared(ctx, userID, updated.ProductID, info.ExternalSKU, updated.CompetitorURL, *updated.LastPrice)
	}
	if before.Median != nil && after.Median != nil && *before.Median > 0 {
		delta := math.Abs(*after.Median-*before.Median) / *before.Median
		if delta > 0.10 {
			s.notifier.NotifyMedianShifted(ctx, userID, updated.ProductID, info.ExternalSKU, *before.Median, *after.Median)
		}
	}
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

// ─── WB helpers ──────────────────────────────────────────────────────────────

// wbURLPattern — URL вида wildberries.ru/catalog/{nmID}/...
var wbURLPattern = regexp.MustCompile(`(?i)wildberries\.ru`)

// wbPureIDPattern — чистое числовое значение длиной 6–12 цифр без http
var wbPureIDPattern = regexp.MustCompile(`^\d{6,12}$`)

// isWBTarget — определяет, относится ли таргет к Wildberries.
// Принимает: URL wildberries.ru, чистый NmID (6-12 цифр начинается с wb: или просто цифры).
// Для Ozon характерен префикс "ozon" в URL или 10-значный ID — мы проверяем WB явно.
func isWBTarget(raw string) bool {
	if strings.HasPrefix(strings.ToLower(raw), "wb:") {
		return true
	}
	if wbURLPattern.MatchString(raw) {
		return true
	}
	// Если пользователь передал "wb/{nmID}" или просто nmID с пометкой — нет.
	// Чистые числа без контекста не считаем WB — они могут быть Ozon ID.
	return false
}

type normalizedWBTarget struct {
	NmID       string
	URL        string
	normalized string
}

// normalizeWBTarget — нормализует WB таргет в NmID, URL и normalized key.
// Принимает: "wildberries.ru/catalog/123456/detail.aspx", "wb:123456", "123456789".
func normalizeWBTarget(raw string, maxLen int) (normalizedWBTarget, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > maxLen {
		return normalizedWBTarget{}, ErrInvalidTarget
	}

	// Убираем префикс "wb:" если есть
	cleanValue := value
	if strings.HasPrefix(strings.ToLower(cleanValue), "wb:") {
		cleanValue = cleanValue[3:]
	}

	nmID := ""
	if wbPureIDPattern.MatchString(cleanValue) {
		nmID = cleanValue
	} else if wbURLPattern.MatchString(cleanValue) {
		// Извлекаем nmID из URL: /catalog/{nmID}/detail.aspx
		nmID = WBNmIDFromURL(cleanValue)
	}

	if nmID == "" || !IsValidWBNmID(nmID) {
		return normalizedWBTarget{}, ErrInvalidTarget
	}

	return normalizedWBTarget{
		NmID:       nmID,
		URL:        "https://www.wildberries.ru/catalog/" + nmID + "/detail.aspx",
		normalized: "wb:" + nmID,
	}, nil
}

// wbNmIDFromNormalized — извлекает NmID из normalized URL вида "wb:{nmID}".
func wbNmIDFromNormalized(normalized string) string {
	if strings.HasPrefix(normalized, "wb:") {
		return normalized[3:]
	}
	return ""
}
