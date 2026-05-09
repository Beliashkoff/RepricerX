// Package pricing — движок расчёта цен по стратегии (Этап 5, ТЗ 4.1.1.6).
//
// Логика чистая: Calculate принимает product, strategy, цены конкурентов
// и возвращает CalculateResult без побочных эффектов.
package pricing

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Beliashkoff/RepricerX/internal/domain"
)

const supportedCurrency = "RUB"

// CalculateInput — вход calculator.
type CalculateInput struct {
	Product *domain.Product
	// Strategy = nil допустимо (товар без стратегии) — вернём skipped.
	Strategy *domain.Strategy
	// CompetitorPrices — уже отфильтрованные (не out_of_stock, свежие) цены конкурентов.
	CompetitorPrices []float64
}

// CalculateResult — результат расчёта одного товара.
type CalculateResult struct {
	TargetPrice   float64
	FinalPrice    float64
	ConstraintHit string
	Status        string // PlanItemStatus*
	Reason        string // человекочитаемое описание для UI/логов
	Error         string // непустое для Status=failed
}

// Внутренние типы для парсинга params/constraints.
type fixedParams struct {
	Value float64 `json:"value"`
}
type belowMedianParams struct {
	Pct float64 `json:"pct"`
}
type minCompetitorParams struct {
	Step float64 `json:"step"`
}
type minMarginParams struct {
	MarginPct float64 `json:"margin_pct"`
}

type strategyConstraints struct {
	MinPrice       *float64 `json:"min_price"`
	MaxPrice       *float64 `json:"max_price"`
	MinProfitPct   *float64 `json:"min_profit_pct"`
	MinProfitAbs   *float64 `json:"min_profit_abs"`
	MaxChangePct   *float64 `json:"max_change_pct"`
	MinIntervalMin *int     `json:"min_interval_minutes"`
}

