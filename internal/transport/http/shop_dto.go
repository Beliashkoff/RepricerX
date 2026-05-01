package transport

import (
	"encoding/json"
	"time"
)

type createShopRequest struct {
	Marketplace string          `json:"marketplace" binding:"required"`
	Name        string          `json:"name"        binding:"required"`
	Credentials json.RawMessage `json:"credentials" binding:"required"`
}

type updateShopRequest struct {
	Name              *string         `json:"name"`
	Credentials       json.RawMessage `json:"credentials"`
	AutoUpdateEnabled *bool           `json:"autoUpdateEnabled"`
	ScheduleCron      *string         `json:"scheduleCron"`
}

type shopResponse struct {
	ID                string     `json:"id"`
	Marketplace       string     `json:"marketplace"`
	Name              string     `json:"name"`
	Status            string     `json:"status"`
	AutoUpdateEnabled bool       `json:"autoUpdateEnabled"`
	ScheduleCron      string     `json:"scheduleCron"`
	LastCheckedAt     *time.Time `json:"lastCheckedAt"`
	CreatedAt         time.Time  `json:"createdAt"`
}
