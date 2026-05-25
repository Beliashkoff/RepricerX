package pricing

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func ptr[T any](v T) *T { return &v }

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func makeProduct(currentPrice float64, costPrice *float64) *domain.Product {
	return &domain.Product{
		ID:           uuid.New(),
		ShopID:       uuid.New(),
		CurrentPrice: currentPrice,
		Currency:     "RUB",
		Status:       domain.ProductStatusActive,
		CostPrice:    costPrice,
	}
}

func makeStrategy(stratType string, params, constraints any, fallback string) *domain.Strategy {
	return &domain.Strategy{
		ID:             uuid.New(),
		Type:           stratType,
		Params:         mustJSON(params),
		Constraints:    mustJSON(constraints),
		FallbackPolicy: fallback,
		Enabled:        true,
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.005
}

// ─── tests by case ───────────────────────────────────────────────────────────

func TestCalc_Fixed_Basic(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, nil),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 500}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
	})
	if r.Status != domain.PlanItemStatusPending || !almostEqual(r.FinalPrice, 500) {
		t.Errorf("got %+v, want pending/500", r)
	}
}

func TestCalc_Fixed_CostFloorTriggered(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, ptr(600.0)),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 500}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
	})
	if r.FinalPrice != 600 || r.ConstraintHit != domain.ConstraintCostPriceFloor {
		t.Errorf("got %+v, want final=600 hit=cost_price_floor", r)
	}
}

func TestCalc_Fixed_WithinMinMax(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, nil),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 1000},
			map[string]any{"min_price": 800, "max_price": 1200}, domain.FallbackPolicyKeepCurrent),
	})
	if r.FinalPrice != 1000 || r.ConstraintHit != "" {
		t.Errorf("got %+v, want final=1000 hit=''", r)
	}
}

func TestCalc_Fixed_MaxPriceClamp(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, nil),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 1500},
			map[string]any{"max_price": 1200}, domain.FallbackPolicyKeepCurrent),
	})
	// Сначала max_price=1200, потом max_change_pct может прижать; но max_change_pct не задан.
	if r.FinalPrice != 1200 || r.ConstraintHit != domain.ConstraintMaxPrice {
		t.Errorf("got %+v, want final=1200 hit=max_price", r)
	}
}

func TestCalc_Fixed_MinPriceClamp(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, nil),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 500},
			map[string]any{"min_price": 800}, domain.FallbackPolicyKeepCurrent),
	})
	if r.FinalPrice != 800 || r.ConstraintHit != domain.ConstraintMinPrice {
		t.Errorf("got %+v, want final=800 hit=min_price", r)
	}
}

func TestCalc_Fixed_MaxChangePctDownClamp(t *testing.T) {
	// Хотим 500 от 900 — это -44%. Лимит 10% → final=810.
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, nil),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 500},
			map[string]any{"max_change_pct": 10}, domain.FallbackPolicyKeepCurrent),
	})
	if !almostEqual(r.FinalPrice, 810) || r.ConstraintHit != domain.ConstraintMaxChangePct {
		t.Errorf("got %+v, want final=810 hit=max_change_pct", r)
	}
}

func TestCalc_BelowMedianPct_HappyPath(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:          makeProduct(900, nil),
		Strategy:         makeStrategy(domain.StrategyTypeBelowMedianPct, map[string]any{"pct": 5}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
		CompetitorPrices: []float64{800, 850, 900},
	})
	// median([800,850,900])=850 * 0.95 = 807.5
	if !almostEqual(r.FinalPrice, 807.5) || r.Status != domain.PlanItemStatusPending {
		t.Errorf("got %+v, want final=807.5 pending", r)
	}
}

func TestCalc_BelowMedianPct_EvenCount(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:          makeProduct(900, nil),
		Strategy:         makeStrategy(domain.StrategyTypeBelowMedianPct, map[string]any{"pct": 10}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
		CompetitorPrices: []float64{800, 1000},
	})
	// median([800,1000])=900 * 0.9 = 810
	if !almostEqual(r.FinalPrice, 810) {
		t.Errorf("got %+v, want final=810", r)
	}
}

func TestCalc_BelowMedianPct_CostFloorBlocks(t *testing.T) {
	// median=800*0.95=760, cost=900 → final поднимается до 900.
	r := Calculate(CalculateInput{
		Product:          makeProduct(900, ptr(900.0)),
		Strategy:         makeStrategy(domain.StrategyTypeBelowMedianPct, map[string]any{"pct": 5}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
		CompetitorPrices: []float64{700, 800, 900},
	})
	if r.FinalPrice != 900 || r.ConstraintHit != domain.ConstraintCostPriceFloor {
		t.Errorf("got %+v, want final=900 hit=cost_price_floor", r)
	}
}

func TestCalc_BelowMedianPct_NoCompetitors_KeepCurrent(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, nil),
		Strategy: makeStrategy(domain.StrategyTypeBelowMedianPct, map[string]any{"pct": 5}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
	})
	if r.Status != domain.PlanItemStatusSkipped || r.FinalPrice != 900 {
		t.Errorf("got %+v, want skipped final=900", r)
	}
}

