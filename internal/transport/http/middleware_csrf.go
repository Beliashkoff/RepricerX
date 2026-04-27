package transport

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/Beliashkoff/RepricerX/internal/pkg/auditlog"
	"github.com/Beliashkoff/RepricerX/internal/pkg/netutil"
	"github.com/gin-gonic/gin"
)

// RequireSameOrigin — CSRF defense-in-depth поверх SameSite=Lax.
// Применяется на всех mutating-эндпоинтах (POST/PUT/PATCH/DELETE) под auth-группой.
// Проверяет Origin (или Referer), что он совпадает с одним из allowedOrigins.
func RequireSameOrigin(allowedOrigins []string, audit *auditlog.Logger, trustProxy bool) gin.HandlerFunc {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[normalizeOrigin(o)] = struct{}{}
	}

	return func(c *gin.Context) {
		method := c.Request.Method
		// GET и HEAD не изменяют состояние — CSRF-проверка не нужна.
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			// Некоторые браузеры шлют Referer вместо Origin.
			if ref := c.Request.Header.Get("Referer"); ref != "" {
				origin = extractOrigin(ref)
			}
		}

		ipPrefix := netutil.IPPrefix(c.Request, trustProxy)

		if origin == "" {
			audit.CSRFBlocked("(empty)", c.Request.URL.Path, ipPrefix)
			errResp(c, http.StatusForbidden, "csrf_blocked", "Запрос отклонён: CSRF-защита")
			c.Abort()
			return
		}

		if _, ok := originSet[normalizeOrigin(origin)]; !ok {
			audit.CSRFBlocked(origin, c.Request.URL.Path, ipPrefix)
			errResp(c, http.StatusForbidden, "csrf_blocked", "Запрос отклонён: CSRF-защита")
			c.Abort()
			return
		}

		c.Next()
	}
}

func normalizeOrigin(raw string) string {
	return strings.TrimRight(strings.ToLower(raw), "/")
}

func extractOrigin(referer string) string {
	u, err := url.Parse(referer)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
