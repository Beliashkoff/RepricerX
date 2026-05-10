package domain

import (
	"time"

	"github.com/google/uuid"
)

// User — основная сущность пользователя.
type User struct {
	ID                 uuid.UUID
	Email              string
	PasswordHash       string
	DisplayName        string
	Status             string // pending_verification | active | blocked
	FailedLoginCount   int
	LockoutUntil       *time.Time // NULL пока нет блокировки
	TelegramMutedUntil *time.Time
	CreatedAt          time.Time
}

// Константы статусов — чтобы не было опечаток в строках по всей кодовой базе.
const (
	UserStatusPending = "pending_verification"
	UserStatusActive  = "active"
	UserStatusBlocked = "blocked"
)
