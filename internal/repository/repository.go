// Package repository определяет интерфейсы доступа к данным.
// Все реализации живут рядом в *_pg.go файлах.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/google/uuid"
)

// Sentinel-ошибки — используются сервисами для ветвления без type assertion.
var (
	ErrNotFound   = errors.New("not found")
	ErrEmailTaken = errors.New("email already taken")
)

// UsersRepository — операции с таблицей users.
type UsersRepository interface {
	Create(ctx context.Context, u *domain.User) error
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	UpdateDisplayName(ctx context.Context, id uuid.UUID, name string) error
	// UpdatePasswordHash нужен при смене пароля — вызывающий обязан инвалидировать сессии.
	UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error
	RegisterFailedLogin(ctx context.Context, id uuid.UUID, newCount int, lockoutUntil *time.Time) error
	ResetFailedLogin(ctx context.Context, id uuid.UUID) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
}

// SessionsRepository — операции с таблицей sessions.
type SessionsRepository interface {
	Create(ctx context.Context, s *domain.Session) error
	// GetByTokenHash ищет активную сессию: token_hash=$1 AND idle_expires_at > now() AND absolute_expires_at > now().
	GetByTokenHash(ctx context.Context, tokenHash string) (*domain.Session, error)
	// TouchIdleIfNeeded условно продлевает idle TTL, если до истечения < 12 ч.
	// Возвращает новый idle_expires_at, если обновление произошло; nil — если не нужно.
	TouchIdleIfNeeded(ctx context.Context, id uuid.UUID, candidateIdle time.Time) (*time.Time, error)
	TouchLastSeen(ctx context.Context, id uuid.UUID, at time.Time) error
	DeleteByTokenHash(ctx context.Context, tokenHash string) error
	DeleteByUserID(ctx context.Context, userID uuid.UUID) (int64, error)
	DeleteExpired(ctx context.Context) (int64, error)
}

// EmailVerificationsRepository — операции с таблицей email_verifications.
type EmailVerificationsRepository interface {
	Create(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	// GetUnusedValid ищет токен: token_hash=$1 AND expires_at > now() AND used_at IS NULL.
	GetUnusedValid(ctx context.Context, tokenHash string) (id uuid.UUID, userID uuid.UUID, err error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
	// InvalidatePending помечает used_at=now() для всех неиспользованных токенов юзера (при resend).
	InvalidatePending(ctx context.Context, userID uuid.UUID) error
	DeleteExpired(ctx context.Context) (int64, error)
}

// PasswordResetTokensRepository — операции с одноразовыми токенами сброса пароля.
type PasswordResetTokensRepository interface {
	// Issue инвалидирует старые ожидающие токены пользователя и создаёт новый.
	Issue(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	// Consume атомарно помечает валидный токен использованным, меняет пароль,
	// сбрасывает lockout/fail-счётчик и отзывает все сессии пользователя.
	Consume(ctx context.Context, tokenHash string, newPasswordHash string) (userID uuid.UUID, revokedSessions int64, err error)
	DeleteExpired(ctx context.Context) (int64, error)
}

// ShopsRepository — операции с таблицей shops.
type ShopsRepository interface {
	Create(ctx context.Context, shop *domain.Shop) error
	// GetByID возвращает магазин только если он принадлежит userID.
	GetByID(ctx context.Context, id, userID uuid.UUID) (*domain.Shop, error)
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Shop, error)
	Update(ctx context.Context, shop *domain.Shop) error
	// Delete удаляет магазин только если он принадлежит userID.
	Delete(ctx context.Context, id, userID uuid.UUID) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, checkedAt time.Time) error
}

// IntegrationLogRepository — операции с таблицей integration_log.
type IntegrationLogRepository interface {
	Create(ctx context.Context, e *domain.IntegrationLogEntry) error
	// DeleteOlderThan удаляет записи старше cutoff; возвращает число удалённых строк.
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}
