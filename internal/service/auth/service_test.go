package auth_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/pkg/auditlog"
	"github.com/Beliashkoff/RepricerX/internal/pkg/password"
	"github.com/Beliashkoff/RepricerX/internal/pkg/token"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/Beliashkoff/RepricerX/internal/service/auth"
	"github.com/google/uuid"
)

const testPwd = "ValidPassword12"

var testHash string

func TestMain(m *testing.M) {
	h, err := password.Hash(testPwd)
	if err != nil {
		panic("service_test: hash: " + err.Error())
	}
	testHash = h
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Fake repos
// ---------------------------------------------------------------------------

type fakeUsersRepo struct {
	mu      sync.Mutex
	byEmail map[string]*domain.User
	byID    map[uuid.UUID]*domain.User
}

func newFakeUsers() *fakeUsersRepo {
	return &fakeUsersRepo{byEmail: make(map[string]*domain.User), byID: make(map[uuid.UUID]*domain.User)}
}

func (r *fakeUsersRepo) add(email, hash string, status string) *domain.User {
	u := &domain.User{ID: uuid.New(), Email: email, PasswordHash: hash, Status: status, DisplayName: "Test"}
	r.mu.Lock()
	r.byEmail[email] = u
	r.byID[u.ID] = u
	r.mu.Unlock()
	return u
}

func (r *fakeUsersRepo) get(id uuid.UUID) *domain.User {
	r.mu.Lock()
	u := r.byID[id]
	r.mu.Unlock()
	return u
}

func (r *fakeUsersRepo) Create(_ context.Context, u *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byEmail[u.Email]; ok {
		return repository.ErrEmailTaken
	}
	c := *u
	r.byEmail[u.Email] = &c
	r.byID[u.ID] = &c
	return nil
}

func (r *fakeUsersRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byEmail[email]
	if !ok {
		return nil, repository.ErrNotFound
	}
	c := *u
	return &c, nil
}

func (r *fakeUsersRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	c := *u
	return &c, nil
}

func (r *fakeUsersRepo) UpdateDisplayName(_ context.Context, id uuid.UUID, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[id]; ok {
		u.DisplayName = name
	}
	return nil
}

func (r *fakeUsersRepo) UpdatePasswordHash(_ context.Context, id uuid.UUID, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[id]; ok {
		u.PasswordHash = hash
		r.byEmail[u.Email].PasswordHash = hash
	}
	return nil
}

func (r *fakeUsersRepo) RegisterFailedLogin(_ context.Context, id uuid.UUID, count int, lockoutUntil *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[id]; ok {
		u.FailedLoginCount = count
		u.LockoutUntil = lockoutUntil
		r.byEmail[u.Email].FailedLoginCount = count
		r.byEmail[u.Email].LockoutUntil = lockoutUntil
	}
	return nil
}

func (r *fakeUsersRepo) ResetFailedLogin(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[id]; ok {
		u.FailedLoginCount = 0
		u.LockoutUntil = nil
		r.byEmail[u.Email].FailedLoginCount = 0
		r.byEmail[u.Email].LockoutUntil = nil
	}
	return nil
}

func (r *fakeUsersRepo) UpdateStatus(_ context.Context, id uuid.UUID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[id]; ok {
		u.Status = status
		r.byEmail[u.Email].Status = status
	}
	return nil
}

// ---------------------------------------------------------------------------

type fakeSessionsRepo struct {
	mu       sync.Mutex
	sessions map[string]*domain.Session
}

func newFakeSessions() *fakeSessionsRepo {
	return &fakeSessionsRepo{sessions: make(map[string]*domain.Session)}
}

func (r *fakeSessionsRepo) add(userID uuid.UUID, tokenHash string, idleExpires, absExpires time.Time) *domain.Session {
	s := &domain.Session{
		ID: uuid.New(), UserID: userID, TokenHash: tokenHash,
		CreatedAt: time.Now(), LastSeenAt: time.Now(),
		IdleExpiresAt: idleExpires, AbsoluteExpiresAt: absExpires,
	}
	r.mu.Lock()
	r.sessions[tokenHash] = s
	r.mu.Unlock()
	return s
}

func (r *fakeSessionsRepo) countByUser(userID uuid.UUID) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int
	for _, s := range r.sessions {
		if s.UserID == userID {
			n++
		}
	}
	return n
}

