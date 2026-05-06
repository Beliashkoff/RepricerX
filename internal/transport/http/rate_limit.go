package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Beliashkoff/RepricerX/internal/pkg/netutil"
	"github.com/Beliashkoff/RepricerX/internal/pkg/redislimit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	defaultMaxBodyBytes = 1 << 20

	limitLoginIP       = 20
	limitLoginEmail    = 10
	limitPasswordIP    = 10
	limitPasswordEmail = 3
	limitResetToken    = 10
	limitImportSession = 60
	limitImportUser    = 120
)

type rateLimitKeyFunc func(*gin.Context) (string, bool, error)

type rateLimitSpec struct {
	Scope  string
	Limit  int
	Window time.Duration
	Key    rateLimitKeyFunc
}

func bodySizeLimit(maxBytes int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBodyBytes
	}
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			if c.Request.ContentLength > maxBytes {
				errResp(c, http.StatusRequestEntityTooLarge, "request_too_large", "Размер запроса превышает лимит")
				c.Abort()
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

func rateLimit(limiter redislimit.Limiter, specs ...rateLimitSpec) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limiter == nil {
			c.Next()
			return
		}
		for _, spec := range specs {
			if spec.Key == nil {
				continue
			}
			value, ok, err := spec.Key(c)
			if err != nil {
				if errors.Is(err, http.ErrBodyReadAfterClose) {
					errResp(c, http.StatusBadRequest, "bad_request", "Неверный формат запроса")
				} else {
					errResp(c, http.StatusRequestEntityTooLarge, "request_too_large", "Размер запроса превышает лимит")
				}
				c.Abort()
				return
			}
			if !ok || value == "" {
				continue
			}
			result, err := limiter.Allow(c.Request.Context(), redislimit.Key(spec.Scope, value), spec.Limit, spec.Window)
			if err != nil {
				errResp(c, http.StatusTooManyRequests, "rate_limited", "Слишком много запросов")
				c.Abort()
				return
			}
			if !result.Allowed {
				setRetryAfter(c, result.RetryAfter)
				errResp(c, http.StatusTooManyRequests, "rate_limited", "Слишком много запросов")
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

func ipRateKey(trustProxy bool) rateLimitKeyFunc {
	return func(c *gin.Context) (string, bool, error) {
		ipPrefix := netutil.IPPrefix(c.Request, trustProxy)
		return ipPrefix, ipPrefix != "", nil
	}
}

func jsonFieldRateKey(field string) rateLimitKeyFunc {
	return func(c *gin.Context) (string, bool, error) {
		raw, err := cachedBody(c)
		if err != nil {
			return "", false, err
		}
		var payload map[string]any
		if err = json.Unmarshal(raw, &payload); err != nil {
			return "", false, nil
		}
		value, ok := payload[field].(string)
		value = strings.ToLower(strings.TrimSpace(value))
		return value, ok && value != "", nil
	}
}

func sessionRateKey(c *gin.Context) (string, bool, error) {
	v, ok := c.Get(ctxSessionID)
	if !ok {
		return "", false, nil
	}
	id, ok := v.(uuid.UUID)
	if !ok {
		return fmt.Sprint(v), true, nil
	}
	return id.String(), true, nil
}

func userRateKey(c *gin.Context) (string, bool, error) {
	user := mustUser(c)
	return user.ID.String(), true, nil
}

func cachedBody(c *gin.Context) ([]byte, error) {
	if v, ok := c.Get("cached_body"); ok {
		if raw, ok := v.([]byte); ok {
			return raw, nil
		}
	}
	if c.Request.Body == nil {
		return nil, nil
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	c.Set("cached_body", raw)
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	return raw, nil
}

func setRetryAfter(c *gin.Context, retryAfter time.Duration) {
	seconds := int(retryAfter.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	c.Header("Retry-After", strconv.Itoa(seconds))
}