func TestCalc_BelowMedianPct_NoCompetitors_SetMin(t *testing.T) {
	r := Calculate(CalculateInput{
		Product: makeProduct(900, nil),
		Strategy: makeStrategy(domain.StrategyTypeBelowMedianPct, map[string]any{"pct": 5},
			map[string]any{"min_price": 750}, domain.FallbackPolicySetMin),
	})
	if r.Status != domain.PlanItemStatusSkipped || r.FinalPrice != 750 {
		t.Errorf("got %+v, want skipped final=750 (set_min)", r)
	}
}

func TestCalc_BelowMedianPct_NoCompetitors_SetMin_NoMinPrice(t *testing.T) {
	// SetMin без min_price → fall to keep_current
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, nil),
		Strategy: makeStrategy(domain.StrategyTypeBelowMedianPct, map[string]any{"pct": 5}, map[string]any{}, domain.FallbackPolicySetMin),
	})
	if r.Status != domain.PlanItemStatusSkipped || r.FinalPrice != 900 {
		t.Errorf("got %+v, want skipped final=900 (fallback to keep_current)", r)
	}
}

func TestCalc_MinCompetitorPlusStep_Happy(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:          makeProduct(900, nil),
		Strategy:         makeStrategy(domain.StrategyTypeMinCompetitorPlusStep, map[string]any{"step": 50}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
		CompetitorPrices: []float64{800, 850, 900},
	})
	if !almostEqual(r.FinalPrice, 850) {
		t.Errorf("got %+v, want final=850", r)
	}
}

func TestCalc_MinCompetitorPlusStep_ZeroStep(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:          makeProduct(900, nil),
		Strategy:         makeStrategy(domain.StrategyTypeMinCompetitorPlusStep, map[string]any{"step": 0}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
		CompetitorPrices: []float64{800},
	})
	if r.FinalPrice != 800 {
		t.Errorf("got %+v, want final=800", r)
	}
}

func TestCalc_MinCompetitorPlusStep_CostFloorBlocks(t *testing.T) {
	// min=700 + step=100 = 800; cost=850 → final=850
	r := Calculate(CalculateInput{
		Product:          makeProduct(900, ptr(850.0)),
		Strategy:         makeStrategy(domain.StrategyTypeMinCompetitorPlusStep, map[string]any{"step": 100}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
		CompetitorPrices: []float64{700, 750},
	})
	if r.FinalPrice != 850 || r.ConstraintHit != domain.ConstraintCostPriceFloor {
		t.Errorf("got %+v, want final=850 hit=cost_price_floor", r)
	}
}

func TestCalc_MinCompetitorPlusStep_NoCompetitors_Fallback(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, nil),
		Strategy: makeStrategy(domain.StrategyTypeMinCompetitorPlusStep, map[string]any{"step": 50}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
	})
	if r.Status != domain.PlanItemStatusSkipped || r.FinalPrice != 900 {
		t.Errorf("got %+v, want skipped final=900", r)
	}
}

func TestCalc_MinMarginPct_Happy(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, ptr(500.0)),
		Strategy: makeStrategy(domain.StrategyTypeMinMarginPct, map[string]any{"margin_pct": 30}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
	})
	// 500 * 1.3 = 650
	if !almostEqual(r.FinalPrice, 650) {
		t.Errorf("got %+v, want final=650", r)
	}
}

func TestCalc_MinMarginPct_NoCost_Skipped(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, nil),
		Strategy: makeStrategy(domain.StrategyTypeMinMarginPct, map[string]any{"margin_pct": 30}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
	})
	if r.Status != domain.PlanItemStatusSkipped {
		t.Errorf("got %+v, want skipped", r)
	}
}

func TestCalc_MinMarginPct_MinProfitPctBumps(t *testing.T) {
	// margin_pct=20 → target=500*1.2=600
	// min_profit_pct=30 → minProfit=500*1.3=650
	// final должно быть 650, hit=min_profit_pct
	r := Calculate(CalculateInput{
		Product: makeProduct(900, ptr(500.0)),
		Strategy: makeStrategy(domain.StrategyTypeMinMarginPct, map[string]any{"margin_pct": 20},
			map[string]any{"min_profit_pct": 30}, domain.FallbackPolicyKeepCurrent),
	})
	if !almostEqual(r.FinalPrice, 650) || r.ConstraintHit != domain.ConstraintMinProfitPct {
		t.Errorf("got %+v, want final=650 hit=min_profit_pct", r)
	}
}