func (r *fakeSessionsRepo) Create(_ context.Context, s *domain.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *s
	r.sessions[s.TokenHash] = &c
	return nil
}

func (r *fakeSessionsRepo) GetByTokenHash(_ context.Context, tokenHash string) (*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[tokenHash]
	if !ok {
		return nil, repository.ErrNotFound
	}
	now := time.Now()
	if now.After(s.IdleExpiresAt) || now.After(s.AbsoluteExpiresAt) {
		return nil, repository.ErrNotFound
	}
	c := *s
	return &c, nil
}

// TouchIdleIfNeeded продлевает idle TTL только если до истечения < 12ч.
func (r *fakeSessionsRepo) TouchIdleIfNeeded(_ context.Context, id uuid.UUID, candidateIdle time.Time) (*time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.sessions {
		if s.ID == id {
			if time.Until(s.IdleExpiresAt) < 12*time.Hour {
				s.IdleExpiresAt = candidateIdle
				t := candidateIdle
				return &t, nil
			}
			return nil, nil
		}
	}
	return nil, nil
}

func (r *fakeSessionsRepo) TouchLastSeen(_ context.Context, _ uuid.UUID, _ time.Time) error { return nil }

func (r *fakeSessionsRepo) DeleteByTokenHash(_ context.Context, tokenHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, tokenHash)
	return nil
}

func (r *fakeSessionsRepo) DeleteByUserID(_ context.Context, userID uuid.UUID) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for k, s := range r.sessions {
		if s.UserID == userID {
			delete(r.sessions, k)
			n++
		}
	}
	return n, nil
}

func (r *fakeSessionsRepo) DeleteExpired(_ context.Context) (int64, error) { return 0, nil }

// ---------------------------------------------------------------------------

type verToken struct {
	id        uuid.UUID
	userID    uuid.UUID
	expiresAt time.Time
	usedAt    *time.Time
}

type fakeVerificationsRepo struct {
	mu     sync.Mutex
	tokens map[string]*verToken
}

func newFakeVerifications() *fakeVerificationsRepo {
	return &fakeVerificationsRepo{tokens: make(map[string]*verToken)}
}

func (r *fakeVerificationsRepo) addToken(userID uuid.UUID, tokenHash string) {
	r.mu.Lock()
	r.tokens[tokenHash] = &verToken{id: uuid.New(), userID: userID, expiresAt: time.Now().Add(time.Hour)}
	r.mu.Unlock()
}

func (r *fakeVerificationsRepo) Create(_ context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	r.mu.Lock()
	r.tokens[tokenHash] = &verToken{id: uuid.New(), userID: userID, expiresAt: expiresAt}
	r.mu.Unlock()
	return nil
}

func (r *fakeVerificationsRepo) GetUnusedValid(_ context.Context, tokenHash string) (uuid.UUID, uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.tokens[tokenHash]
	if !ok || v.usedAt != nil || time.Now().After(v.expiresAt) {
		return uuid.Nil, uuid.Nil, repository.ErrNotFound
	}
	return v.id, v.userID, nil
}

func (r *fakeVerificationsRepo) MarkUsed(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for _, v := range r.tokens {
		if v.id == id {
			v.usedAt = &now
			return nil
		}
	}
	return repository.ErrNotFound
}

func (r *fakeVerificationsRepo) InvalidatePending(_ context.Context, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for _, v := range r.tokens {
		if v.userID == userID && v.usedAt == nil {
			v.usedAt = &now
		}
	}
	return nil
}

func (r *fakeVerificationsRepo) DeleteExpired(_ context.Context) (int64, error) { return 0, nil }

