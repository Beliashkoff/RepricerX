// Package auth реализует регистрацию, аутентификацию и управление сессиями.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/pkg/auditlog"
	"github.com/Beliashkoff/RepricerX/internal/pkg/mailer"
	"github.com/Beliashkoff/RepricerX/internal/pkg/netutil"
	"github.com/Beliashkoff/RepricerX/internal/pkg/password"
	"github.com/Beliashkoff/RepricerX/internal/pkg/token"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/google/uuid"
)

// Публичные ошибки сервиса — хендлеры маппят их в HTTP-коды.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email taken")
	ErrSessionNotFound    = errors.New("session not found")
	ErrUserBlocked        = errors.New("user blocked")
	ErrInvalidResetToken  = errors.New("invalid reset token")
)

// Config — зависимости сервиса.
type Config struct {
	IdleTTL          time.Duration // 24 ч из конфига
	AbsoluteTTL      time.Duration // 7 дней из конфига
	TrustProxy       bool
	VerificationURL  string // базовый URL без query string
	PasswordResetURL string // базовый URL без query string
}

// Service — основной auth-сервис.
type Service struct {
	users         repository.UsersRepository
	sessions      repository.SessionsRepository
	verifications repository.EmailVerificationsRepository
	resetTokens   repository.PasswordResetTokensRepository
	mailer        mailer.Mailer
	audit         *auditlog.Logger
	cfg           Config

	// resendLimiter — rate-limit resend verification: 1 запрос в минуту на email.
	resendMu           sync.Mutex
	resendLastAt       map[string]time.Time
	resetMu            sync.Mutex
	resetLastAt        map[string]time.Time
	resetAttemptLastAt map[string]time.Time
}

func New(
	users repository.UsersRepository,
	sessions repository.SessionsRepository,
	verifications repository.EmailVerificationsRepository,
	resetTokens repository.PasswordResetTokensRepository,
	m mailer.Mailer,
	audit *auditlog.Logger,
	cfg Config,
) *Service {
	return &Service{
		users:              users,
		sessions:           sessions,
		verifications:      verifications,
		resetTokens:        resetTokens,
		mailer:             m,
		audit:              audit,
		cfg:                cfg,
		resendLastAt:       make(map[string]time.Time),
		resetLastAt:        make(map[string]time.Time),
		resetAttemptLastAt: make(map[string]time.Time),
	}
}

// RegisterResult — результат успешной регистрации.
type RegisterResult struct {
	Email string
}

// Register создаёт нового пользователя и отправляет письмо верификации.
// Сессия НЕ создаётся — вход возможен только после подтверждения email.
func (s *Service) Register(ctx context.Context, email, pwd, displayName string) (*RegisterResult, error) {
	if err := validateEmail(email); err != nil {
		return nil, ErrInvalidEmail
	}
	if err := validatePassword(pwd); err != nil {
		return nil, ErrWeakPassword
	}

	hash, err := password.Hash(pwd)
	if err != nil {
		return nil, fmt.Errorf("register: hash password: %w", err)
	}

	user := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: hash,
		DisplayName:  displayName,
		Status:       domain.UserStatusPending,
	}
	if err = s.users.Create(ctx, user); err != nil {
		if errors.Is(err, repository.ErrEmailTaken) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("register: create user: %w", err)
	}

	// Отправляем письмо асинхронно — не блокируем HTTP-ответ.
	go func(u *domain.User) {
		if err := s.sendVerification(context.Background(), u); err != nil {
			s.audit.EmailSendFailed(u.ID, err)
		} else {
			s.audit.EmailVerificationSent(u.ID)
		}
	}(user)

	return &RegisterResult{Email: email}, nil
}