func TestCalc_MinMarginPct_MinProfitAbsBumps(t *testing.T) {
	// margin=10 → 500*1.1=550
	// min_profit_abs=200 → minProfit=500+200=700
	r := Calculate(CalculateInput{
		Product: makeProduct(900, ptr(500.0)),
		Strategy: makeStrategy(domain.StrategyTypeMinMarginPct, map[string]any{"margin_pct": 10},
			map[string]any{"min_profit_abs": 200}, domain.FallbackPolicyKeepCurrent),
	})
	if !almostEqual(r.FinalPrice, 700) || r.ConstraintHit != domain.ConstraintMinProfitAbs {
		t.Errorf("got %+v, want final=700 hit=min_profit_abs", r)
	}
}

func TestCalc_MinMarginPct_MinProfitBothStricter(t *testing.T) {
	// pct=20 → 500*1.2=600
	// abs=200 → 500+200=700
	// → берём 700 (более строгое)
	r := Calculate(CalculateInput{
		Product: makeProduct(900, ptr(500.0)),
		Strategy: makeStrategy(domain.StrategyTypeMinMarginPct, map[string]any{"margin_pct": 10},
			map[string]any{"min_profit_pct": 20, "min_profit_abs": 200}, domain.FallbackPolicyKeepCurrent),
	})
	if !almostEqual(r.FinalPrice, 700) || r.ConstraintHit != domain.ConstraintMinProfitAbs {
		t.Errorf("got %+v, want final=700 hit=min_profit_abs", r)
	}
}

func TestCalc_Fixed_MinProfitPct_AlreadySatisfied(t *testing.T) {
	// fixed=500, cost=400, min_profit_pct=15 → minProfit=460 → 500≥460, не трогаем
	r := Calculate(CalculateInput{
		Product: makeProduct(900, ptr(400.0)),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 500},
			map[string]any{"min_profit_pct": 15}, domain.FallbackPolicyKeepCurrent),
	})
	if r.FinalPrice != 500 || r.ConstraintHit != "" {
		t.Errorf("got %+v, want final=500 hit=''", r)
	}
}

func TestCalc_Fixed_MinProfitPct_NoCostIgnored(t *testing.T) {
	// cost=nil → min_profit_pct игнорируется
	r := Calculate(CalculateInput{
		Product: makeProduct(900, nil),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 500},
			map[string]any{"min_profit_pct": 30}, domain.FallbackPolicyKeepCurrent),
	})
	if r.FinalPrice != 500 {
		t.Errorf("got %+v, want final=500", r)
	}
}

func TestCalc_BelowMedianPct_MaxChangePctDominates(t *testing.T) {
	// pct=20, median([800,820,840])=820, target=820*0.8=656; current=1000, max_change=10% → final=900
	r := Calculate(CalculateInput{
		Product: makeProduct(1000, nil),
		Strategy: makeStrategy(domain.StrategyTypeBelowMedianPct, map[string]any{"pct": 20},
			map[string]any{"max_change_pct": 10}, domain.FallbackPolicyKeepCurrent),
		CompetitorPrices: []float64{800, 820, 840},
	})
	if !almostEqual(r.FinalPrice, 900) || r.ConstraintHit != domain.ConstraintMaxChangePct {
		t.Errorf("got %+v, want final=900 hit=max_change_pct", r)
	}
}

func TestCalc_Rounding(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, nil),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 999.999}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
	})
	if r.FinalPrice != 1000.00 {
		t.Errorf("got %v, want rounded 1000.00", r.FinalPrice)
	}
}

func TestCalc_Fixed_CostExceedsMinPrice(t *testing.T) {
	// value=200, min_price=300, max_price=500, cost=400 → cost_floor выигрывает = 400
	r := Calculate(CalculateInput{
		Product: makeProduct(900, ptr(400.0)),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 200},
			map[string]any{"min_price": 300, "max_price": 500}, domain.FallbackPolicyKeepCurrent),
	})
	if r.FinalPrice != 400 || r.ConstraintHit != domain.ConstraintCostPriceFloor {
		t.Errorf("got %+v, want final=400 hit=cost_price_floor", r)
	}
}

func TestCalc_Fixed_MaxChangePctUpClamp(t *testing.T) {
	// fixed=2000, current=1000, max_change=50% → upper=1500 → final=1500
	r := Calculate(CalculateInput{
		Product: makeProduct(1000, nil),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 2000},
			map[string]any{"max_change_pct": 50}, domain.FallbackPolicyKeepCurrent),
	})
	if !almostEqual(r.FinalPrice, 1500) || r.ConstraintHit != domain.ConstraintMaxChangePct {
		t.Errorf("got %+v, want final=1500 hit=max_change_pct", r)
	}
}

