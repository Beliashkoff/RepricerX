package auth_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration/oauth"
	"github.com/Beliashkoff/RepricerX/internal/pkg/oauthstate"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	authsvc "github.com/Beliashkoff/RepricerX/internal/service/auth"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Fake OAuth provider
// ---------------------------------------------------------------------------

type fakeProvider struct {
	name domain.OAuthProvider
	// возвращаемые токен/юзер; ошибки задаются через setError
	tokenForCode map[string]string
	userForToken map[string]domain.OAuthUserInfo
	exchangeErr  error
	fetchErr     error

	mu                   sync.Mutex
	lastCode             string
	lastVerifier         string
	lastAccessToken      string
	lastAuthorizationURL string
}

func newFakeProvider(name domain.OAuthProvider) *fakeProvider {
	return &fakeProvider{
		name:         name,
		tokenForCode: make(map[string]string),
		userForToken: make(map[string]domain.OAuthUserInfo),
	}
}

func (p *fakeProvider) Name() domain.OAuthProvider { return p.name }

func (p *fakeProvider) AuthorizationURL(state, codeChallenge string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	u := "https://fake/" + string(p.name) + "/auth?state=" + state + "&code_challenge=" + codeChallenge
	p.lastAuthorizationURL = u
	return u
}

func (p *fakeProvider) Exchange(_ context.Context, code, codeVerifier string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastCode = code
	p.lastVerifier = codeVerifier
	if p.exchangeErr != nil {
		return "", p.exchangeErr
	}
	tok, ok := p.tokenForCode[code]
	if !ok {
		return "", oauth.ErrProviderRejected
	}
	p.lastAccessToken = tok
	return tok, nil
}

func (p *fakeProvider) FetchUser(_ context.Context, accessToken string) (domain.OAuthUserInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fetchErr != nil {
		return domain.OAuthUserInfo{}, p.fetchErr
	}
	info, ok := p.userForToken[accessToken]
	if !ok {
		return domain.OAuthUserInfo{}, oauth.ErrProviderRejected
	}
	return info, nil
}

// ---------------------------------------------------------------------------
// Fake oauthstate.Store
// ---------------------------------------------------------------------------

type fakeStore struct {
	mu     sync.Mutex
	states map[string]oauthstate.StatePayload
	links  map[string]oauthstate.LinkPayload
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		states: make(map[string]oauthstate.StatePayload),
		links:  make(map[string]oauthstate.LinkPayload),
	}
}

func (s *fakeStore) SaveState(_ context.Context, state string, p oauthstate.StatePayload, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = p
	return nil
}

func (s *fakeStore) ConsumeState(_ context.Context, state string) (oauthstate.StatePayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.states[state]
	if !ok {
		return oauthstate.StatePayload{}, oauthstate.ErrNotFound
	}
	delete(s.states, state)
	return p, nil
}

func (s *fakeStore) SaveLink(_ context.Context, token string, p oauthstate.LinkPayload, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links[token] = p
	return nil
}

func (s *fakeStore) ConsumeLink(_ context.Context, token string) (oauthstate.LinkPayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.links[token]
	if !ok {
		return oauthstate.LinkPayload{}, oauthstate.ErrNotFound
	}
	delete(s.links, token)
	return p, nil
}

// ---------------------------------------------------------------------------
// Fake OAuth identities repo
// ---------------------------------------------------------------------------

type fakeOAuthIdentitiesRepo struct {
	mu             sync.Mutex
	byProviderExtID map[string]*domain.OAuthIdentity // key: "provider|external_id"
}

func newFakeOAuthIdentities() *fakeOAuthIdentitiesRepo {
	return &fakeOAuthIdentitiesRepo{
		byProviderExtID: make(map[string]*domain.OAuthIdentity),
	}
}

func keyFor(p domain.OAuthProvider, id string) string { return string(p) + "|" + id }

func (r *fakeOAuthIdentitiesRepo) Create(_ context.Context, i *domain.OAuthIdentity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := keyFor(i.Provider, i.ExternalID)
	if _, exists := r.byProviderExtID[k]; exists {
		return repository.ErrDuplicate
	}
	c := *i
	r.byProviderExtID[k] = &c
	return nil
}