// ---------------------------------------------------------------------------

type resetTok struct {
	id        uuid.UUID
	userID    uuid.UUID
	expiresAt time.Time
	usedAt    *time.Time
}

type fakeResetTokensRepo struct {
	mu       sync.Mutex
	tokens   map[string]*resetTok
	users    *fakeUsersRepo
	sessions *fakeSessionsRepo
}

func newFakeResetTokens(u *fakeUsersRepo, s *fakeSessionsRepo) *fakeResetTokensRepo {
	return &fakeResetTokensRepo{tokens: make(map[string]*resetTok), users: u, sessions: s}
}

func (r *fakeResetTokensRepo) addToken(userID uuid.UUID, tokenHash string) {
	r.mu.Lock()
	r.tokens[tokenHash] = &resetTok{id: uuid.New(), userID: userID, expiresAt: time.Now().Add(time.Hour)}
	r.mu.Unlock()
}

func (r *fakeResetTokensRepo) Issue(_ context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, t := range r.tokens {
		if t.userID == userID {
			delete(r.tokens, k)
		}
	}
	r.tokens[tokenHash] = &resetTok{id: uuid.New(), userID: userID, expiresAt: expiresAt}
	return nil
}

// Consume атомарно: помечает токен использованным, меняет пароль,
// сбрасывает счётчик неудач и отзывает все сессии.
func (r *fakeResetTokensRepo) Consume(_ context.Context, tokenHash, newHash string) (uuid.UUID, int64, error) {
	r.mu.Lock()
	t, ok := r.tokens[tokenHash]
	if !ok || t.usedAt != nil || time.Now().After(t.expiresAt) {
		r.mu.Unlock()
		return uuid.Nil, 0, repository.ErrNotFound
	}
	now := time.Now()
	t.usedAt = &now
	userID := t.userID
	r.mu.Unlock()

	_ = r.users.UpdatePasswordHash(context.Background(), userID, newHash)
	_ = r.users.ResetFailedLogin(context.Background(), userID)
	revoked, _ := r.sessions.DeleteByUserID(context.Background(), userID)
	return userID, revoked, nil
}

func (r *fakeResetTokensRepo) DeleteExpired(_ context.Context) (int64, error) { return 0, nil }

// ---------------------------------------------------------------------------

type fakeMailer struct{}

func (m *fakeMailer) Send(_ context.Context, _, _, _, _ string) error { return nil }

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

type testDeps struct {
	svc           *auth.Service
	users         *fakeUsersRepo
	sessions      *fakeSessionsRepo
	verifications *fakeVerificationsRepo
	resets        *fakeResetTokensRepo
}

func newTestDeps() *testDeps {
	users := newFakeUsers()
	sessions := newFakeSessions()
	verifications := newFakeVerifications()
	resets := newFakeResetTokens(users, sessions)
	audit := auditlog.New(slog.New(slog.NewTextHandler(io.Discard, nil)))

	svc := auth.New(users, sessions, verifications, resets, &fakeMailer{}, audit,
		auth.Config{IdleTTL: 24 * time.Hour, AbsoluteTTL: 7 * 24 * time.Hour},
	)
	return &testDeps{svc: svc, users: users, sessions: sessions, verifications: verifications, resets: resets}
}

// ---------------------------------------------------------------------------
// Brute-force / lockout tests
// ---------------------------------------------------------------------------

