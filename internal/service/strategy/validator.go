package strategy

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/Beliashkoff/RepricerX/internal/domain"
)

const maxMoneyValue = 9_999_999_999.99

var validFallbackPolicies = map[string]bool{
	domain.FallbackPolicyKeepCurrent: true,
	domain.FallbackPolicySetFixed:    true,
	domain.FallbackPolicySetMin:      true,
}

var validStrategyTypes = map[string]bool{
	domain.StrategyTypeFixed:                true,
	domain.StrategyTypeBelowMedianPct:       true,
	domain.StrategyTypeMinCompetitorPlusStep: true,
	domain.StrategyTypeMinMarginPct:         true,
}

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
	FallbackPrice  *float64 `json:"fallback_price"`
}

func validateType(t string) error {
	if !validStrategyTypes[t] {
		return fmt.Errorf("%w: %q", ErrInvalidStrategyType, t)
	}
	return nil
}

func validateFallbackPolicy(p string) error {
	if !validFallbackPolicies[p] {
		return fmt.Errorf("%w: %q", ErrInvalidStrategyType, p)
	}
	return nil
}

func validateParams(stratType string, rawParams json.RawMessage) error {
	if len(rawParams) == 0 {
		return fmt.Errorf("%w: params required", ErrInvalidStrategyParams)
	}
	switch stratType {
	case domain.StrategyTypeFixed:
		var p fixedParams
		if err := json.Unmarshal(rawParams, &p); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidStrategyParams, err)
		}
		if err := validateMoney(p.Value); err != nil || p.Value <= 0 {
			return fmt.Errorf("%w: value must be in (0, %.2f]", ErrInvalidStrategyParams, maxMoneyValue)
		}
	case domain.StrategyTypeBelowMedianPct:
		var p belowMedianParams
		if err := json.Unmarshal(rawParams, &p); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidStrategyParams, err)
		}
		if p.Pct < 0 || p.Pct > 100 {
			return fmt.Errorf("%w: pct must be in [0, 100]", ErrInvalidStrategyParams)
		}
	case domain.StrategyTypeMinCompetitorPlusStep:
		var p minCompetitorParams
		if err := json.Unmarshal(rawParams, &p); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidStrategyParams, err)
		}
		if p.Step < 0 {
			return fmt.Errorf("%w: step must be >= 0", ErrInvalidStrategyParams)
		}
	case domain.StrategyTypeMinMarginPct:
		var p minMarginParams
		if err := json.Unmarshal(rawParams, &p); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidStrategyParams, err)
		}
		if p.MarginPct < 0 {
			return fmt.Errorf("%w: margin_pct must be >= 0", ErrInvalidStrategyParams)
		}
	}
	return nil
}

func validateConstraints(rawConstraints json.RawMessage) error {
	if len(rawConstraints) == 0 {
		return nil
	}
	var c strategyConstraints
	if err := json.Unmarshal(rawConstraints, &c); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidConstraints, err)
	}
	if c.MinPrice != nil {
		if err := validateMoney(*c.MinPrice); err != nil {
			return fmt.Errorf("%w: min_price invalid", ErrInvalidConstraints)
		}
	}
	if c.MaxPrice != nil {
		if err := validateMoney(*c.MaxPrice); err != nil {
			return fmt.Errorf("%w: max_price invalid", ErrInvalidConstraints)
		}
	}
	if c.MinPrice != nil && c.MaxPrice != nil && *c.MinPrice > *c.MaxPrice {
		return fmt.Errorf("%w: min_price must be <= max_price", ErrInvalidConstraints)
	}
	if c.MinProfitPct != nil {
		if *c.MinProfitPct < 0 {
			return fmt.Errorf("%w: min_profit_pct must be >= 0", ErrInvalidConstraints)
		}
	}
	if c.MinProfitAbs != nil {
		if err := validateMoney(*c.MinProfitAbs); err != nil {
			return fmt.Errorf("%w: min_profit_abs invalid", ErrInvalidConstraints)
		}
	}
	if c.MaxChangePct != nil {
		if *c.MaxChangePct < 0 {
			return fmt.Errorf("%w: max_change_pct must be >= 0", ErrInvalidConstraints)
		}
	}
	if c.MinIntervalMin != nil {
		if *c.MinIntervalMin < 1 || *c.MinIntervalMin > 1440 {
			return fmt.Errorf("%w: min_interval_minutes must be in [1, 1440]", ErrInvalidConstraints)
		}
	}
	if c.FallbackPrice != nil {
		if err := validateMoney(*c.FallbackPrice); err != nil || *c.FallbackPrice <= 0 {
			return fmt.Errorf("%w: fallback_price must be > 0", ErrInvalidConstraints)
		}
	}
	return nil
}

func validateMoney(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > maxMoneyValue {
		return ErrInvalidStrategyParams
	}
	return nil
}
