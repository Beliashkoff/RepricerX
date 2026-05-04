package transport

import "time"

type createProductRequest struct {
	ExternalSKU  string   `json:"externalSku" binding:"required"`
	Name         string   `json:"name" binding:"required"`
	CurrentPrice float64  `json:"currentPrice"`
	Currency     string   `json:"currency"`
	Status       string   `json:"status"`
	MinPrice     *float64 `json:"minPrice"`
	MaxPrice     *float64 `json:"maxPrice"`
	CostPrice    *float64 `json:"costPrice"`
}

type productResponse struct {
	ID           string     `json:"id"`
	ShopID       string     `json:"shopId"`
	ExternalSKU  string     `json:"externalSku"`
	Name         string     `json:"name"`
	CurrentPrice float64    `json:"currentPrice"`
	Currency     string     `json:"currency"`
	Status       string     `json:"status"`
	MinPrice     *float64   `json:"minPrice"`
	MaxPrice     *float64   `json:"maxPrice"`
	CostPrice    *float64   `json:"costPrice"`
	StockCount   int        `json:"stockCount"`
	Rating       *float64   `json:"rating"`
	ReviewsCount int        `json:"reviewsCount"`
	LastSyncedAt *time.Time `json:"lastSyncedAt"`
	HasStrategy  bool       `json:"hasStrategy"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type productListResponse struct {
	Items      []productResponse `json:"items"`
	Pagination paginationInfo    `json:"pagination"`
}

type paginationInfo struct {
	Page    int `json:"page"`
	PerPage int `json:"perPage"`
	Total   int `json:"total"`
}

type importStartResponse struct {
	ImportID  string    `json:"importId"`
	JobID     *string   `json:"jobId,omitempty"`
	ShopID    string    `json:"shopId"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"startedAt"`
	PollURL   string    `json:"pollUrl"`
}

type importStatusResponse struct {
	ID         string           `json:"id"`
	JobID      *string          `json:"jobId,omitempty"`
	ShopID     string           `json:"shopId"`
	Status     string           `json:"status"`
	JobStatus  string           `json:"jobStatus,omitempty"`
	Total      int              `json:"total"`
	Added      int              `json:"added"`
	Updated    int              `json:"updated"`
	Skipped    int              `json:"skipped"`
	Failed     int              `json:"failed"`
	Errors     []importErrorDTO `json:"errors"`
	StartedAt  time.Time        `json:"startedAt"`
	FinishedAt *time.Time       `json:"finishedAt"`
}

type importErrorDTO struct {
	ExternalSKU string `json:"externalSku,omitempty"`
	Code        string `json:"code"`
	Message     string `json:"message"`
}

type bulkPatchProductItem struct {
	ID        string   `json:"id" binding:"required"`
	MinPrice  *float64 `json:"minPrice"`
	MaxPrice  *float64 `json:"maxPrice"`
	CostPrice *float64 `json:"costPrice"`
}

type bulkPatchRequest struct {
	Products []bulkPatchProductItem `json:"products" binding:"required,min=1,max=100"`
}

type bulkPatchResponse struct {
	Updated int `json:"updated"`
}

type importErrorsResponse struct {
	Items   []importErrorDTO `json:"items"`
	Total   int              `json:"total"`
	Page    int              `json:"page"`
	PerPage int              `json:"perPage"`
}