func (r *fakeOAuthIdentitiesRepo) GetByProviderAndExternalID(_ context.Context, p domain.OAuthProvider, externalID string) (*domain.OAuthIdentity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.byProviderExtID[keyFor(p, externalID)]
	if !ok {
		return nil, repository.ErrNotFound
	}
	c := *v
	return &c, nil
}

func (r *fakeOAuthIdentitiesRepo) TouchLastLogin(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.byProviderExtID {
		if v.ID == id {
			v.LastLoginAt = time.Now()
			return nil
		}
	}
	return nil
}

func (r *fakeOAuthIdentitiesRepo) ListByUserID(_ context.Context, userID uuid.UUID) ([]*domain.OAuthIdentity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.OAuthIdentity
	for _, v := range r.byProviderExtID {
		if v.UserID == userID {
			c := *v
			out = append(out, &c)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type oauthDeps struct {
	*testDeps
	store    *fakeStore
	provider *fakeProvider
	idents   *fakeOAuthIdentitiesRepo
}

func newOAuthTestDeps(t *testing.T, providerName domain.OAuthProvider) *oauthDeps {
	t.Helper()
	td := newTestDeps()
	store := newFakeStore()
	provider := newFakeProvider(providerName)
	idents := newFakeOAuthIdentities()
	td.svc.AttachOAuth(
		map[domain.OAuthProvider]oauth.Provider{providerName: provider},
		store,
		idents,
	)
	return &oauthDeps{testDeps: td, store: store, provider: provider, idents: idents}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestBeginOAuth_GeneratesURLAndSavesState(t *testing.T) {
	d := newOAuthTestDeps(t, domain.OAuthProviderYandex)
	url, err := d.svc.BeginOAuth(context.Background(), domain.OAuthProviderYandex)
	if err != nil {
		t.Fatalf("BeginOAuth: %v", err)
	}
	if url == "" {
		t.Fatalf("ожидался непустой authURL")
	}
	d.store.mu.Lock()
	if len(d.store.states) != 1 {
		t.Fatalf("ожидался 1 сохранённый state, got %d", len(d.store.states))
	}
	var saved oauthstate.StatePayload
	for _, v := range d.store.states {
		saved = v
	}
	d.store.mu.Unlock()
	if saved.Provider != domain.OAuthProviderYandex {
		t.Fatalf("ожидался provider=yandex, got %q", saved.Provider)
	}
	if len(saved.CodeVerifier) < 32 {
		t.Fatalf("ожидался verifier ≥ 32 символов, got %d", len(saved.CodeVerifier))
	}
}

func TestBeginOAuth_UnknownProvider(t *testing.T) {
	d := newOAuthTestDeps(t, domain.OAuthProviderYandex)
	_, err := d.svc.BeginOAuth(context.Background(), domain.OAuthProviderVK)
	if !errors.Is(err, authsvc.ErrUnknownOAuthProvider) {
		t.Fatalf("ожидалось ErrUnknownOAuthProvider, got %v", err)
	}
}

func TestCompleteOAuth_CreatesNewUserAndSession(t *testing.T) {
	d := newOAuthTestDeps(t, domain.OAuthProviderYandex)
	state := "state-1"
	verifier := "verifier-1"
	_ = d.store.SaveState(context.Background(), state, oauthstate.StatePayload{
		Provider: domain.OAuthProviderYandex, CodeVerifier: verifier,
	}, time.Minute)

	d.provider.tokenForCode["code-1"] = "tok-1"
	d.provider.userForToken["tok-1"] = domain.OAuthUserInfo{
		ProviderUserID: "ya-42", Email: "new@example.com", DisplayName: "Новый",
	}

	r := httptest.NewRequest("GET", "/cb", nil)
	login, link, err := d.svc.CompleteOAuth(context.Background(), r, state, "code-1")
	if err != nil {
		t.Fatalf("CompleteOAuth: %v", err)
	}
	if link != nil {
		t.Fatalf("не ожидался OAuthLinkRequest")
	}
	if login == nil || login.User == nil {
		t.Fatalf("ожидался успешный LoginResult")
	}
	if login.User.Email != "new@example.com" {
		t.Fatalf("email = %q, ожидался new@example.com", login.User.Email)
	}
	if login.User.PasswordHash != "" {
		t.Fatalf("OAuth-only пользователь должен иметь пустой PasswordHash")
	}
	if login.User.Status != domain.UserStatusActive {
		t.Fatalf("status = %q, ожидался active", login.User.Status)
	}
	if d.sessions.countByUser(login.User.ID) != 1 {
		t.Fatalf("должна быть создана 1 сессия")
	}
	// identity создана?
	id, ierr := d.idents.GetByProviderAndExternalID(context.Background(), domain.OAuthProviderYandex, "ya-42")
	if ierr != nil {
		t.Fatalf("identity не создана: %v", ierr)
	}
	if id.UserID != login.User.ID {
		t.Fatalf("identity.UserID не совпадает")
	}
}

func TestCompleteOAuth_RepeatLoginExistingIdentity(t *testing.T) {
	d := newOAuthTestDeps(t, domain.OAuthProviderYandex)

	// Уже есть юзер + identity (например, после прошлого логина).
	user := d.users.add("alice@example.com", "", domain.UserStatusActive)
	_ = d.idents.Create(context.Background(), &domain.OAuthIdentity{
		ID: uuid.New(), UserID: user.ID, Provider: domain.OAuthProviderYandex,
		ExternalID: "ya-7", Email: user.Email, CreatedAt: time.Now(), LastLoginAt: time.Now(),
	})

	state := "state-2"
	_ = d.store.SaveState(context.Background(), state, oauthstate.StatePayload{
		Provider: domain.OAuthProviderYandex, CodeVerifier: "v",
	}, time.Minute)
	d.provider.tokenForCode["c"] = "t"
	d.provider.userForToken["t"] = domain.OAuthUserInfo{ProviderUserID: "ya-7", Email: user.Email}

	login, link, err := d.svc.CompleteOAuth(context.Background(), httptest.NewRequest("GET", "/cb", nil), state, "c")
	if err != nil {
		t.Fatalf("CompleteOAuth: %v", err)
	}
	if link != nil {
		t.Fatalf("не ожидался OAuthLinkRequest")
	}
	if login.User.ID != user.ID {
		t.Fatalf("должны вернуть существующего юзера, got %v", login.User.ID)
	}
}

func TestCompleteOAuth_EmailConflictReturnsLinkRequest(t *testing.T) {
	d := newOAuthTestDeps(t, domain.OAuthProviderYandex)

	// Email уже занят email/password-аккаунтом (PasswordHash непустой).
	existing := d.users.add("bob@example.com", testHash, domain.UserStatusActive)

	state := "state-3"
	_ = d.store.SaveState(context.Background(), state, oauthstate.StatePayload{
		Provider: domain.OAuthProviderYandex, CodeVerifier: "v",
	}, time.Minute)
	d.provider.tokenForCode["c"] = "t"
	d.provider.userForToken["t"] = domain.OAuthUserInfo{
		ProviderUserID: "ya-99", Email: "bob@example.com", DisplayName: "Boby",
	}

	login, link, err := d.svc.CompleteOAuth(context.Background(), httptest.NewRequest("GET", "/cb", nil), state, "c")
	if err != nil {
		t.Fatalf("CompleteOAuth: %v", err)
	}
	if login != nil {
		t.Fatalf("не ожидался LoginResult — нужно подтверждение")
	}
	if link == nil {
		t.Fatalf("ожидался OAuthLinkRequest")
	}
	if link.Email != existing.Email {
		t.Fatalf("link.Email = %q, ожидался %q", link.Email, existing.Email)
	}
	if link.Provider != domain.OAuthProviderYandex {
		t.Fatalf("link.Provider = %q", link.Provider)
	}
	if link.LinkToken == "" {
		t.Fatalf("link.LinkToken пустой")
	}
	// identity ещё НЕ должна быть создана.
	_, ierr := d.idents.GetByProviderAndExternalID(context.Background(), domain.OAuthProviderYandex, "ya-99")
	if !errors.Is(ierr, repository.ErrNotFound) {
		t.Fatalf("identity не должна существовать до подтверждения")
	}
}

func TestCompleteOAuth_InvalidState(t *testing.T) {
	d := newOAuthTestDeps(t, domain.OAuthProviderYandex)
	_, _, err := d.svc.CompleteOAuth(context.Background(), httptest.NewRequest("GET", "/cb", nil), "bogus", "c")
	if !errors.Is(err, authsvc.ErrInvalidOAuthState) {
		t.Fatalf("ожидалось ErrInvalidOAuthState, got %v", err)
	}
}

func TestCompleteOAuth_ProviderFailure(t *testing.T) {
	d := newOAuthTestDeps(t, domain.OAuthProviderYandex)
	state := "state-fail"
	_ = d.store.SaveState(context.Background(), state, oauthstate.StatePayload{
		Provider: domain.OAuthProviderYandex, CodeVerifier: "v",
	}, time.Minute)
	d.provider.exchangeErr = oauth.ErrProviderUnavailable

	_, _, err := d.svc.CompleteOAuth(context.Background(), httptest.NewRequest("GET", "/cb", nil), state, "c")
	if !errors.Is(err, authsvc.ErrOAuthProviderFailed) {
		t.Fatalf("ожидалось ErrOAuthProviderFailed, got %v", err)
	}
}

func TestConfirmOAuthLink_SuccessWithRightPassword(t *testing.T) {
	d := newOAuthTestDeps(t, domain.OAuthProviderYandex)
	user := d.users.add("carol@example.com", testHash, domain.UserStatusActive)

	linkToken := "link-1"
	_ = d.store.SaveLink(context.Background(), linkToken, oauthstate.LinkPayload{
		Provider: domain.OAuthProviderYandex, ExternalUserID: "ya-1",
		Email: user.Email, UserID: user.ID, DisplayName: "Carol",
	}, time.Minute)

	login, err := d.svc.ConfirmOAuthLink(context.Background(), httptest.NewRequest("POST", "/link", nil), linkToken, testPwd)
	if err != nil {
		t.Fatalf("ConfirmOAuthLink: %v", err)
	}
	if login.User.ID != user.ID {
		t.Fatalf("должны вернуть того же юзера")
	}
	if _, ierr := d.idents.GetByProviderAndExternalID(context.Background(), domain.OAuthProviderYandex, "ya-1"); ierr != nil {
		t.Fatalf("identity должна быть создана: %v", ierr)
	}
	if d.sessions.countByUser(user.ID) != 1 {
		t.Fatalf("должна быть создана сессия")
	}
}

func TestConfirmOAuthLink_WrongPasswordIncrementsFailCount(t *testing.T) {
	d := newOAuthTestDeps(t, domain.OAuthProviderYandex)
	user := d.users.add("dave@example.com", testHash, domain.UserStatusActive)

	linkToken := "link-2"
	_ = d.store.SaveLink(context.Background(), linkToken, oauthstate.LinkPayload{
		Provider: domain.OAuthProviderYandex, ExternalUserID: "ya-2",
		Email: user.Email, UserID: user.ID,
	}, time.Minute)

	_, err := d.svc.ConfirmOAuthLink(context.Background(), httptest.NewRequest("POST", "/link", nil), linkToken, "wrong-password")
	if !errors.Is(err, authsvc.ErrInvalidCredentials) {
		t.Fatalf("ожидалось ErrInvalidCredentials, got %v", err)
	}
	// identity НЕ должна быть создана.
	if _, ierr := d.idents.GetByProviderAndExternalID(context.Background(), domain.OAuthProviderYandex, "ya-2"); !errors.Is(ierr, repository.ErrNotFound) {
		t.Fatalf("identity не должна быть создана при неверном пароле")
	}
	// Счётчик failed_login_count увеличен.
	got := d.users.get(user.ID)
	if got.FailedLoginCount != 1 {
		t.Fatalf("FailedLoginCount = %d, ожидался 1", got.FailedLoginCount)
	}
}

func TestConfirmOAuthLink_InvalidToken(t *testing.T) {
	d := newOAuthTestDeps(t, domain.OAuthProviderYandex)
	_, err := d.svc.ConfirmOAuthLink(context.Background(), httptest.NewRequest("POST", "/link", nil), "nope", "any")
	if !errors.Is(err, authsvc.ErrInvalidOAuthState) {
		t.Fatalf("ожидалось ErrInvalidOAuthState, got %v", err)
	}
}

func TestOAuthDisabled_WhenNotAttached(t *testing.T) {
	d := newTestDeps()
	_, err := d.svc.BeginOAuth(context.Background(), domain.OAuthProviderYandex)
	if !errors.Is(err, authsvc.ErrOAuthDisabled) {
		t.Fatalf("ожидалось ErrOAuthDisabled, got %v", err)
	}
}