// sendVerification генерирует токен и отправляет письмо.
func (s *Service) sendVerification(ctx context.Context, user *domain.User) error {
	plaintext, hashHex, err := token.Generate()
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(24 * time.Hour)
	if err = s.verifications.Create(ctx, user.ID, hashHex, expiresAt); err != nil {
		return fmt.Errorf("create verification token: %w", err)
	}

	url := s.cfg.VerificationURL + "?token=" + plaintext
	htmlBody, textBody, err := mailer.RenderVerification(mailer.VerificationData{
		DisplayName: user.DisplayName,
		URL:         url,
	})
	if err != nil {
		return fmt.Errorf("render email template: %w", err)
	}
	return s.mailer.Send(ctx, user.Email, "Подтверждение email — RepricerX", htmlBody, textBody)
}

// VerifyEmailResult — результат верификации: нужен редирект на frontend.
type VerifyEmailResult struct {
	Success bool
}

// VerifyEmail активирует аккаунт по plaintext-токену из письма.
func (s *Service) VerifyEmail(ctx context.Context, plaintextToken string) *VerifyEmailResult {
	tokenHash := token.Hash(plaintextToken)

	id, userID, err := s.verifications.GetUnusedValid(ctx, tokenHash)
	if err != nil {
		return &VerifyEmailResult{Success: false}
	}

	// Транзакционно помечаем токен использованным и активируем пользователя.
	// pgxpool не даёт транзакций через интерфейс — делаем две операции последовательно;
	// anti-replay обеспечивается условием AND status='pending_verification' в UpdateStatus.
	if err = s.verifications.MarkUsed(ctx, id); err != nil {
		return &VerifyEmailResult{Success: false}
	}
	if err = s.users.UpdateStatus(ctx, userID, domain.UserStatusActive); err != nil {
		return &VerifyEmailResult{Success: false}
	}

	s.audit.EmailVerificationUsed(userID)
	return &VerifyEmailResult{Success: true}
}

// ResendVerification повторно отправляет письмо верификации.
// Всегда возвращает nil — не раскрывает существование email.
func (s *Service) ResendVerification(ctx context.Context, email string) error {
	// Rate-limit: не чаще 1 раза в минуту на email.
	s.resendMu.Lock()
	lastAt, ok := s.resendLastAt[email]
	if ok && time.Since(lastAt) < time.Minute {
		s.resendMu.Unlock()
		return nil
	}
	s.resendLastAt[email] = time.Now()
	s.resendMu.Unlock()

	user, err := s.users.GetByEmail(ctx, email)
	if err != nil || user.Status != domain.UserStatusPending {
		return nil // silent no-op
	}

	// Инвалидируем старые токены перед выдачей нового.
	_ = s.verifications.InvalidatePending(ctx, user.ID)

	if err = s.sendVerification(ctx, user); err != nil {
		s.audit.EmailSendFailed(user.ID, err)
	} else {
		s.audit.EmailVerificationSent(user.ID)
	}
	return nil
}

// ForgotPassword выдаёт одноразовый токен сброса пароля и отправляет письмо.
// Всегда возвращает nil — не раскрывает существование email.
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	email = strings.TrimSpace(email)
	if email == "" || validateEmail(email) != nil {
		return nil
	}

	if s.resetRateLimited(email) {
		return nil
	}

	user, err := s.users.GetByEmail(ctx, email)
	if err != nil || user.Status != domain.UserStatusActive {
		return nil
	}

	plaintext, hashHex, err := token.Generate()
	if err != nil {
		return fmt.Errorf("forgot password: generate token: %w", err)
	}

	expiresAt := time.Now().Add(time.Hour)
	if err = s.resetTokens.Issue(ctx, user.ID, hashHex, expiresAt); err != nil {
		return fmt.Errorf("forgot password: issue token: %w", err)
	}

	resetURL := s.cfg.PasswordResetURL + "#token=" + plaintext
	htmlBody, textBody, err := mailer.RenderPasswordReset(mailer.PasswordResetData{
		DisplayName: user.DisplayName,
		URL:         resetURL,
	})
	if err != nil {
		return fmt.Errorf("forgot password: render email template: %w", err)
	}
	// Отправляем письмо асинхронно — не блокируем HTTP-ответ.
	go func(u *domain.User, html, text string) {
		if err := s.mailer.Send(context.Background(), u.Email, "Сброс пароля — RepricerX", html, text); err != nil {
			s.audit.EmailSendFailed(u.ID, err)
		} else {
			s.audit.PasswordResetSent(u.ID)
		}
	}(user, htmlBody, textBody)
	return nil
}