// Calculate — основная функция расчёта цены.
//
// Порядок применения ограничений:
//  1. Pre-checks (currency, status, current_price > 0).
//  2. Расчёт target_price по формуле стратегии (или fallback при отсутствии конкурентов).
//  3. Cost-floor: final = max(final, cost_price) — защита от убытков (всегда).
//  4. min_profit_pct/abs — поверх cost-floor (более строгое).
//  5. min_price/max_price — clamp.
//  6. max_change_pct — clamp по дельте от current.
//  7. Cost-floor финальная защита (ни один шаг не должен опустить ниже cost).
//  8. Округление до 2 знаков.
func Calculate(in CalculateInput) CalculateResult {
	if in.Product == nil {
		return skipped("invalid_input", "missing product")
	}

	// Pre-checks.
	if in.Product.Currency != "" && in.Product.Currency != supportedCurrency {
		return skipped(domain.ReasonUnsupportedCurrency,
			fmt.Sprintf("currency %q not supported", in.Product.Currency))
	}
	if in.Product.Status == domain.ProductStatusArchived {
		return skipped(domain.ReasonProductArchived, "product archived")
	}
	if in.Product.CurrentPrice <= 0 {
		return skipped(domain.ReasonInvalidCurrent, "invalid current_price")
	}

	if in.Strategy == nil {
		return skipped("no_strategy", "no strategy assigned")
	}
	if !in.Strategy.Enabled {
		return skipped(domain.ReasonStrategyDisabled, "strategy disabled")
	}

	constraints, err := parseConstraints(in.Strategy.Constraints)
	if err != nil {
		return failed("invalid_constraints", err.Error())
	}

	// Шаг 1: формула → target.
	target, formulaSkip := computeTarget(in.Strategy, in.Product, in.CompetitorPrices)
	if formulaSkip != nil {
		// Применяем fallback policy.
		return applyFallback(in.Product, in.Strategy, constraints, formulaSkip.reason, formulaSkip.detail)
	}

	costPrice := costPriceOrNil(in.Product)
	final := target
	hit := ""

	// Шаг 2: cost-floor (всегда, если cost задан).
	if costPrice != nil && final < *costPrice {
		final = *costPrice
		hit = domain.ConstraintCostPriceFloor
	}

	// Шаг 3: min_profit_pct / min_profit_abs (только если cost задан).
	if costPrice != nil {
		minProfit := *costPrice
		profitHit := ""
		if constraints.MinProfitPct != nil {
			candidate := *costPrice * (1 + *constraints.MinProfitPct/100)
			if candidate > minProfit {
				minProfit = candidate
				profitHit = domain.ConstraintMinProfitPct
			}
		}
		if constraints.MinProfitAbs != nil {
			candidate := *costPrice + *constraints.MinProfitAbs
			if candidate > minProfit {
				minProfit = candidate
				profitHit = domain.ConstraintMinProfitAbs
			}
		}
		if final < minProfit {
			final = minProfit
			if profitHit != "" {
				hit = profitHit
			}
		}
	}

	// Шаг 4: min_price / max_price clamp.
	if constraints.MaxPrice != nil && final > *constraints.MaxPrice {
		final = *constraints.MaxPrice
		hit = domain.ConstraintMaxPrice
	}
	if constraints.MinPrice != nil && final < *constraints.MinPrice {
		final = *constraints.MinPrice
		hit = domain.ConstraintMinPrice
	}

	// Шаг 5: max_change_pct (clamp по дельте от current).
	if constraints.MaxChangePct != nil && in.Product.CurrentPrice > 0 {
		maxDelta := in.Product.CurrentPrice * (*constraints.MaxChangePct / 100)
		upper := in.Product.CurrentPrice + maxDelta
		lower := in.Product.CurrentPrice - maxDelta
		if final > upper {
			final = upper
			hit = domain.ConstraintMaxChangePct
		} else if final < lower {
			final = lower
			hit = domain.ConstraintMaxChangePct
		}
	}

	// Шаг 6: повторная проверка cost_floor — никакой шаг не должен опустить final ниже cost.
	if costPrice != nil && final < *costPrice {
		final = *costPrice
		hit = domain.ConstraintCostPriceFloor
	}

	final = roundMoney(final)
	target = roundMoney(target)

	return CalculateResult{
		TargetPrice:   target,
		FinalPrice:    final,
		ConstraintHit: hit,
		Status:        domain.PlanItemStatusPending,
		Reason:        formulaReason(in.Strategy, in.Product, in.CompetitorPrices, target),
	}
}

// computeTarget применяет формулу стратегии. Возвращает target или formulaSkip,
// если формула не может быть применена (отсутствие cost, отсутствие конкурентов).
type formulaSkipReason struct {
	reason string
	detail string
}

func computeTarget(s *domain.Strategy, p *domain.Product, competitors []float64) (float64, *formulaSkipReason) {
	switch s.Type {
	case domain.StrategyTypeFixed:
		var fp fixedParams
		if err := json.Unmarshal(s.Params, &fp); err != nil {
			return 0, &formulaSkipReason{"invalid_params", err.Error()}
		}
		return fp.Value, nil

	case domain.StrategyTypeBelowMedianPct:
		var bp belowMedianParams
		if err := json.Unmarshal(s.Params, &bp); err != nil {
			return 0, &formulaSkipReason{"invalid_params", err.Error()}
		}
		if len(competitors) == 0 {
			return 0, &formulaSkipReason{domain.ReasonNoCompetitors, "no competitors for below_median_pct"}
		}
		return median(competitors) * (1 - bp.Pct/100), nil

	case domain.StrategyTypeMinCompetitorPlusStep:
		var mp minCompetitorParams
		if err := json.Unmarshal(s.Params, &mp); err != nil {
			return 0, &formulaSkipReason{"invalid_params", err.Error()}
		}
		if len(competitors) == 0 {
			return 0, &formulaSkipReason{domain.ReasonNoCompetitors, "no competitors for min_competitor_plus_step"}
		}
		return minPrice(competitors) + mp.Step, nil

	case domain.StrategyTypeMinMarginPct:
		var mm minMarginParams
		if err := json.Unmarshal(s.Params, &mm); err != nil {
			return 0, &formulaSkipReason{"invalid_params", err.Error()}
		}
		cost := costPriceOrNil(p)
		if cost == nil {
			return 0, &formulaSkipReason{domain.ReasonMissingCost, "min_margin_pct requires cost_price"}
		}
		return *cost * (1 + mm.MarginPct/100), nil
	}
	return 0, &formulaSkipReason{"unknown_strategy_type", s.Type}
}

