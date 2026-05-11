package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration/oauth"
	"github.com/Beliashkoff/RepricerX/internal/pkg/netutil"
	"github.com/Beliashkoff/RepricerX/internal/pkg/oauthstate"
	"github.com/Beliashkoff/RepricerX/internal/pkg/password"
	"github.com/Beliashkoff/RepricerX/internal/pkg/token"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

// Публичные OAuth-ошибки, маппятся в HTTP-уровне.
var (
	ErrUnknownOAuthProvider = errors.New("unknown oauth provider")
	ErrOAuthDisabled        = errors.New("oauth provider disabled")
	ErrInvalidOAuthState    = errors.New("invalid oauth state")
	ErrOAuthProviderFailed  = errors.New("oauth provider failed")
)

// stateTTL / linkTTL — окно, в течение которого state-токен (между /start и
// /callback) и link-токен (между /callback и /link-oauth) считаются валидными.
const (
	stateTTL = 10 * time.Minute
	linkTTL  = 10 * time.Minute
)

// OAuthLinkRequest — ответ CompleteOAuth, когда email от провайдера уже
// принадлежит существующему email/password-аккаунту. Хендлер редиректит на
// /link-oauth с этими параметрами; фронт собирает пароль и зовёт
// ConfirmOAuthLink.
type OAuthLinkRequest struct {
	LinkToken string
	Email     string
	Provider  domain.OAuthProvider
}

// AttachOAuth подключает OAuth-зависимости. Если не вызвать — методы
// BeginOAuth/CompleteOAuth/ConfirmOAuthLink возвращают ErrOAuthDisabled.
//
// providers — мап провайдер→адаптер. Допустимо передать только один
// провайдер (например, только Яндекс): хендлер вернёт 503 для VK.
func (s *Service) AttachOAuth(
	providers map[domain.OAuthProvider]oauth.Provider,
	store oauthstate.Store,
	identities repository.OAuthIdentitiesRepository,
) {
	s.oauthProviders = providers
	s.oauthStore = store
	s.oauthIdentities = identities
}

// BeginOAuth готовит редирект на форму согласия провайдера.
// Возвращает (authURL, state) — хендлер должен сделать 302 на authURL.
func (s *Service) BeginOAuth(ctx context.Context, provider domain.OAuthProvider) (string, error) {
	if s.oauthStore == nil || s.oauthProviders == nil {
		return "", ErrOAuthDisabled
	}
	p, ok := s.oauthProviders[provider]
	if !ok {
		return "", ErrUnknownOAuthProvider
	}

	state, err := randomToken(32)
	if err != nil {
		return "", fmt.Errorf("begin oauth: state: %w", err)
	}
	verifier, err := randomToken(32) // 32 байт → 43 base64url-символа (≥43 по PKCE)
	if err != nil {
		return "", fmt.Errorf("begin oauth: verifier: %w", err)
	}
	challenge := codeChallengeS256(verifier)

	err = s.oauthStore.SaveState(ctx, state, oauthstate.StatePayload{
		Provider:     provider,
		CodeVerifier: verifier,
	}, stateTTL)
	if err != nil {
		return "", fmt.Errorf("begin oauth: save state: %w", err)
	}
	return p.AuthorizationURL(state, challenge), nil
}

