package domain

import (
	"time"

	"github.com/google/uuid"
)

type IntegrationLogEntry struct {
	ID            uuid.UUID
	ShopID        *uuid.UUID
	Operation     string
	HTTPStatus    *int
	ErrorText     string
	CorrelationID uuid.UUID
	CreatedAt     time.Time
}
