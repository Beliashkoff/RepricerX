package transport

import "time"

type competitorRequest struct {
	Target string `json:"target" binding:"required"`
}

type competitorResponse struct {
	ID                  string     `json:"id"`
	ProductID           string     `json:"productId"`
	Marketplace         string     `json:"marketplace"`
	Source              string     `json:"source"`
	CompetitorURL       string     `json:"competitorUrl"`
	OzonPublicProductID *string    `json:"ozonPublicProductId"`
	LastPrice           *float64   `json:"lastPrice"`
	LastAvailability    string     `json:"lastAvailability"`
	LastCheckedAt       *time.Time `json:"lastCheckedAt"`
	LastErrorCode       string     `json:"lastErrorCode"`
	LastStatus          string     `json:"lastStatus"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}