// TestLogin_LockoutThresholds проверяет, что:
// после 5 неудач → lockout 5 мин, после 10 → 15 мин, после 20 → 60 мин.
func TestLogin_LockoutThresholds(t *testing.T) {
	cases := []struct {
		startCount  int
		wantMinutes int
	}{
		{4, 5},
		{9, 15},
		{19, 60},
	}

	for _, tc := range cases {
		t.Run("after_count", func(t *testing.T) {
			d := newTestDeps()
			user := d.users.add("u@test.com", testHash, domain.UserStatusActive)

			// Проставляем накопленные провалы напрямую в репозиторий.
			d.users.mu.Lock()
			d.users.byID[user.ID].FailedLoginCount = tc.startCount
			d.users.byEmail[user.Email].FailedLoginCount = tc.startCount
			d.users.mu.Unlock()

			// Ещё одна неудачная попытка → должна сработать блокировка.
			req := httptest.NewRequest("POST", "/", nil)
			_, err := d.svc.Login(context.Background(), req, user.Email, "wrong-password-xyz")
			if err != auth.ErrInvalidCredentials {
				t.Fatalf("ожидали ErrInvalidCredentials, получили %v", err)
			}

			stored := d.users.get(user.ID)
			if stored.LockoutUntil == nil {
				t.Fatalf("startCount=%d: LockoutUntil должен быть выставлен", tc.startCount)
			}
			wantDur := time.Duration(tc.wantMinutes) * time.Minute
			got := time.Until(*stored.LockoutUntil)
			// Допускаем ±5 секунд погрешности исполнения теста.
			if got < wantDur-5*time.Second || got > wantDur+5*time.Second {
				t.Errorf("startCount=%d: ожидали lockout ~%v, получили %v", tc.startCount, wantDur, got.Round(time.Second))
			}
		})
	}
}

// TestLogin_LockoutCheckedBeforePassword: заблокированный пользователь
// не может войти даже с верным паролем.
func TestLogin_LockoutCheckedBeforePassword(t *testing.T) {
	d := newTestDeps()
	user := d.users.add("u@test.com", testHash, domain.UserStatusActive)

	// Выставляем блокировку напрямую.
	future := time.Now().Add(10 * time.Minute)
	d.users.mu.Lock()
	d.users.byID[user.ID].LockoutUntil = &future
	d.users.byEmail[user.Email].LockoutUntil = &future
	d.users.mu.Unlock()

	req := httptest.NewRequest("POST", "/", nil)
	_, err := d.svc.Login(context.Background(), req, user.Email, testPwd) // верный пароль
	if err != auth.ErrInvalidCredentials {
		t.Fatalf("заблокированный пользователь с верным паролем должен получать ErrInvalidCredentials, получили %v", err)
	}
}

// TestLogin_SuccessResetsFailedCounter: успешный вход сбрасывает счётчик неудач.
func TestLogin_SuccessResetsFailedCounter(t *testing.T) {
	d := newTestDeps()
	user := d.users.add("u@test.com", testHash, domain.UserStatusActive)

	d.users.mu.Lock()
	d.users.byID[user.ID].FailedLoginCount = 3
	d.users.byEmail[user.Email].FailedLoginCount = 3
	d.users.mu.Unlock()

	req := httptest.NewRequest("POST", "/", nil)
	_, err := d.svc.Login(context.Background(), req, user.Email, testPwd)
	if err != nil {
		t.Fatalf("ожидали успех, получили %v", err)
	}

	stored := d.users.get(user.ID)
	if stored.FailedLoginCount != 0 {
		t.Errorf("FailedLoginCount должен быть 0 после успешного входа, получили %d", stored.FailedLoginCount)
	}
}

// TestLogin_WrongPasswordIncrementsCounter: каждый провал увеличивает счётчик.
func TestLogin_WrongPasswordIncrementsCounter(t *testing.T) {
	d := newTestDeps()
	user := d.users.add("u@test.com", testHash, domain.UserStatusActive)

	req := httptest.NewRequest("POST", "/", nil)
	for i := 1; i <= 3; i++ {
		_, _ = d.svc.Login(context.Background(), req, user.Email, "wrong"+string(rune('0'+i)))
		stored := d.users.get(user.ID)
		if stored.FailedLoginCount != i {
			t.Fatalf("попытка %d: ожидали FailedLoginCount=%d, получили %d", i, i, stored.FailedLoginCount)
		}
	}
}

// ---------------------------------------------------------------------------
// Email verification anti-replay
// ---------------------------------------------------------------------------

