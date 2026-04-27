package transport

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const sessionCookieName = "rx_session"

// SetSessionCookie выставляет cookie сессии с нужными флагами безопасности.
// secure=true только в production — dev работает без HTTPS.
func SetSessionCookie(c *gin.Context, plaintext string, idleExpires time.Time, secure bool) {
	maxAge := int(time.Until(idleExpires).Seconds())
	if maxAge <= 0 {
		maxAge = 1
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    plaintext,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie удаляет cookie на клиенте (logout, истечение сессии).
func ClearSessionCookie(c *gin.Context, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   0,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
