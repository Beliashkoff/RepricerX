//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/pkg/auditlog"
	"github.com/Beliashkoff/RepricerX/internal/pkg/mailer"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	authsvc "github.com/Beliashkoff/RepricerX/internal/service/auth"
	transport "github.com/Beliashkoff/RepricerX/internal/transport/http"
	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	testOrigin      = "http://localhost:5173"
	testFrontendURL = "http://localhost:5173"

	testEmail    = "test@example.com"
	testPassword = "ValidPass123!"
	testName     = "Test User"
)

var (
	testPool   *pgxpool.Pool
	testSrv    *httptest.Server
	testMailer *capturingMailer
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Println("integration: DATABASE_URL not set, skipping")
		os.Exit(0)
	}

	pool, err := setupDB(dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: setup db: %v\n", err)
		os.Exit(1)
	}
	testPool = pool

	testMailer = &capturingMailer{}
	testSrv = buildServer(pool, testMailer)

	code := m.Run()

	testSrv.Close()
	pool.Close()
	os.Exit(code)
}

func setupDB(dsn string) (*pgxpool.Pool, error) {
	m, err := migrate.New("file://../../migrations", dsn)
	if err != nil {
		return nil, fmt.Errorf("migrate.New: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return nil, fmt.Errorf("migrate up: %w", err)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	return pool, nil
}

func buildServer(pool *pgxpool.Pool, m mailer.Mailer) *httptest.Server {
	gin.SetMode(gin.TestMode)

	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	audit := auditlog.New(log)

	svc := authsvc.New(
		repository.NewUsersRepository(pool),
		repository.NewSessionsRepository(pool),
		repository.NewEmailVerificationsRepository(pool),
		m,
		audit,
		authsvc.Config{
			IdleTTL:         24 * time.Hour,
			AbsoluteTTL:     7 * 24 * time.Hour,
			VerificationURL: testFrontendURL + "/verify",
		},
	)

	r := gin.New()
	r.Use(gin.Recovery())
	transport.RegisterRoutes(r, transport.RouterConfig{
		AuthSvc:        svc,
		Audit:          audit,
		AllowedOrigins: []string{testOrigin},
		TrustProxy:     false,
		SecureCookie:   false,
		FrontendURL:    testFrontendURL,
	})
	return httptest.NewServer(r)
}

// --- DB helpers ---

func truncate(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		TRUNCATE TABLE email_verifications, sessions, users
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	testMailer.reset()
}

func activateUser(t *testing.T, email string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`UPDATE users SET status = 'active' WHERE email = $1`, email)
	if err != nil {
		t.Fatalf("activate user %q: %v", email, err)
	}
}

func getUserID(t *testing.T, email string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE email = $1`, email).Scan(&id)
	if err != nil {
		t.Fatalf("get user id for %q: %v", email, err)
	}
	return id
}

// --- HTTP helpers ---

func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func doJSON(t *testing.T, client *http.Client, method, path string, body any, headers ...map[string]string) *http.Response {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, testSrv.URL+path, reqBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, h := range headers {
		for k, v := range h {
			req.Header.Set(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func mustStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want HTTP %d, got %d: %s", want, resp.StatusCode, body)
	}
}

func mustDecode(t *testing.T, resp *http.Response, target any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}

func mustErrorCode(t *testing.T, resp *http.Response, code string) {
	t.Helper()
	var env struct {
		Error struct{ Code string } `json:"error"`
	}
	mustDecode(t, resp, &env)
	if env.Error.Code != code {
		t.Fatalf("want error code %q, got %q", code, env.Error.Code)
	}
}

// withOrigin returns headers with the test Origin.
func withOrigin() map[string]string {
	return map[string]string{"Origin": testOrigin}
}

// --- Email helpers ---

// capturingMailer records outgoing emails for inspection in tests.
type capturingMailer struct {
	mu   sync.Mutex
	sent []capturedEmail
}

type capturedEmail struct {
	to, subject, html, text string
}

func (m *capturingMailer) Send(_ context.Context, to, subject, html, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, capturedEmail{to, subject, html, text})
	return nil
}

func (m *capturingMailer) last() *capturedEmail {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return nil
	}
	e := m.sent[len(m.sent)-1]
	return &e
}

func (m *capturingMailer) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = m.sent[:0]
}

// extractVerificationToken parses the plaintext token from a captured email's text body.
// The text template writes the full verification URL on its own line.
func extractVerificationToken(t *testing.T, email *capturedEmail) string {
	t.Helper()
	for _, line := range strings.Split(email.text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "http") && strings.Contains(line, "token=") {
			u, err := url.Parse(line)
			if err != nil {
				t.Fatalf("parse verification url %q: %v", line, err)
			}
			tok := u.Query().Get("token")
			if tok == "" {
				t.Fatal("token param not found in verification url")
			}
			return tok
		}
	}
	t.Fatal("verification url not found in email text body")
	return ""
}
