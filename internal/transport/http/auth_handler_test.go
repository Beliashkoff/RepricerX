package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
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
	autosvc "github.com/Beliashkoff/RepricerX/internal/service/auth"
	transport "github.com/Beliashkoff/RepricerX/internal/transport/http"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// testPasswordPlain — пароль для тестового пользователя.
const testPasswordPlain = "ValidPassword12"

// testPasswordHash вычисляется один раз в TestMain.
var testPasswordHash string

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	h, err := password.Hash(testPasswordPlain)
	if err != nil {
		panic("auth_handler_test: hash password: " + err.Error())
	}
	testPasswordHash = h
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Fake repositories
// ---------------------------------------------------------------------------

type fakeUsersRepo struct {
	mu      sync.Mutex
	byEmail map[string]*domain.User
	byID    map[uuid.UUID]*domain.User
}

func newFakeUsers() *fakeUsersRepo {
	return &fakeUsersRepo{
		byEmail: make(map[string]*domain.User),
		byID:    make(map[uuid.UUID]*domain.User),
	}
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
		r.byEmail[u.Email].DisplayName = name
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

// addActiveUser — вспомогательный метод для тестов.
func (r *fakeUsersRepo) addActiveUser(email, passwordHash, displayName string) *domain.User {
	u := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
		Status:       domain.UserStatusActive,
		CreatedAt:    time.Now(),
	}
	r.mu.Lock()
	r.byEmail[email] = u
	r.byID[u.ID] = u
	r.mu.Unlock()
	return u
}

// ---------------------------------------------------------------------------

type fakeSessionsRepo struct {
	mu       sync.Mutex
	sessions map[string]*domain.Session // keyed by token hash
}

func newFakeSessions() *fakeSessionsRepo {
	return &fakeSessionsRepo{sessions: make(map[string]*domain.Session)}
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
	c := *s
	return &c, nil
}

func (r *fakeSessionsRepo) TouchIdleIfNeeded(_ context.Context, _ uuid.UUID, _ time.Time) (*time.Time, error) {
	return nil, nil
}

func (r *fakeSessionsRepo) TouchLastSeen(_ context.Context, _ uuid.UUID, _ time.Time) error {
	return nil
}

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