// applyFallback — формула не применима. Применяем strategy.fallback_policy.
func applyFallback(p *domain.Product, s *domain.Strategy, c *strategyConstraints, reason, detail string) CalculateResult {
	switch s.FallbackPolicy {
	case domain.FallbackPolicySetMin:
		if c.MinPrice != nil {
			final := roundMoney(applyCostFloor(*c.MinPrice, p))
			return CalculateResult{
				TargetPrice:   0,
				FinalPrice:    final,
				ConstraintHit: "",
				Status:        domain.PlanItemStatusSkipped,
				Reason:        fmt.Sprintf("%s: %s; fallback=set_min", reason, detail),
			}
		}
		// fallthrough: нет min_price → keep_current
		fallthrough
	case domain.FallbackPolicyKeepCurrent:
		final := roundMoney(applyCostFloor(p.CurrentPrice, p))
		return CalculateResult{
			TargetPrice:   0,
			FinalPrice:    final,
			ConstraintHit: "",
			Status:        domain.PlanItemStatusSkipped,
			Reason:        fmt.Sprintf("%s: %s; fallback=keep_current", reason, detail),
		}
	default:
		final := roundMoney(applyCostFloor(p.CurrentPrice, p))
		return CalculateResult{
			TargetPrice:   0,
			FinalPrice:    final,
			ConstraintHit: "",
			Status:        domain.PlanItemStatusSkipped,
			Reason:        fmt.Sprintf("%s: %s; fallback=keep_current (default)", reason, detail),
		}
	}
}

func applyCostFloor(price float64, p *domain.Product) float64 {
	cost := costPriceOrNil(p)
	if cost != nil && price < *cost {
		return *cost
	}
	return price
}

func parseConstraints(raw json.RawMessage) (*strategyConstraints, error) {
	var c strategyConstraints
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return &c, nil
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// costPriceOrNil возвращает указатель на CostPrice, но трактует 0 как nil
// (чтобы пользователь не "случайно" обнулил защиту).
func costPriceOrNil(p *domain.Product) *float64 {
	if p == nil || p.CostPrice == nil {
		return nil
	}
	if *p.CostPrice <= 0 {
		return nil
	}
	return p.CostPrice
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func minPrice(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func skipped(reason, detail string) CalculateResult {
	return CalculateResult{
		Status: domain.PlanItemStatusSkipped,
		Reason: strings.TrimSpace(fmt.Sprintf("%s: %s", reason, detail)),
	}
}

func failed(reason, detail string) CalculateResult {
	return CalculateResult{
		Status: domain.PlanItemStatusFailed,
		Error:  strings.TrimSpace(fmt.Sprintf("%s: %s", reason, detail)),
		Reason: reason,
	}
}

// formulaReason — короткое человекочитаемое описание формулы (для UI).
func formulaReason(s *domain.Strategy, p *domain.Product, competitors []float64, target float64) string {
	switch s.Type {
	case domain.StrategyTypeFixed:
		return fmt.Sprintf("fixed: value=%.2f", target)
	case domain.StrategyTypeBelowMedianPct:
		return fmt.Sprintf("below_median_pct: median=%.2f, n=%d", median(competitors), len(competitors))
	case domain.StrategyTypeMinCompetitorPlusStep:
		return fmt.Sprintf("min_competitor_plus_step: min=%.2f, n=%d", minPrice(competitors), len(competitors))
	case domain.StrategyTypeMinMarginPct:
		cost := costPriceOrNil(p)
		if cost != nil {
			return fmt.Sprintf("min_margin_pct: cost=%.2f", *cost)
		}
		return "min_margin_pct"
	}
	return s.Type
}
