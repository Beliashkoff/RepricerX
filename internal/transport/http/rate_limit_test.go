package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/pkg/redislimit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fakeRateLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func (l *fakeRateLimiter) Allow(_ context.Context, key string, limit int, window time.Duration) (redislimit.Result, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts == nil {
		l.counts = make(map[string]int)
	}
	l.counts[key]++
	count := l.counts[key]
	return redislimit.Result{
		Allowed:    count <= limit,
		Count:      count,
		Limit:      limit,
		RetryAfter: window,
	}, nil
}

func TestRateLimitByEmailReturns429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := &fakeRateLimiter{}
	r := gin.New()
	r.POST("/login", rateLimit(limiter,
		rateLimitSpec{Scope: "auth:login:email", Limit: 1, Window: time.Minute, Key: jsonFieldRateKey("email")},
	), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	body := `{"email":"User@Example.com","password":"ValidPass123!"}`
	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body)))
	if first.Code != http.StatusOK {
		t.Fatalf("first request: want 200, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	r.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body)))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: want 429, got %d: %s", second.Code, second.Body.String())
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("want Retry-After header")
	}
}

func TestBodySizeLimitReturns413(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(bodySizeLimit(8))
	r.POST("/login", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":"too-large@example.com"}`)))
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestImportPollingRateLimitBySessionAndUserReturns429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := &fakeRateLimiter{}
	userID := uuid.New()
	sessionID := uuid.New()

	r := gin.New()
	r.GET("/imports/:id", func(c *gin.Context) {
		c.Set(ctxUser, &domain.User{ID: userID})
		c.Set(ctxSessionID, sessionID)
		c.Next()
	}, rateLimit(limiter,
		rateLimitSpec{Scope: "imports:poll:session", Limit: 1, Window: time.Minute, Key: sessionRateKey},
		rateLimitSpec{Scope: "imports:poll:user", Limit: 10, Window: time.Minute, Key: userRateKey},
	), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/imports/"+uuid.NewString(), nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first request: want 200, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	r.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/imports/"+uuid.NewString(), nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: want 429, got %d: %s", second.Code, second.Body.String())
	}
}
