package domain

import (
	"time"

	"github.com/google/uuid"
)

// OAuthProvider — поддерживаемые внешние провайдеры идентификации.
type OAuthProvider string

const (
	OAuthProviderVK     OAuthProvider = "vk"
	OAuthProviderYandex OAuthProvider = "yandex"
)

// IsValid проверяет, что значение — известный провайдер.
func (p OAuthProvider) IsValid() bool {
	switch p {
	case OAuthProviderVK, OAuthProviderYandex:
		return true
	default:
		return false
	}
}

// OAuthIdentity — связь внешнего аккаунта (VK / Яндекс) с локальным пользователем.
// UNIQUE(provider, external_id) на уровне БД гарантирует один-к-одному.
type OAuthIdentity struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Provider    OAuthProvider
	ExternalID  string
	Email       string
	CreatedAt   time.Time
	LastLoginAt time.Time
}

// OAuthUserInfo — нормализованные данные о пользователе от провайдера.
// Возвращается адаптерами `internal/integration/oauth/{vkid,yandex}`.
type OAuthUserInfo struct {
	ProviderUserID string
	Email          string
	DisplayName    string
}