// TestVerifyEmail_FirstUseSucceeds: первое использование токена активирует аккаунт.
func TestVerifyEmail_FirstUseSucceeds(t *testing.T) {
	d := newTestDeps()
	user := d.users.add("u@test.com", testHash, domain.UserStatusPending)

	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	d.verifications.addToken(user.ID, tokenHash)

	result := d.svc.VerifyEmail(context.Background(), plaintext)
	if !result.Success {
		t.Fatal("первое использование должно быть успешным")
	}

	stored := d.users.get(user.ID)
	if stored.Status != domain.UserStatusActive {
		t.Errorf("статус после верификации: %q, ожидали active", stored.Status)
	}
}

// TestVerifyEmail_AntiReplay: повторное использование того же токена отклоняется.
func TestVerifyEmail_AntiReplay(t *testing.T) {
	d := newTestDeps()
	user := d.users.add("u@test.com", testHash, domain.UserStatusPending)

	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	d.verifications.addToken(user.ID, tokenHash)

	d.svc.VerifyEmail(context.Background(), plaintext) // первое использование

	result := d.svc.VerifyEmail(context.Background(), plaintext) // повтор
	if result.Success {
		t.Fatal("повторное использование токена должно возвращать Success=false")
	}
}

// TestVerifyEmail_InvalidToken: несуществующий токен отклоняется.
func TestVerifyEmail_InvalidToken(t *testing.T) {
	d := newTestDeps()

	result := d.svc.VerifyEmail(context.Background(), "completely-invalid-plaintext")
	if result.Success {
		t.Fatal("несуществующий токен не должен давать Success=true")
	}
}

// ---------------------------------------------------------------------------
// Password reset atomicity
// ---------------------------------------------------------------------------

// TestResetPassword_AtomicEffect: после успешного сброса пароля:
// - хеш пароля изменился
// - все сессии отозваны
// - счётчик неудач сброшен
func TestResetPassword_AtomicEffect(t *testing.T) {
	d := newTestDeps()
	user := d.users.add("u@test.com", testHash, domain.UserStatusActive)

	// Проставляем "плохое" состояние: 5 неудач + 2 активных сессии.
	d.users.mu.Lock()
	d.users.byID[user.ID].FailedLoginCount = 5
	d.users.byEmail[user.Email].FailedLoginCount = 5
	d.users.mu.Unlock()

	d.sessions.add(user.ID, "hash1", time.Now().Add(24*time.Hour), time.Now().Add(7*24*time.Hour))
	d.sessions.add(user.ID, "hash2", time.Now().Add(24*time.Hour), time.Now().Add(7*24*time.Hour))

	if n := d.sessions.countByUser(user.ID); n != 2 {
		t.Fatalf("setup: ожидали 2 сессии, есть %d", n)
	}

	// Выдаём reset-токен.
	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	d.resets.addToken(user.ID, tokenHash)

	req := httptest.NewRequest("POST", "/", nil)
	newPwd := "NewValidPwd999"
	if err := d.svc.ResetPassword(context.Background(), req, plaintext, newPwd); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	stored := d.users.get(user.ID)

	// 1. Пароль изменился.
	ok, _ := password.Verify(newPwd, stored.PasswordHash)
	if !ok {
		t.Error("хеш пароля не обновился")
	}
	// Старый пароль больше не работает.
	ok, _ = password.Verify(testPwd, stored.PasswordHash)
	if ok {
		t.Error("старый пароль всё ещё проходит верификацию")
	}

	// 2. Все сессии удалены.
	if n := d.sessions.countByUser(user.ID); n != 0 {
		t.Errorf("ожидали 0 сессий, осталось %d", n)
	}

	// 3. Счётчик неудач сброшен.
	if stored.FailedLoginCount != 0 {
		t.Errorf("FailedLoginCount должен быть 0, получили %d", stored.FailedLoginCount)
	}
}