// CompleteOAuth завершает поток. Возвращает либо LoginResult (создана сессия),
// либо OAuthLinkRequest (нужна привязка с подтверждением паролем). Точно один
// из двух будет ненулевым при err == nil.
func (s *Service) CompleteOAuth(ctx context.Context, r *http.Request, state, code string) (*LoginResult, *OAuthLinkRequest, error) {
	if s.oauthStore == nil || s.oauthProviders == nil || s.oauthIdentities == nil {
		return nil, nil, ErrOAuthDisabled
	}

	statePayload, err := s.oauthStore.ConsumeState(ctx, state)
	if err != nil {
		return nil, nil, ErrInvalidOAuthState
	}

	provider, ok := s.oauthProviders[statePayload.Provider]
	if !ok {
		return nil, nil, ErrUnknownOAuthProvider
	}

	accessToken, err := provider.Exchange(ctx, code, statePayload.CodeVerifier)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrOAuthProviderFailed, err)
	}

	info, err := provider.FetchUser(ctx, accessToken)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrOAuthProviderFailed, err)
	}
	info.Email = strings.ToLower(strings.TrimSpace(info.Email))
	if info.ProviderUserID == "" {
		return nil, nil, ErrOAuthProviderFailed
	}

	// 1) Существует ли уже привязка (provider, external_id)?
	identity, err := s.oauthIdentities.GetByProviderAndExternalID(ctx, statePayload.Provider, info.ProviderUserID)
	if err == nil {
		user, uerr := s.users.GetByID(ctx, identity.UserID)
		if uerr != nil {
			return nil, nil, fmt.Errorf("oauth: load linked user: %w", uerr)
		}
		if user.Status == domain.UserStatusBlocked {
			s.audit.BlockedLoginAttempt(user.ID, netutil.IPPrefix(r, s.cfg.TrustProxy))
			return nil, nil, ErrUserBlocked
		}
		_ = s.oauthIdentities.TouchLastLogin(ctx, identity.ID)
		login, lerr := s.createSession(ctx, r, user)
		if lerr != nil {
			return nil, nil, lerr
		}
		return login, nil, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, nil, fmt.Errorf("oauth: lookup identity: %w", err)
	}

	// 2) Привязки нет. Проверяем, не зарегистрирован ли уже email.
	if info.Email != "" {
		existing, lookupErr := s.users.GetByEmail(ctx, info.Email)
		if lookupErr == nil {
			if existing.Status == domain.UserStatusBlocked {
				s.audit.BlockedLoginAttempt(existing.ID, netutil.IPPrefix(r, s.cfg.TrustProxy))
				return nil, nil, ErrUserBlocked
			}
			// У существующего пользователя есть пароль? Только тогда требуем подтверждение
			// паролем. Если пароля нет (теоретически — другой OAuth-провайдер с тем же email),
			// привязываем без пароля.
			if existing.PasswordHash != "" {
				linkToken, terr := randomToken(32)
				if terr != nil {
					return nil, nil, fmt.Errorf("oauth: link token: %w", terr)
				}
				if perr := s.oauthStore.SaveLink(ctx, linkToken, oauthstate.LinkPayload{
					Provider:       statePayload.Provider,
					ExternalUserID: info.ProviderUserID,
					Email:          info.Email,
					UserID:         existing.ID,
					DisplayName:    info.DisplayName,
				}, linkTTL); perr != nil {
					return nil, nil, fmt.Errorf("oauth: save link: %w", perr)
				}
				return nil, &OAuthLinkRequest{
					LinkToken: linkToken,
					Email:     existing.Email,
					Provider:  statePayload.Provider,
				}, nil
			}
			// Пароля нет → автоматическая привязка identity к существующему юзеру.
			if cerr := s.oauthIdentities.Create(ctx, &domain.OAuthIdentity{
				ID:          uuid.New(),
				UserID:      existing.ID,
				Provider:    statePayload.Provider,
				ExternalID:  info.ProviderUserID,
				Email:       info.Email,
				CreatedAt:   time.Now(),
				LastLoginAt: time.Now(),
			}); cerr != nil {
				return nil, nil, fmt.Errorf("oauth: create identity: %w", cerr)
			}
			login, lerr := s.createSession(ctx, r, existing)
			if lerr != nil {
				return nil, nil, lerr
			}
			return login, nil, nil
		} else if !errors.Is(lookupErr, repository.ErrNotFound) {
			return nil, nil, fmt.Errorf("oauth: lookup user by email: %w", lookupErr)
		}
	}

	// 3) Нет ни identity, ни существующего email — создаём нового пользователя.
	newUser := &domain.User{
		ID:           uuid.New(),
		Email:        oauthEmailFor(statePayload.Provider, info),
		PasswordHash: "", // OAuth-only — БД хранит NULL
		DisplayName:  fallbackDisplayName(info.DisplayName, statePayload.Provider),
		Status:       domain.UserStatusActive,
	}
	if cerr := s.users.Create(ctx, newUser); cerr != nil {
		// На случай гонки: email мог быть занят между нашей проверкой и вставкой.
		if errors.Is(cerr, repository.ErrEmailTaken) {
			return nil, nil, ErrOAuthProviderFailed
		}
		return nil, nil, fmt.Errorf("oauth: create user: %w", cerr)
	}
	if cerr := s.oauthIdentities.Create(ctx, &domain.OAuthIdentity{
		ID:          uuid.New(),
		UserID:      newUser.ID,
		Provider:    statePayload.Provider,
		ExternalID:  info.ProviderUserID,
		Email:       info.Email,
		CreatedAt:   time.Now(),
		LastLoginAt: time.Now(),
	}); cerr != nil {
		return nil, nil, fmt.Errorf("oauth: create identity: %w", cerr)
	}
	login, lerr := s.createSession(ctx, r, newUser)
	if lerr != nil {
		return nil, nil, lerr
	}
	return login, nil, nil
}

