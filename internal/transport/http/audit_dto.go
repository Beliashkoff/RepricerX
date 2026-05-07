package transport

import "time"

type priceChangeResponse struct {
	ID            string    `json:"id"`
	ShopID        string    `json:"shop_id"`
	ProductID     string    `json:"product_id"`
	ProductName   string    `json:"product_name"`
	StrategyID    *string   `json:"strategy_id"`
	OldPrice      float64   `json:"old_price"`
	NewPrice      float64   `json:"new_price"`
	TargetPrice   float64   `json:"target_price"`
	Reason        string    `json:"reason"`
	ConstraintHit *string   `json:"constraint_hit"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type summaryResponse struct {
	TotalUpdates      int       `json:"total_updates"`
	SuccessfulUpdates int       `json:"successful_updates"`
	FailedUpdates     int       `json:"failed_updates"`
	AvgChangePct      float64   `json:"avg_change_pct"`
	PeriodStart       time.Time `json:"period_start"`
	PeriodEnd         time.Time `json:"period_end"`
}
