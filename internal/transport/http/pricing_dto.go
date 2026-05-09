package transport

import "time"

type simulatePriceRequest struct {
	ProductID        string    `json:"product_id" binding:"required"`
	StrategyID       string    `json:"strategy_id" binding:"required"`
	CompetitorPrice  *float64  `json:"competitor_price"`
	CompetitorPrices []float64 `json:"competitor_prices"`
	CostPrice        *float64  `json:"cost_price"`
}

type simulatePriceResponse struct {
	TargetPrice      float64  `json:"target_price"`
	FinalPrice       float64  `json:"final_price"`
	ConstraintHit    *string  `json:"constraint_hit"`
	Status           string   `json:"status"`
	Reason           string   `json:"reason"`
	ChangePct        float64  `json:"change_pct"`
	CompetitorPrice  *float64 `json:"competitor_price,omitempty"`
	CompetitorSource string   `json:"competitor_source,omitempty"`
}

type recalculateRequest struct {
	ShopID     string   `json:"shop_id" binding:"required"`
	ProductIDs []string `json:"product_ids"`
}

type pricePlanResponse struct {
	ID        string    `json:"id"`
	ShopID    string    `json:"shop_id"`
	Status    string    `json:"status"`
	Total     int       `json:"total"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type recalculateResponse struct {
	Plan  pricePlanResponse `json:"plan"`
	JobID string            `json:"job_id"`
}

type pricePlanItemResponse struct {
	ID            string  `json:"id"`
	ProductID     string  `json:"product_id"`
	ProductName   string  `json:"product_name"`
	StrategyID    *string `json:"strategy_id,omitempty"`
	CurrentPrice  float64 `json:"current_price"`
	TargetPrice   float64 `json:"target_price"`
	FinalPrice    float64 `json:"final_price"`
	ConstraintHit string  `json:"constraint_hit"`
	Status        string  `json:"status"`
	Error         string  `json:"error,omitempty"`
}

type pricePlanDetailResponse struct {
	Plan  pricePlanResponse        `json:"plan"`
	Items []pricePlanItemResponse  `json:"items"`
}

type pricePlanListResponse struct {
	Items   []pricePlanResponse `json:"items"`
	Total   int                 `json:"total"`
	Limit   int                 `json:"limit"`
	Offset  int                 `json:"offset"`
}