// ConfirmOAuthLink завершает привязку OAuth-идентичности к существующему
// email/password-аккаунту. Пользователь обязан ввести правильный пароль.
func (s *Service) ConfirmOAuthLink(ctx context.Context, r *http.Request, linkToken, pwd string) (*LoginResult, error) {
	if s.oauthStore == nil || s.oauthIdentities == nil {
		return nil, ErrOAuthDisabled
	}

	payload, err := s.oauthStore.ConsumeLink(ctx, linkToken)
	if err != nil {
		return nil, ErrInvalidOAuthState
	}

	user, err := s.users.GetByID(ctx, payload.UserID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	ipPrefix := netutil.IPPrefix(r, s.cfg.TrustProxy)
	now := time.Now()

	// Lockout / blocked перед проверкой пароля.
	if user.Status == domain.UserStatusBlocked {
		s.audit.BlockedLoginAttempt(user.ID, ipPrefix)
		return nil, ErrInvalidCredentials
	}
	if user.LockoutUntil != nil && user.LockoutUntil.After(now) {
		_, _ = password.Verify(pwd, fakeArgon2idHash)
		s.audit.AuthFailed(user.Email, "locked_out", ipPrefix)
		return nil, ErrInvalidCredentials
	}
	if user.PasswordHash == "" {
		// OAuth-only пользователь не может пройти линковку — ему нечего вводить.
		s.audit.AuthFailed(user.Email, "no_password", ipPrefix)
		return nil, ErrInvalidCredentials
	}
	ok, err := password.Verify(pwd, user.PasswordHash)
	if err != nil || !ok {
		newCount := user.FailedLoginCount + 1
		lu := lockoutUntil(newCount, now)
		_ = s.users.RegisterFailedLogin(ctx, user.ID, newCount, lu)
		s.audit.AuthFailed(user.Email, "bad_password", ipPrefix)
		return nil, ErrInvalidCredentials
	}

	_ = s.users.ResetFailedLogin(ctx, user.ID)

	if cerr := s.oauthIdentities.Create(ctx, &domain.OAuthIdentity{
		ID:          uuid.New(),
		UserID:      user.ID,
		Provider:    payload.Provider,
		ExternalID:  payload.ExternalUserID,
		Email:       payload.Email,
		CreatedAt:   now,
		LastLoginAt: now,
	}); cerr != nil {
		// Если кто-то уже успел привязать ту же identity к другому юзеру — это
		// необычно, но не критично. Возвращаем generic-ошибку, чтобы не палить
		// внутреннее состояние.
		if errors.Is(cerr, repository.ErrDuplicate) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("link oauth: create identity: %w", cerr)
	}
	return s.createSession(ctx, r, user)
}

// createSession инкапсулирует выпуск нового session-cookie. Используется и
// при OAuth-логине, и при подтверждении линковки.
func (s *Service) createSession(ctx context.Context, r *http.Request, user *domain.User) (*LoginResult, error) {
	now := time.Now()
	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		return nil, fmt.Errorf("session: generate token: %w", err)
	}
	ipPrefix := netutil.IPPrefix(r, s.cfg.TrustProxy)
	ua := truncateUA(r.UserAgent())

	sess := &domain.Session{
		ID:                uuid.New(),
		UserID:            user.ID,
		TokenHash:         tokenHash,
		CreatedAt:         now,
		LastSeenAt:        now,
		IdleExpiresAt:     now.Add(s.cfg.IdleTTL),
		AbsoluteExpiresAt: now.Add(s.cfg.AbsoluteTTL),
		UserAgent:         ua,
		IPPrefix:          ipPrefix,
	}
	if err = s.sessions.Create(ctx, sess); err != nil {
		return nil, fmt.Errorf("session: create: %w", err)
	}
	s.audit.SessionCreated(user.ID, sess.ID, ipPrefix, ua)
	return &LoginResult{User: user, Session: sess, Plaintext: plaintext}, nil
}

// randomToken — 32 байт энтропии в base64url без паддинга (43 символа).
func randomToken(nBytes int) (string, error) {
	raw := make([]byte, nBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// codeChallengeS256 — base64url(sha256(verifier)) по RFC 7636.
func codeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// oauthEmailFor возвращает email для нового пользователя. Если провайдер не
// дал email — генерим уникальный placeholder, чтобы не падать на UNIQUE.
func oauthEmailFor(provider domain.OAuthProvider, info domain.OAuthUserInfo) string {
	if info.Email != "" {
		return info.Email
	}
	hash := sha256.Sum256([]byte(string(provider) + "|" + info.ProviderUserID))
	return string(provider) + "_" + hex.EncodeToString(hash[:6]) + "@noemail.repricerx.local"
}

func fallbackDisplayName(name string, provider domain.OAuthProvider) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	switch provider {
	case domain.OAuthProviderVK:
		return "Пользователь VK"
	case domain.OAuthProviderYandex:
		return "Пользователь Яндекса"
	default:
		return "Пользователь"
	}
}
