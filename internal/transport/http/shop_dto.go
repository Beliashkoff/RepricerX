package transport

import (
	"encoding/json"
	"time"
)

type createShopRequest struct {
	Marketplace string          `json:"marketplace" binding:"required" example:"wb"`
	Name        string          `json:"name"        binding:"required" example:"Мой магазин на WB"`
	Credentials json.RawMessage `json:"credentials" binding:"required" swaggertype:"object" example:"{\"api_key\":\"wb-token\"}"`
}

type updateShopRequest struct {
	Name              *string         `json:"name"              example:"Новое название"`
	Credentials       json.RawMessage `json:"credentials"       swaggertype:"object"`
	AutoUpdateEnabled *bool           `json:"autoUpdateEnabled" example:"true"`
	ScheduleCron      *string         `json:"scheduleCron"      example:"0 */4 * * *"`
}

type shopResponse struct {
	ID                string     `json:"id"                example:"550e8400-e29b-41d4-a716-446655440000"`
	Marketplace       string     `json:"marketplace"       example:"wb"`
	Name              string     `json:"name"              example:"Мой магазин на WB"`
	Status            string     `json:"status"            example:"active"`
	AutoUpdateEnabled bool       `json:"autoUpdateEnabled" example:"false"`
	ScheduleCron      string     `json:"scheduleCron"      example:"0 */4 * * *"`
	LastCheckedAt     *time.Time `json:"lastCheckedAt"`
	CreatedAt         time.Time  `json:"createdAt"`
}

type testConnectionResponse struct {
	Status string `json:"status" example:"active"`
}
