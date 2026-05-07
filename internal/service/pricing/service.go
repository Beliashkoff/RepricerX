package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrProductNotFound   = errors.New("product not found")
	ErrStrategyNotFound  = errors.New("strategy not found")
	ErrInvalidSimulation = errors.New("invalid pricing simulation")
)

type SimulateInput struct {
	ProductID       uuid.UUID
	StrategyID      uuid.UUID
	CompetitorPrice *float64
	CostPrice       *float64
}

type SimulateResult struct {
	TargetPrice      float64
	FinalPrice       float64
	ConstraintHit    *string
	Reason           string
	ChangePct        float64
	CompetitorPrice  *float64
	CompetitorSource string
}

type Service struct {
	products         repository.ProductsRepository
	strategies       repository.StrategiesRepository
	competitors      repository.ProductCompetitorsRepository
	competitorMaxAge time.Duration
}

type Option func(*Service)

func WithCompetitors(repo repository.ProductCompetitorsRepository) Option {
	return func(s *Service) {
		s.competitors = repo
	}
}

func New(products repository.ProductsRepository, strategies repository.StrategiesRepository, opts ...Option) *Service {
	s := &Service{products: products, strategies: strategies, competitorMaxAge: 24 * time.Hour}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) Simulate(ctx context.Context, userID uuid.UUID, input SimulateInput) (*SimulateResult, error) {
	product, err := s.products.GetByIDForUser(ctx, userID, input.ProductID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}
	strategy, err := s.strategies.GetByIDForUser(ctx, userID, input.StrategyID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrStrategyNotFound
	}
	if err != nil {
		return nil, err
	}
	if !strategy.Enabled {
		return nil, fmt.Errorf("%w: strategy disabled", ErrInvalidSimulation)
	}
	competitorPrice := input.CompetitorPrice
	competitorSource := ""
	if competitorPrice != nil {
		competitorSource = "manual"
	} else if s.competitors != nil {
		latest, err := s.competitors.LatestFreshPrice(ctx, userID, input.ProductID, s.competitorMaxAge)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		if latest != nil {
			competitorPrice = latest
			competitorSource = "auto"
		}
	}

	calcInput := input
	calcInput.CompetitorPrice = competitorPrice
	target, reason, err := calculateTarget(product.CurrentPrice, product.CostPrice, calcInput, strategy)
	if err != nil {
		return nil, err
	}
	final, hit := applyConstraints(product.CurrentPrice, target, strategy.Constraints)
	return &SimulateResult{
		TargetPrice:      roundMoney(target),
		FinalPrice:       roundMoney(final),
		ConstraintHit:    hit,
		Reason:           reason,
		ChangePct:        roundPercent(percentChange(product.CurrentPrice, final)),
		CompetitorPrice:  competitorPrice,
		CompetitorSource: competitorSource,
	}, nil
}

func calculateTarget(current float64, productCost *float64, input SimulateInput, strategy *domain.Strategy) (float64, string, error) {
	switch strategy.Type {
	case domain.StrategyTypeFixed:
		var params struct {
			Value float64 `json:"value"`
			Price float64 `json:"price"`
		}
		if err := json.Unmarshal(strategy.Params, &params); err != nil {
			return 0, "", fmt.Errorf("%w: invalid fixed params", ErrInvalidSimulation)
		}
		target := params.Value
		if target == 0 {
			target = params.Price
		}
		if target <= 0 {
			return 0, "", fmt.Errorf("%w: fixed price required", ErrInvalidSimulation)
		}
		return target, "fixed: фиксированная цена", nil
	case domain.StrategyTypeBelowMedianPct:
		var params struct {
			Pct float64 `json:"pct"`
		}
		if err := json.Unmarshal(strategy.Params, &params); err != nil {
			return 0, "", fmt.Errorf("%w: invalid below_median_pct params", ErrInvalidSimulation)
		}
		base := current
		if input.CompetitorPrice != nil && *input.CompetitorPrice > 0 {
			base = *input.CompetitorPrice
		}
		return base * (1 - params.Pct/100), "below_median_pct: расчёт относительно цены конкурента", nil
	case domain.StrategyTypeMinCompetitorPlusStep:
		if input.CompetitorPrice == nil || *input.CompetitorPrice <= 0 {
			return 0, "", fmt.Errorf("%w: competitor_price required", ErrInvalidSimulation)
		}
		var params struct {
			Step float64 `json:"step"`
		}
		if err := json.Unmarshal(strategy.Params, &params); err != nil {
			return 0, "", fmt.Errorf("%w: invalid min_competitor_plus_step params", ErrInvalidSimulation)
		}
		return *input.CompetitorPrice + params.Step, "min_competitor_plus_step: цена конкурента плюс шаг", nil
	case domain.StrategyTypeMinMarginPct:
		cost := productCost
		if input.CostPrice != nil {
			cost = input.CostPrice
		}
		if cost == nil || *cost <= 0 {
			return 0, "", fmt.Errorf("%w: cost_price required", ErrInvalidSimulation)
		}
		var params struct {
			MarginPct float64 `json:"margin_pct"`
		}
		if err := json.Unmarshal(strategy.Params, &params); err != nil {
			return 0, "", fmt.Errorf("%w: invalid min_margin_pct params", ErrInvalidSimulation)
		}
		return *cost * (1 + params.MarginPct/100), "min_margin_pct: цена с минимальной маржой", nil
	default:
		return 0, "", fmt.Errorf("%w: unknown strategy type", ErrInvalidSimulation)
	}
}

func applyConstraints(current, target float64, raw json.RawMessage) (float64, *string) {
	var c struct {
		MinPrice     *float64 `json:"min_price"`
		MaxPrice     *float64 `json:"max_price"`
		MaxChangePct *float64 `json:"max_change_pct"`
	}
	_ = json.Unmarshal(raw, &c)

	final := target
	var hit *string
	setHit := func(name string) {
		if hit == nil {
			hit = &name
		}
	}
	if c.MinPrice != nil && final < *c.MinPrice {
		final = *c.MinPrice
		setHit("min_price")
	}
	if c.MaxPrice != nil && final > *c.MaxPrice {
		final = *c.MaxPrice
		setHit("max_price")
	}
	if c.MaxChangePct != nil && current > 0 {
		limit := current * (*c.MaxChangePct / 100)
		minAllowed := current - limit
		maxAllowed := current + limit
		if final < minAllowed {
			final = minAllowed
			setHit("max_change_pct")
		}
		if final > maxAllowed {
			final = maxAllowed
			setHit("max_change_pct")
		}
	}
	return final, hit
}

func percentChange(current, next float64) float64 {
	if current == 0 {
		return 0
	}
	return ((next - current) / current) * 100
}

func roundMoney(v float64) float64 {
	return math.Round(v*100) / 100
}

func roundPercent(v float64) float64 {
	return math.Round(v*10) / 10
}