// TestResetPassword_TokenUsedOnce: повторное использование reset-токена отклоняется.
func TestResetPassword_TokenUsedOnce(t *testing.T) {
	d := newTestDeps()
	user := d.users.add("u@test.com", testHash, domain.UserStatusActive)

	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	d.resets.addToken(user.ID, tokenHash)

	req := httptest.NewRequest("POST", "/", nil)
	if err := d.svc.ResetPassword(context.Background(), req, plaintext, "NewValidPwd999"); err != nil {
		t.Fatalf("первый сброс: %v", err)
	}

	if err := d.svc.ResetPassword(context.Background(), req, plaintext, "AnotherPwd999"); err != auth.ErrInvalidResetToken {
		t.Fatalf("повторный сброс: ожидали ErrInvalidResetToken, получили %v", err)
	}
}

// ---------------------------------------------------------------------------
// Session TTL
// ---------------------------------------------------------------------------

// TestSession_IdleRenewal_Under12h: если до истечения idle TTL < 12ч — продлевается.
func TestSession_IdleRenewal_Under12h(t *testing.T) {
	d := newTestDeps()
	user := d.users.add("u@test.com", testHash, domain.UserStatusActive)

	// Сессия истекает через 6 ч — меньше 12 ч, должна продлиться.
	sess := d.sessions.add(user.ID, "tok1",
		time.Now().Add(6*time.Hour),
		time.Now().Add(7*24*time.Hour),
	)

	newIdle, err := d.svc.TouchSession(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if newIdle == nil {
		t.Fatal("ожидали обновления idle TTL (< 12ч до истечения), получили nil")
	}
	if time.Until(*newIdle) < 23*time.Hour {
		t.Errorf("новый idle TTL слишком мал: %v", time.Until(*newIdle))
	}
}

// TestSession_IdleRenewal_Over12h: если до истечения idle TTL > 12ч — не продлевается.
func TestSession_IdleRenewal_Over12h(t *testing.T) {
	d := newTestDeps()
	user := d.users.add("u@test.com", testHash, domain.UserStatusActive)

	// Сессия истекает через 18 ч — больше 12 ч, не должна продлеваться.
	sess := d.sessions.add(user.ID, "tok2",
		time.Now().Add(18*time.Hour),
		time.Now().Add(7*24*time.Hour),
	)

	newIdle, err := d.svc.TouchSession(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if newIdle != nil {
		t.Errorf("idle TTL не должен продлеваться (> 12ч до истечения), получили %v", newIdle)
	}
}

// TestSession_AbsoluteTTL_Enforced: сессия с истёкшим absolute TTL недействительна.
func TestSession_AbsoluteTTL_Enforced(t *testing.T) {
	d := newTestDeps()
	user := d.users.add("u@test.com", testHash, domain.UserStatusActive)

	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	// Absolute TTL в прошлом, idle ещё активен.
	d.sessions.add(user.ID, tokenHash,
		time.Now().Add(24*time.Hour),
		time.Now().Add(-1*time.Second), // истёк
	)

	_, err = d.svc.GetSessionByToken(context.Background(), plaintext)
	if err != auth.ErrSessionNotFound {
		t.Fatalf("ожидали ErrSessionNotFound для истёкшей сессии, получили %v", err)
	}
}

// TestSession_IdleTTL_Enforced: сессия с истёкшим idle TTL недействительна.
func TestSession_IdleTTL_Enforced(t *testing.T) {
	d := newTestDeps()
	user := d.users.add("u@test.com", testHash, domain.UserStatusActive)

	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	// Idle TTL в прошлом, absolute ещё активен.
	d.sessions.add(user.ID, tokenHash,
		time.Now().Add(-1*time.Second), // истёк
		time.Now().Add(7*24*time.Hour),
	)

	_, err = d.svc.GetSessionByToken(context.Background(), plaintext)
	if err != auth.ErrSessionNotFound {
		t.Fatalf("ожидали ErrSessionNotFound для истёкшей сессии, получили %v", err)
	}
}
