package transport

import (
	"encoding/json"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
)

type createStrategyRequest struct {
	Name           string          `json:"name"           binding:"required"`
	Type           string          `json:"type"           binding:"required"`
	Params         json.RawMessage `json:"params"         binding:"required"`
	Constraints    json.RawMessage `json:"constraints"`
	FallbackPolicy string          `json:"fallbackPolicy" binding:"required"`
	Priority       int             `json:"priority"`
	Enabled        bool            `json:"enabled"`
}

type updateStrategyRequest struct {
	Name           *string         `json:"name"`
	Type           *string         `json:"type"`
	Params         json.RawMessage `json:"params"`
	Constraints    json.RawMessage `json:"constraints"`
	FallbackPolicy *string         `json:"fallbackPolicy"`
	Priority       *int            `json:"priority"`
	Enabled        *bool           `json:"enabled"`
}

type assignStrategyRequest struct {
	ProductIDs []string `json:"productIds" binding:"required"`
}

type strategyResponse struct {
	ID             uuid.UUID       `json:"id"`
	UserID         uuid.UUID       `json:"userId"`
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	Params         json.RawMessage `json:"params"`
	Constraints    json.RawMessage `json:"constraints"`
	FallbackPolicy string          `json:"fallbackPolicy"`
	Priority       int             `json:"priority"`
	Enabled        bool            `json:"enabled"`
	AssignedCount  int             `json:"assignedCount"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type strategyDetailResponse struct {
	strategyResponse
	AssignedProductIDs []uuid.UUID `json:"assignedProductIds"`
}

func toStrategyResponse(s *domain.Strategy, assignedCount int) strategyResponse {
	return strategyResponse{
		ID:             s.ID,
		UserID:         s.UserID,
		Name:           s.Name,
		Type:           s.Type,
		Params:         s.Params,
		Constraints:    s.Constraints,
		FallbackPolicy: s.FallbackPolicy,
		Priority:       s.Priority,
		Enabled:        s.Enabled,
		AssignedCount:  assignedCount,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}