// ResetPassword атомарно использует reset-токен, меняет пароль и отзывает сессии.
func (s *Service) ResetPassword(ctx context.Context, r *http.Request, plaintextToken, newPassword string) error {
	plaintextToken = strings.TrimSpace(plaintextToken)
	if plaintextToken == "" {
		return ErrInvalidResetToken
	}
	tokenHash := token.Hash(plaintextToken)
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	ipPrefix := ""
	if r != nil {
		ipPrefix = netutil.IPPrefix(r, s.cfg.TrustProxy)
	}
	if s.resetAttemptRateLimited(ipPrefix, tokenHash) {
		return ErrInvalidResetToken
	}

	hash, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("reset password: hash password: %w", err)
	}

	userID, revokedSessions, err := s.resetTokens.Consume(ctx, tokenHash, hash)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrInvalidResetToken
	}
	if err != nil {
		return fmt.Errorf("reset password: consume token: %w", err)
	}

	s.audit.PasswordResetUsed(userID, revokedSessions)
	return nil
}

// LoginResult — возвращается при успешном логине.
type LoginResult struct {
	User      *domain.User
	Session   *domain.Session
	Plaintext string // cookie-значение; plaintext нигде больше не хранится
}

// Login аутентифицирует пользователя и создаёт сессию.
// При любой ошибке возвращает ErrInvalidCredentials — одинаковый ответ снаружи.
func (s *Service) Login(ctx context.Context, r *http.Request, email, pwd string) (*LoginResult, error) {
	ipPrefix := netutil.IPPrefix(r, s.cfg.TrustProxy)
	ua := truncateUA(r.UserAgent())
	now := time.Now()

	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		// Пользователь не найден — считаем argon2id поверх фиктивного хеша для constant-time.
		_, _ = password.Verify(pwd, fakeArgon2idHash)
		s.audit.AuthFailed(email, "no_user", ipPrefix)
		return nil, ErrInvalidCredentials
	}

	// Проверяем lockout ДО проверки пароля.
	if user.LockoutUntil != nil && user.LockoutUntil.After(now) {
		// Всё равно считаем argon2id — constant-time.
		_, _ = password.Verify(pwd, fakeArgon2idHash)
		s.audit.AuthFailed(email, "locked_out", ipPrefix)
		return nil, ErrInvalidCredentials
	}

	ok, err := password.Verify(pwd, user.PasswordHash)
	if err != nil || !ok {
		newCount := user.FailedLoginCount + 1
		lu := lockoutUntil(newCount, now)
		_ = s.users.RegisterFailedLogin(ctx, user.ID, newCount, lu)
		s.audit.AuthFailed(email, "bad_password", ipPrefix)
		return nil, ErrInvalidCredentials
	}

	// Пароль верный — проверяем статус.
	switch user.Status {
	case domain.UserStatusPending:
		s.audit.AuthFailed(email, "not_verified", ipPrefix)
		return nil, ErrInvalidCredentials
	case domain.UserStatusBlocked:
		s.audit.BlockedLoginAttempt(user.ID, ipPrefix)
		return nil, ErrInvalidCredentials
	}

	// Успех.
	_ = s.users.ResetFailedLogin(ctx, user.ID)

	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		return nil, fmt.Errorf("login: generate token: %w", err)
	}

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
		return nil, fmt.Errorf("login: create session: %w", err)
	}

	s.audit.SessionCreated(user.ID, sess.ID, ipPrefix, ua)
	return &LoginResult{User: user, Session: sess, Plaintext: plaintext}, nil
}