// addSession добавляет сессию напрямую для тестового setup.
func (r *fakeSessionsRepo) addSession(userID uuid.UUID, tokenHash string) *domain.Session {
	s := &domain.Session{
		ID:                uuid.New(),
		UserID:            userID,
		TokenHash:         tokenHash,
		CreatedAt:         time.Now(),
		LastSeenAt:        time.Now(),
		IdleExpiresAt:     time.Now().Add(24 * time.Hour),
		AbsoluteExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	r.mu.Lock()
	r.sessions[tokenHash] = s
	r.mu.Unlock()
	return s
}

// ---------------------------------------------------------------------------

type verToken struct {
	id        uuid.UUID
	userID    uuid.UUID
	expiresAt time.Time
	usedAt    *time.Time
}

type fakeVerificationsRepo struct {
	mu     sync.Mutex
	tokens map[string]*verToken // keyed by token hash
	users  *fakeUsersRepo
}

func newFakeVerifications(users ...*fakeUsersRepo) *fakeVerificationsRepo {
	r := &fakeVerificationsRepo{tokens: make(map[string]*verToken)}
	if len(users) > 0 {
		r.users = users[0]
	}
	return r
}

func (r *fakeVerificationsRepo) Create(_ context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[tokenHash] = &verToken{id: uuid.New(), userID: userID, expiresAt: expiresAt}
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

func (r *fakeVerificationsRepo) ConsumeAndActivate(_ context.Context, tokenHash string) (uuid.UUID, error) {
	r.mu.Lock()
	v, ok := r.tokens[tokenHash]
	if !ok || v.usedAt != nil || time.Now().After(v.expiresAt) {
		r.mu.Unlock()
		return uuid.Nil, repository.ErrNotFound
	}
	now := time.Now()
	v.usedAt = &now
	userID := v.userID
	r.mu.Unlock()

	if r.users != nil {
		r.users.mu.Lock()
		defer r.users.mu.Unlock()
		u, ok := r.users.byID[userID]
		if !ok || u.Status != domain.UserStatusPending {
			return uuid.Nil, repository.ErrNotFound
		}
		u.Status = domain.UserStatusActive
		r.users.byEmail[u.Email].Status = domain.UserStatusActive
	}
	return userID, nil
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

type resetToken struct {
	id        uuid.UUID
	userID    uuid.UUID
	expiresAt time.Time
	usedAt    *time.Time
}

type fakeResetTokensRepo struct {
	mu       sync.Mutex
	tokens   map[string]*resetToken // keyed by token hash
	users    *fakeUsersRepo
	sessions *fakeSessionsRepo
}

func newFakeResetTokens(users *fakeUsersRepo, sessions *fakeSessionsRepo) *fakeResetTokensRepo {
	return &fakeResetTokensRepo{
		tokens:   make(map[string]*resetToken),
		users:    users,
		sessions: sessions,
	}
}

func (r *fakeResetTokensRepo) Issue(_ context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// invalidate existing tokens for this user
	for k, t := range r.tokens {
		if t.userID == userID {
			delete(r.tokens, k)
		}
	}
	r.tokens[tokenHash] = &resetToken{id: uuid.New(), userID: userID, expiresAt: expiresAt}
	return nil
}

func (r *fakeResetTokensRepo) Consume(_ context.Context, tokenHash, newPasswordHash string) (uuid.UUID, int64, error) {
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

	_ = r.users.UpdatePasswordHash(context.Background(), userID, newPasswordHash)
	revoked, _ := r.sessions.DeleteByUserID(context.Background(), userID)
	return userID, revoked, nil
}

func (r *fakeResetTokensRepo) DeleteExpired(_ context.Context) (int64, error) { return 0, nil }

// addResetToken добавляет токен напрямую для тестового setup.
func (r *fakeResetTokensRepo) addResetToken(userID uuid.UUID, tokenHash string) {
	r.mu.Lock()
	r.tokens[tokenHash] = &resetToken{
		id:        uuid.New(),
		userID:    userID,
		expiresAt: time.Now().Add(time.Hour),
	}
	r.mu.Unlock()
}

// ---------------------------------------------------------------------------

type fakeMailer struct {
	mu   sync.Mutex
	sent []string
}

func (m *fakeMailer) Send(_ context.Context, to, _, _, _ string) error {
	m.mu.Lock()
	m.sent = append(m.sent, to)
	m.mu.Unlock()
	return nil
}

// ---------------------------------------------------------------------------
// Test setup helpers
// ---------------------------------------------------------------------------

type testDeps struct {
	svc      *autosvc.Service
	users    *fakeUsersRepo
	sessions *fakeSessionsRepo
	resets   *fakeResetTokensRepo
	mailer   *fakeMailer
	audit    *auditlog.Logger
}

func newTestDeps() *testDeps {
	users := newFakeUsers()
	sessions := newFakeSessions()
	verifications := newFakeVerifications(users)
	resets := newFakeResetTokens(users, sessions)
	m := &fakeMailer{}
	audit := auditlog.New(slog.New(slog.NewTextHandler(io.Discard, nil)))

	svc := autosvc.New(
		users, sessions, verifications, resets, m, audit,
		autosvc.Config{
			IdleTTL:     24 * time.Hour,
			AbsoluteTTL: 7 * 24 * time.Hour,
		},
	)
	return &testDeps{
		svc:      svc,
		users:    users,
		sessions: sessions,
		resets:   resets,
		mailer:   m,
		audit:    audit,
	}
}

func (d *testDeps) engine() *gin.Engine {
	r := gin.New()
	authH := transport.NewAuthHandler(d.svc, false, "http://localhost")

	pub := r.Group("/api/auth")
	pub.POST("/register", authH.Register)
	pub.POST("/login", authH.Login)
	pub.GET("/verify", authH.VerifyEmail)
	pub.POST("/verification/resend", authH.ResendVerification)
	pub.POST("/password/forgot", authH.ForgotPassword)
	pub.POST("/password/reset", authH.ResetPassword)

	requireAuth := transport.RequireAuth(d.svc, d.audit, false, false)
	protected := r.Group("/api", requireAuth)
	protected.GET("/auth/me", authH.Me)
	protected.POST("/auth/logout", authH.Logout)
	protected.PATCH("/auth/me", authH.UpdateMe)

	return r
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func postJSON(r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func postJSONWithCookie(r *gin.Engine, path, cookieValue string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "rx_session", Value: cookieValue})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func getWithCookie(r *gin.Engine, path, cookieValue string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookieValue != "" {
		req.AddCookie(&http.Cookie{Name: "rx_session", Value: cookieValue})
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func sessionCookieFrom(w *httptest.ResponseRecorder) string {
	for _, c := range w.Result().Cookies() {
		if c.Name == "rx_session" {
			return c.Value
		}
	}
	return ""
}

func bodyCode(w *httptest.ResponseRecorder) string {
	var env struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return env.Error.Code
}

// ---------------------------------------------------------------------------
// Register tests
// ---------------------------------------------------------------------------

func TestRegister_Success(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	w := postJSON(r, "/api/auth/register", map[string]string{
		"email": "alice@example.com", "password": testPasswordPlain, "displayName": "Alice",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("ожидали 201, получили %d: %s", w.Code, w.Body)
	}

	var resp struct{ Email string }
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Email != "alice@example.com" {
		t.Errorf("email в ответе: %q", resp.Email)
	}
}

func TestRegister_BadJSON(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("ожидали 400, получили %d", w.Code)
	}
}

func TestRegister_MissingField(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	w := postJSON(r, "/api/auth/register", map[string]string{
		"email": "alice@example.com",
		// password и displayName отсутствуют
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ожидали 400, получили %d", w.Code)
	}
}

func TestRegister_WeakPassword(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	w := postJSON(r, "/api/auth/register", map[string]string{
		"email": "bob@example.com", "password": "short1", "displayName": "Bob",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ожидали 400, получили %d", w.Code)
	}
	if code := bodyCode(w); code != "weak_password" {
		t.Errorf("ожидали weak_password, получили %q", code)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	w := postJSON(r, "/api/auth/register", map[string]string{
		"email": "not-an-email", "password": testPasswordPlain, "displayName": "Bob",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ожидали 400, получили %d", w.Code)
	}
	if code := bodyCode(w); code != "invalid_email" {
		t.Errorf("ожидали invalid_email, получили %q", code)
	}
}

func TestRegister_EmailTaken(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	body := map[string]string{
		"email": "taken@example.com", "password": testPasswordPlain, "displayName": "X",
	}
	postJSON(r, "/api/auth/register", body) // первая регистрация
	w := postJSON(r, "/api/auth/register", body) // повтор

	if w.Code != http.StatusConflict {
		t.Fatalf("ожидали 409, получили %d", w.Code)
	}
	if code := bodyCode(w); code != "email_taken" {
		t.Errorf("ожидали email_taken, получили %q", code)
	}
}

// ---------------------------------------------------------------------------
// Login tests
// ---------------------------------------------------------------------------

func TestLogin_Success(t *testing.T) {
	d := newTestDeps()
	d.users.addActiveUser("alice@example.com", testPasswordHash, "Alice")
	r := d.engine()

	w := postJSON(r, "/api/auth/login", map[string]string{
		"email": "alice@example.com", "password": testPasswordPlain,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("ожидали 200, получили %d: %s", w.Code, w.Body)
	}

	cookie := sessionCookieFrom(w)
	if cookie == "" {
		t.Error("rx_session cookie не установлена")
	}

	var resp struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Email != "alice@example.com" {
		t.Errorf("email: %q", resp.Email)
	}
}

func TestLogin_BadJSON(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString("{"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Handler returns 401 (not 400) for bad JSON at login — security design.
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("ожидали 401, получили %d", w.Code)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	d := newTestDeps()
	d.users.addActiveUser("alice@example.com", testPasswordHash, "Alice")
	r := d.engine()

	w := postJSON(r, "/api/auth/login", map[string]string{
		"email": "alice@example.com", "password": "WrongPassword99",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("ожидали 401, получили %d", w.Code)
	}
	if code := bodyCode(w); code != "invalid_credentials" {
		t.Errorf("ожидали invalid_credentials, получили %q", code)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	w := postJSON(r, "/api/auth/login", map[string]string{
		"email": "nobody@example.com", "password": testPasswordPlain,
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("ожидали 401, получили %d", w.Code)
	}
}

func TestLogin_NotVerifiedUser(t *testing.T) {
	d := newTestDeps()
	// пользователь в статусе pending_verification
	u := &domain.User{
		ID:           uuid.New(),
		Email:        "pending@example.com",
		PasswordHash: testPasswordHash,
		DisplayName:  "Pending",
		Status:       domain.UserStatusPending,
	}
	d.users.byEmail[u.Email] = u
	d.users.byID[u.ID] = u
	r := d.engine()

	w := postJSON(r, "/api/auth/login", map[string]string{
		"email": "pending@example.com", "password": testPasswordPlain,
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("ожидали 401, получили %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Logout tests
// ---------------------------------------------------------------------------

func TestLogout_WithSession(t *testing.T) {
	d := newTestDeps()
	user := d.users.addActiveUser("alice@example.com", testPasswordHash, "Alice")

	// создаём сессию вручную, чтобы не запускать argon2 в Login
	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	d.sessions.addSession(user.ID, tokenHash)
	r := d.engine()

	w := postJSONWithCookie(r, "/api/auth/logout", plaintext, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("ожидали 204, получили %d", w.Code)
	}

	// сессия должна быть удалена
	d.sessions.mu.Lock()
	_, stillExists := d.sessions.sessions[tokenHash]
	d.sessions.mu.Unlock()
	if stillExists {
		t.Error("сессия не была удалена после logout")
	}
}

func TestLogout_WithoutCookie(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Logout регистрируется под RequireAuth, поэтому запрос без cookie отклоняется
	// middleware до вызова хендлера — ожидаем 401, а не 204.
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("ожидали 401, получили %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Me (RequireAuth middleware + handler) tests
// ---------------------------------------------------------------------------

func TestMe_Authenticated(t *testing.T) {
	d := newTestDeps()
	user := d.users.addActiveUser("alice@example.com", testPasswordHash, "Alice")

	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	d.sessions.addSession(user.ID, tokenHash)
	r := d.engine()

	w := getWithCookie(r, "/api/auth/me", plaintext)
	if w.Code != http.StatusOK {
		t.Fatalf("ожидали 200, получили %d: %s", w.Code, w.Body)
	}

	var resp struct {
		Email  string `json:"email"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Email != "alice@example.com" {
		t.Errorf("email: %q", resp.Email)
	}
	if resp.Status != domain.UserStatusActive {
		t.Errorf("status: %q", resp.Status)
	}
}

func TestMe_NoCookie(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	w := getWithCookie(r, "/api/auth/me", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("ожидали 401, получили %d", w.Code)
	}
}

func TestMe_InvalidSession(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	w := getWithCookie(r, "/api/auth/me", "totally-invalid-token")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("ожидали 401, получили %d", w.Code)
	}
}

func TestMe_BlockedUser(t *testing.T) {
	d := newTestDeps()
	user := d.users.addActiveUser("blocked@example.com", testPasswordHash, "Blocked")
	// меняем статус на blocked после создания сессии
	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	d.sessions.addSession(user.ID, tokenHash)

	d.users.mu.Lock()
	d.users.byID[user.ID].Status = domain.UserStatusBlocked
	d.users.byEmail[user.Email].Status = domain.UserStatusBlocked
	d.users.mu.Unlock()

	r := d.engine()

	w := getWithCookie(r, "/api/auth/me", plaintext)
	if w.Code != http.StatusForbidden {
		t.Fatalf("ожидали 403, получили %d", w.Code)
	}
	if code := bodyCode(w); code != "user_blocked" {
		t.Errorf("ожидали user_blocked, получили %q", code)
	}
}

// ---------------------------------------------------------------------------
// UpdateMe tests
// ---------------------------------------------------------------------------

func TestUpdateMe_Success(t *testing.T) {
	d := newTestDeps()
	user := d.users.addActiveUser("alice@example.com", testPasswordHash, "Alice")

	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	d.sessions.addSession(user.ID, tokenHash)
	r := d.engine()

	b, _ := json.Marshal(map[string]string{"displayName": "Alice Updated"})
	req := httptest.NewRequest(http.MethodPatch, "/api/auth/me", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "rx_session", Value: plaintext})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ожидали 200, получили %d: %s", w.Code, w.Body)
	}

	var resp struct{ DisplayName string `json:"displayName"` }
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.DisplayName != "Alice Updated" {
		t.Errorf("displayName: %q", resp.DisplayName)
	}
}

func TestUpdateMe_BadJSON(t *testing.T) {
	d := newTestDeps()
	user := d.users.addActiveUser("alice@example.com", testPasswordHash, "Alice")

	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	d.sessions.addSession(user.ID, tokenHash)
	r := d.engine()

	req := httptest.NewRequest(http.MethodPatch, "/api/auth/me", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "rx_session", Value: plaintext})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("ожидали 400, получили %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// VerifyEmail tests
// ---------------------------------------------------------------------------

func TestVerifyEmail_ValidToken(t *testing.T) {
	d := newTestDeps()
	// Создаём pending пользователя с токеном верификации.
	u := &domain.User{
		ID: uuid.New(), Email: "new@example.com",
		PasswordHash: testPasswordHash, DisplayName: "New",
		Status: domain.UserStatusPending,
	}
	d.users.byEmail[u.Email] = u
	d.users.byID[u.ID] = u

	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	// перестраиваем сервис с нашим vRepo (передаём d.users для ConsumeAndActivate)
	vRepo := newFakeVerifications(d.users)
	_ = vRepo.Create(context.Background(), u.ID, tokenHash, time.Now().Add(time.Hour))

	svc := autosvc.New(
		d.users, d.sessions, vRepo, d.resets, d.mailer, d.audit,
		autosvc.Config{IdleTTL: 24 * time.Hour, AbsoluteTTL: 7 * 24 * time.Hour},
	)
	authH := transport.NewAuthHandler(svc, false, "http://localhost")
	r := gin.New()
	r.GET("/api/auth/verify", authH.VerifyEmail)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/verify?token="+plaintext, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("ожидали 302, получили %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "http://localhost/login?verified=1" {
		t.Errorf("Location: %q", loc)
	}
}

func TestVerifyEmail_InvalidToken(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/verify?token=invalid-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("ожидали 302, получили %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "http://localhost/login?verified=0" {
		t.Errorf("Location: %q", loc)
	}
}

// ---------------------------------------------------------------------------
// ForgotPassword tests
// ---------------------------------------------------------------------------

func TestForgotPassword_AlwaysAccepted(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	for _, email := range []string{"exists@example.com", "nobody@example.com", ""} {
		w := postJSON(r, "/api/auth/password/forgot", map[string]string{"email": email})
		if w.Code != http.StatusAccepted {
			t.Errorf("email=%q: ожидали 202, получили %d", email, w.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// ResetPassword tests
// ---------------------------------------------------------------------------

func TestResetPassword_PasswordMismatch(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	w := postJSON(r, "/api/auth/password/reset", map[string]string{
		"token": "sometoken", "password": testPasswordPlain, "passwordConfirmation": "DifferentPassword12",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ожидали 400, получили %d: %s", w.Code, w.Body)
	}
	if code := bodyCode(w); code != "password_mismatch" {
		t.Errorf("ожидали password_mismatch, получили %q", code)
	}
}

func TestResetPassword_MissingPassword(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	w := postJSON(r, "/api/auth/password/reset", map[string]string{
		"token": "sometoken",
		// password отсутствует
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ожидали 400, получили %d", w.Code)
	}
}

func TestResetPassword_InvalidToken(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	w := postJSON(r, "/api/auth/password/reset", map[string]string{
		"token": "bad-reset-token", "password": testPasswordPlain, "passwordConfirmation": testPasswordPlain,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ожидали 400, получили %d: %s", w.Code, w.Body)
	}
	if code := bodyCode(w); code != "invalid_reset_token" {
		t.Errorf("ожидали invalid_reset_token, получили %q", code)
	}
}

func TestResetPassword_Success(t *testing.T) {
	d := newTestDeps()
	user := d.users.addActiveUser("alice@example.com", testPasswordHash, "Alice")

	plaintext, tokenHash, err := token.Generate()
	if err != nil {
		t.Fatal(err)
	}
	d.resets.addResetToken(user.ID, tokenHash)
	r := d.engine()

	w := postJSON(r, "/api/auth/password/reset", map[string]string{
		"token":                plaintext,
		"password":             "NewValidPassword12",
		"passwordConfirmation": "NewValidPassword12",
	})
	if w.Code != http.StatusNoContent {
		t.Fatalf("ожидали 204, получили %d: %s", w.Code, w.Body)
	}
}

func TestResetPassword_BadJSON(t *testing.T) {
	d := newTestDeps()
	r := d.engine()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/password/reset", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("ожидали 400, получили %d", w.Code)
	}
}
