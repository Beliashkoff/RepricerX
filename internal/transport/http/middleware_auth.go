package transport

import (
	"net/http"

	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/pkg/auditlog"
	"github.com/Beliashkoff/RepricerX/internal/pkg/netutil"
	"github.com/Beliashkoff/RepricerX/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// RequireAuth проверяет сессию и кладёт user/session в context.
//  1. Cookie есть → hash → GetByTokenHash (фильтр по TTL в SQL).
//  2. Пользователь active → fingerprint-check (мягкий лог).
//  3. Условное продление idle TTL → Set-Cookie только при фактическом обновлении.
func RequireAuth(svc *auth.Service, audit *auditlog.Logger, trustProxy, secure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		plaintext, err := c.Cookie(sessionCookieName)
		if err != nil || plaintext == "" {
			ClearSessionCookie(c, secure)
			errResp(c, http.StatusUnauthorized, "unauthorized", "Требуется авторизация")
			c.Abort()
			return
		}

		sess, err := svc.GetSessionByToken(c.Request.Context(), plaintext)
		if err != nil {
			ClearSessionCookie(c, secure)
			errResp(c, http.StatusUnauthorized, "unauthorized", "Сессия не найдена или истекла")
			c.Abort()
			return
		}

		user, err := svc.GetUser(c.Request.Context(), sess.UserID)
		if err != nil || user.Status != domain.UserStatusActive {
			// Пользователь заблокирован: удаляем все его сессии.
			svc.RevokeAllSessions(c.Request.Context(), sess.UserID)
			ClearSessionCookie(c, secure)
			errResp(c, http.StatusForbidden, "user_blocked", "Аккаунт заблокирован")
			c.Abort()
			return
		}

		// Fingerprint-check: мягкое предупреждение, сессию не инвалидируем.
		curUA := truncateUA(c.Request.UserAgent())
		curIP := netutil.IPPrefix(c.Request, trustProxy)
		if curUA != sess.UserAgent {
			audit.SessionFingerprintMismatch(user.ID, sess.ID, "user_agent", sess.UserAgent, curUA)
		}
		if curIP != sess.IPPrefix {
			audit.SessionFingerprintMismatch(user.ID, sess.ID, "ip_prefix", sess.IPPrefix, curIP)
		}

		// Условное продление idle TTL.
		newIdle, _ := svc.TouchSession(c.Request.Context(), sess)
		if newIdle != nil {
			// TTL фактически продлён — обновляем cookie.
			SetSessionCookie(c, plaintext, *newIdle, secure)
		}

		c.Set(ctxUser, user)
		c.Set(ctxSessionID, sess.ID)
		c.Set(ctxPlaintext, plaintext)
		c.Next()
	}
}

// truncateUA переопределена здесь, чтобы не импортировать service/auth из transport.
func truncateUA(ua string) string {
	runes := []rune(ua)
	if len(runes) > 255 {
		return string(runes[:255])
	}
	return ua
}
