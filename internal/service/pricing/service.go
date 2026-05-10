package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

// MarketplaceFactory создаёт клиент маркетплейса по shopID и расшифрованным credentials.
// Та же сигнатура, что в shop.MarketplaceFactory / product.MarketplaceFactory —
// дублируется здесь, чтобы избежать импорта shop/product service из pricing.
type MarketplaceFactory func(shopID string, credsJSON []byte) (integration.Marketplace, error)

var (
	ErrProductNotFound   = errors.New("product not found")
	ErrStrategyNotFound  = errors.New("strategy not found")
	ErrShopNotFound      = errors.New("shop not found")
	ErrPlanNotFound      = errors.New("price plan not found")
	ErrInvalidSimulation = errors.New("invalid pricing simulation")
)

const maxRecalculateBatch = 1000

// SimulateInput — обратно совместим с API. CompetitorPrice — одна цена;
// CompetitorPrices — список (для медианы). Если переданы оба, берётся CompetitorPrices.
type SimulateInput struct {
	ProductID        uuid.UUID
	StrategyID       uuid.UUID
	CompetitorPrice  *float64
	CompetitorPrices []float64
	CostPrice        *float64
}

type SimulateResult struct {
	TargetPrice      float64
	FinalPrice       float64
	ConstraintHit    *string
	Status           string
	Reason           string
	ChangePct        float64
	CompetitorPrice  *float64
	CompetitorSource string
}

// DispatcherTrigger — минимальный интерфейс для auto-dispatch hook (Этап 6).
// Реализуется dispatcher.Service.EnqueueDispatch. Здесь интерфейс — чтобы
// pricing не зависел от dispatcher на уровне типов (избегаем перекрёстного импорта).
type DispatcherTrigger interface {
	EnqueueDispatch(ctx context.Context, userID, planID uuid.UUID) (*domain.BackgroundJob, error)
}

type Service struct {
	products         repository.ProductsRepository
	strategies       repository.StrategiesRepository
	competitors      repository.ProductCompetitorsRepository
	plans            repository.PricePlansRepository
	jobs             repository.BackgroundJobsRepository
	shops            repository.ShopsRepository
	assignments      repository.StrategyAssignmentsRepository
	dispatcher       DispatcherTrigger
	notifier         NotifierEmitter
	competitorMaxAge time.Duration
	priceMaxAge      time.Duration // sync через ListSKUs если old; 0 = sync выключен
	secret           string        // для расшифровки credentials_encrypted
	factories        map[string]MarketplaceFactory
}

// NotifierEmitter — минимальный интерфейс к notifier.Service. Дублируется,
// чтобы не импортировать notifier из pricing (циклы).
type NotifierEmitter interface {
	NotifyRecalcCompleted(ctx context.Context, userID, planID, shopID uuid.UUID, total, calculated, skipped, errs int)
	NotifyConstraintHit(ctx context.Context, userID, planID, shopID uuid.UUID, minPrice, maxPrice, maxChangePct, other int)
}

type Option func(*Service)

func WithCompetitors(repo repository.ProductCompetitorsRepository) Option {
	return func(s *Service) { s.competitors = repo }
}

func WithPlans(plans repository.PricePlansRepository) Option {
	return func(s *Service) { s.plans = plans }
}

func WithJobs(jobs repository.BackgroundJobsRepository) Option {
	return func(s *Service) { s.jobs = jobs }
}

func WithShops(shops repository.ShopsRepository) Option {
	return func(s *Service) { s.shops = shops }
}

func WithAssignments(a repository.StrategyAssignmentsRepository) Option {
	return func(s *Service) { s.assignments = a }
}

// WithDispatcher подключает auto-dispatch hook (Этап 6): после успешного
// ExecuteRecalcJob, если у магазина auto_update_enabled=true, вызывается
// EnqueueDispatch для немедленной отправки plan-а в МП.
func WithDispatcher(d DispatcherTrigger) Option {
	return func(s *Service) { s.dispatcher = d }
}

// WithNotifier — опциональный хук уведомлений.
func WithNotifier(n NotifierEmitter) Option {
	return func(s *Service) { s.notifier = n }
}

// WithPriceSync включает автоматическую синхронизацию current_price товаров
// старше maxAge через MarketplaceFactory. Если хотя бы один товар в plan-е
// имеет last_synced_at старше maxAge, ListSKUs вызывается один раз для всего
// магазина перед расчётом, и products.UpsertImported обновляет цены.
//
// secret — APP_SECRET_KEY для расшифровки shops.credentials_encrypted.
// factories — те же factories, что и в product/shop service (по 1 на marketplace).
func WithPriceSync(secret string, factories map[string]MarketplaceFactory, maxAge time.Duration) Option {
	return func(s *Service) {
		s.secret = secret
		s.factories = factories
		s.priceMaxAge = maxAge
	}
}

