// Package oauth определяет общий интерфейс для VK ID / Яндекс ID и
// возвращает нейтральные ошибки, чтобы вызывающий сервис не зависел
// от деталей конкретного провайдера.
package oauth

import (
	"context"
	"errors"
	"net/url"

	"github.com/Beliashkoff/RepricerX/internal/domain"
)

// Ошибки, которые могут вернуть адаптеры.
// Сервис маппит их в ErrOAuthProviderFailed / 502 на HTTP-уровне.
var (
	ErrProviderUnavailable = errors.New("oauth: провайдер недоступен")
	ErrProviderRejected    = errors.New("oauth: провайдер отверг запрос")
	ErrInvalidResponse     = errors.New("oauth: некорректный ответ провайдера")
)

// Provider — общий интерфейс OAuth2-провайдера для регистрации/логина.
//
// Поток:
//  1. AuthorizationURL — построить URL для редиректа клиента к провайдеру;
//  2. Exchange — обменять authorization code на access_token (с PKCE);
//  3. FetchUser — получить нормализованные данные пользователя.
//
// Параметр callback в Exchange — это полный query string из callback URL,
// нужен для провайдеро-специфичных параметров (например, VK ID OAuth 2.1
// возвращает device_id, который обязателен в token request). Адаптеры
// игнорируют то, что им не нужно.
//
// Все методы должны быть context-aware и идемпотентны.
type Provider interface {
	Name() domain.OAuthProvider
	AuthorizationURL(state, codeChallenge string) string
	Exchange(ctx context.Context, code, codeVerifier string, callback url.Values) (accessToken string, err error)
	FetchUser(ctx context.Context, accessToken string) (domain.OAuthUserInfo, error)
}