func TestCalc_UnsupportedCurrency(t *testing.T) {
	p := makeProduct(900, ptr(500.0))
	p.Currency = "USD"
	r := Calculate(CalculateInput{
		Product:  p,
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 100}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
	})
	if r.Status != domain.PlanItemStatusSkipped {
		t.Errorf("got %+v, want skipped (currency)", r)
	}
}

func TestCalc_ArchivedProduct(t *testing.T) {
	p := makeProduct(900, nil)
	p.Status = domain.ProductStatusArchived
	r := Calculate(CalculateInput{
		Product:  p,
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 100}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
	})
	if r.Status != domain.PlanItemStatusSkipped {
		t.Errorf("got %+v, want skipped (archived)", r)
	}
}

func TestCalc_DisabledStrategy(t *testing.T) {
	s := makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 500}, map[string]any{}, domain.FallbackPolicyKeepCurrent)
	s.Enabled = false
	r := Calculate(CalculateInput{Product: makeProduct(900, nil), Strategy: s})
	if r.Status != domain.PlanItemStatusSkipped {
		t.Errorf("got %+v, want skipped (disabled)", r)
	}
}

func TestCalc_NoStrategy(t *testing.T) {
	r := Calculate(CalculateInput{Product: makeProduct(900, nil), Strategy: nil})
	if r.Status != domain.PlanItemStatusSkipped {
		t.Errorf("got %+v, want skipped", r)
	}
}

func TestCalc_InvalidCurrent(t *testing.T) {
	r := Calculate(CalculateInput{
		Product:  makeProduct(0, nil),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 500}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
	})
	if r.Status != domain.PlanItemStatusSkipped {
		t.Errorf("got %+v, want skipped (current=0)", r)
	}
}

func TestCalc_CostZeroTreatedAsNil(t *testing.T) {
	// cost=0 → не должно блокировать (трактуется как nil)
	r := Calculate(CalculateInput{
		Product:  makeProduct(900, ptr(0.0)),
		Strategy: makeStrategy(domain.StrategyTypeFixed, map[string]any{"value": 500}, map[string]any{}, domain.FallbackPolicyKeepCurrent),
	})
	if r.FinalPrice != 500 {
		t.Errorf("got %+v, want final=500 (cost=0 ignored)", r)
	}
}

func TestCalc_FallbackSetFixed_WithPrice(t *testing.T) {
	// fallback=set_fixed + fallback_price=750 → при отсутствии конкурентов должны получить 750
	r := Calculate(CalculateInput{
		Product:          makeProduct(900, nil),
		Strategy:         makeStrategy(domain.StrategyTypeBelowMedianPct, map[string]any{"pct": 5}, map[string]any{"fallback_price": 750}, domain.FallbackPolicySetFixed),
		CompetitorPrices: nil, // нет конкурентов → формула не применима → fallback
	})
	if r.Status != domain.PlanItemStatusSkipped {
		t.Errorf("got status=%q, want skipped", r.Status)
	}
	if r.FinalPrice != 750 {
		t.Errorf("got FinalPrice=%.2f, want 750 (fallback set_fixed)", r.FinalPrice)
	}
}

func TestCalc_FallbackSetFixed_WithoutPrice_DegradesToKeepCurrent(t *testing.T) {
	// fallback=set_fixed но fallback_price не задана → деградирует до keep_current
	r := Calculate(CalculateInput{
		Product:          makeProduct(900, nil),
		Strategy:         makeStrategy(domain.StrategyTypeBelowMedianPct, map[string]any{"pct": 5}, map[string]any{}, domain.FallbackPolicySetFixed),
		CompetitorPrices: nil,
	})
	if r.Status != domain.PlanItemStatusSkipped {
		t.Errorf("got status=%q, want skipped", r.Status)
	}
	if r.FinalPrice != 900 {
		t.Errorf("got FinalPrice=%.2f, want 900 (keep_current degradation)", r.FinalPrice)
	}
}

func TestCalc_FallbackSetFixed_BelowCostFloor(t *testing.T) {
	// fallback_price=300 но cost=500 → cost-floor поднимает до 500
	r := Calculate(CalculateInput{
		Product:          makeProduct(900, ptr(500.0)),
		Strategy:         makeStrategy(domain.StrategyTypeBelowMedianPct, map[string]any{"pct": 5}, map[string]any{"fallback_price": 300}, domain.FallbackPolicySetFixed),
		CompetitorPrices: nil,
	})
	if r.FinalPrice != 500 {
		t.Errorf("got FinalPrice=%.2f, want 500 (cost-floor applied over fallback_price=300)", r.FinalPrice)
	}
}
