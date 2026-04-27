package domain

import (
	"time"

	"github.com/google/uuid"
)

// Session — серверная сессия пользователя.
// В БД хранится только token_hash (sha256 plaintext-токена из cookie).
// Plaintext никогда не попадает в БД — только в cookie клиента.
type Session struct {
	ID      uuid.UUID
	UserID  uuid.UUID

	// TokenHash = sha256(cookie_value) в hex. UNIQUE-индекс обеспечивает O(1) lookup.
	TokenHash string

	CreatedAt  time.Time
	LastSeenAt time.Time

	// idle_expires_at — скользящий TTL (24 ч), условно обновляется при запросах.
	// absolute_expires_at — фиксированный кэп от момента логина (7 дней).
	// Сессия валидна, пока now() < IdleExpiresAt && now() < AbsoluteExpiresAt.
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time

	// Fingerprint для мягкой детекции смены сети/браузера (не инвалидирует сессию).
	UserAgent string
	IPPrefix  string
}