// Logout инвалидирует сессию по plaintext-токену из cookie.
func (s *Service) Logout(ctx context.Context, plaintext string) {
	tokenHash := token.Hash(plaintext)
	// Пробуем найти сессию для audit-лога — ошибку игнорируем, logout безусловный.
	sess, _ := s.sessions.GetByTokenHash(ctx, tokenHash)
	_ = s.sessions.DeleteByTokenHash(ctx, tokenHash)
	if sess != nil {
		s.audit.SessionRevoked(sess.UserID, sess.ID, "logout")
	}
}

// GetSessionByToken возвращает сессию по plaintext cookie-токену.
// Используется в middleware RequireAuth.
func (s *Service) GetSessionByToken(ctx context.Context, plaintext string) (*domain.Session, error) {
	tokenHash := token.Hash(plaintext)
	sess, err := s.sessions.GetByTokenHash(ctx, tokenHash)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrSessionNotFound
	}
	return sess, err
}

// TouchSession обновляет last_seen_at и при необходимости продлевает idle TTL.
// Возвращает новый idle_expires_at, если TTL продлён (middleware должен обновить cookie).
func (s *Service) TouchSession(ctx context.Context, sess *domain.Session) (*time.Time, error) {
	now := time.Now()
	_ = s.sessions.TouchLastSeen(ctx, sess.ID, now)

	newIdle, err := s.sessions.TouchIdleIfNeeded(ctx, sess.ID, now.Add(s.cfg.IdleTTL))
	if err != nil {
		return nil, err
	}
	if newIdle != nil {
		s.audit.SessionRefreshed(sess.UserID, sess.ID)
	}
	return newIdle, nil
}

// GetUser возвращает пользователя по ID.
func (s *Service) GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	u, err := s.users.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrSessionNotFound
	}
	return u, err
}

// UpdateDisplayName обновляет отображаемое имя.
func (s *Service) UpdateDisplayName(ctx context.Context, id uuid.UUID, name string) error {
	return s.users.UpdateDisplayName(ctx, id, name)
}

// RevokeAllSessions удаляет все сессии пользователя (при блокировке).
func (s *Service) RevokeAllSessions(ctx context.Context, userID uuid.UUID) {
	_, _ = s.sessions.DeleteByUserID(ctx, userID)
}

func (s *Service) resetRateLimited(email string) bool {
	key := strings.ToLower(email)
	now := time.Now()

	s.resetMu.Lock()
	defer s.resetMu.Unlock()

	for k, at := range s.resetLastAt {
		if now.Sub(at) > time.Hour {
			delete(s.resetLastAt, k)
		}
	}

	lastAt, ok := s.resetLastAt[key]
	if ok && now.Sub(lastAt) < time.Minute {
		return true
	}
	s.resetLastAt[key] = now
	return false
}

func (s *Service) resetAttemptRateLimited(ipPrefix, tokenHash string) bool {
	now := time.Now()

	s.resetMu.Lock()
	defer s.resetMu.Unlock()

	for k, at := range s.resetAttemptLastAt {
		if now.Sub(at) > time.Minute {
			delete(s.resetAttemptLastAt, k)
		}
	}

	for _, key := range []string{"ip:" + ipPrefix, "token:" + tokenHash} {
		if key == "ip:" {
			continue
		}
		lastAt, ok := s.resetAttemptLastAt[key]
		if ok && now.Sub(lastAt) < 2*time.Second {
			return true
		}
	}
	for _, key := range []string{"ip:" + ipPrefix, "token:" + tokenHash} {
		if key == "ip:" {
			continue
		}
		s.resetAttemptLastAt[key] = now
	}
	return false
}

func truncateUA(ua string) string {
	runes := []rune(ua)
	if len(runes) > 255 {
		return string(runes[:255])
	}
	return ua
}