func New(products repository.ProductsRepository, strategies repository.StrategiesRepository, opts ...Option) *Service {
	s := &Service{
		products:         products,
		strategies:       strategies,
		competitorMaxAge: 24 * time.Hour,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ─── Simulate ────────────────────────────────────────────────────────────────

func (s *Service) Simulate(ctx context.Context, userID uuid.UUID, input SimulateInput) (*SimulateResult, error) {
	product, err := s.products.GetByIDForUser(ctx, userID, input.ProductID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}
	if input.CostPrice != nil {
		// override cost для симуляции
		product.CostPrice = input.CostPrice
	}

	strategy, err := s.strategies.GetByIDForUser(ctx, userID, input.StrategyID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrStrategyNotFound
	}
	if err != nil {
		return nil, err
	}

	competitorPrices, source := s.resolveCompetitorPrices(ctx, userID, input)

	res := Calculate(CalculateInput{
		Product:          product,
		Strategy:         strategy,
		CompetitorPrices: competitorPrices,
	})

	var hit *string
	if res.ConstraintHit != "" {
		h := res.ConstraintHit
		hit = &h
	}
	var firstCompetitor *float64
	if len(competitorPrices) > 0 {
		c := competitorPrices[0]
		firstCompetitor = &c
	}
	return &SimulateResult{
		TargetPrice:      res.TargetPrice,
		FinalPrice:       res.FinalPrice,
		ConstraintHit:    hit,
		Status:           res.Status,
		Reason:           res.Reason,
		ChangePct:        roundPercent(percentChange(product.CurrentPrice, res.FinalPrice)),
		CompetitorPrice:  firstCompetitor,
		CompetitorSource: source,
	}, nil
}

// resolveCompetitorPrices: приоритет — input.CompetitorPrices, потом input.CompetitorPrice (одна),
// потом БД (latest fresh price из ProductCompetitors).
func (s *Service) resolveCompetitorPrices(ctx context.Context, userID uuid.UUID, input SimulateInput) ([]float64, string) {
	if len(input.CompetitorPrices) > 0 {
		out := make([]float64, 0, len(input.CompetitorPrices))
		for _, p := range input.CompetitorPrices {
			if p > 0 {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out, "manual"
		}
	}
	if input.CompetitorPrice != nil && *input.CompetitorPrice > 0 {
		return []float64{*input.CompetitorPrice}, "manual"
	}
	if s.competitors != nil {
		latest, err := s.competitors.LatestFreshPrice(ctx, userID, input.ProductID, s.competitorMaxAge)
		if err == nil && latest != nil && *latest > 0 {
			return []float64{*latest}, "auto"
		}
	}
	return nil, ""
}

// ─── Recalculate (async) ─────────────────────────────────────────────────────

// Recalculate создаёт PricePlan(status=pending) и enqueue job для воркера.
// productIDs пуст → весь магазин (все товары с назначенной стратегией).
func (s *Service) Recalculate(ctx context.Context, userID, shopID uuid.UUID, productIDs []uuid.UUID) (*domain.PricePlan, *domain.BackgroundJob, error) {
	if s.plans == nil || s.jobs == nil || s.shops == nil {
		return nil, nil, errors.New("pricing service: plans/jobs/shops repositories required for Recalculate")
	}

	if _, err := s.shops.GetByID(ctx, shopID, userID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, ErrShopNotFound
		}
		return nil, nil, fmt.Errorf("recalculate: get shop: %w", err)
	}

	if len(productIDs) > maxRecalculateBatch {
		return nil, nil, fmt.Errorf("%w: max %d products per recalculation", ErrInvalidSimulation, maxRecalculateBatch)
	}

	plan, err := s.plans.Create(ctx, shopID)
	if err != nil {
		return nil, nil, fmt.Errorf("recalculate: create plan: %w", err)
	}

	payload := domain.PriceRecalculationJobPayload{
		PlanID:            plan.ID,
		ShopID:            shopID,
		ProductIDs:        productIDs,
		RequestedByUserID: userID,
	}
	payloadBytes, _ := json.Marshal(payload)

	job, err := s.jobs.Enqueue(ctx, repository.BackgroundJobEnqueue{
		JobType:     domain.BackgroundJobTypePriceRecalculation,
		Queue:       "default",
		Priority:    100,
		Payload:     payloadBytes,
		MaxAttempts: 3,
	})
	if err != nil {
		// откат статуса плана — иначе он останется висеть
		_ = s.plans.UpdateStatus(ctx, plan.ID, domain.PlanStatusFailed)
		return nil, nil, fmt.Errorf("recalculate: enqueue job: %w", err)
	}
	return plan, job, nil
}

func (s *Service) GetPlan(ctx context.Context, userID, planID uuid.UUID) (*domain.PricePlan, []*domain.PricePlanItem, error) {
	if s.plans == nil {
		return nil, nil, errors.New("plans repository not configured")
	}
	plan, items, err := s.plans.GetByIDForUser(ctx, userID, planID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil, ErrPlanNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	return plan, items, nil
}

func (s *Service) ListPlans(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.PricePlan, int, error) {
	if s.plans == nil {
		return nil, 0, errors.New("plans repository not configured")
	}
	return s.plans.ListByUser(ctx, userID, limit, offset)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func percentChange(current, next float64) float64 {
	if current == 0 {
		return 0
	}
	return ((next - current) / current) * 100
}

func roundMoney(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

func roundPercent(v float64) float64 {
	return math.Round(v*10) / 10
}
