package transport

type simulatePriceRequest struct {
	ProductID       string   `json:"product_id" binding:"required"`
	StrategyID      string   `json:"strategy_id" binding:"required"`
	CompetitorPrice *float64 `json:"competitor_price"`
	CostPrice       *float64 `json:"cost_price"`
}

type simulatePriceResponse struct {
	TargetPrice      float64  `json:"target_price"`
	FinalPrice       float64  `json:"final_price"`
	ConstraintHit    *string  `json:"constraint_hit"`
	Reason           string   `json:"reason"`
	ChangePct        float64  `json:"change_pct"`
	CompetitorPrice  *float64 `json:"competitor_price,omitempty"`
	CompetitorSource string   `json:"competitor_source,omitempty"`
}
