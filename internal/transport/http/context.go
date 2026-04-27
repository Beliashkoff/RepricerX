package transport

import (
	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/gin-gonic/gin"
)

const (
	ctxUser      = "auth_user"
	ctxSessionID = "auth_session_id"
	ctxPlaintext = "auth_plaintext"
)

// mustUser извлекает аутентифицированного пользователя из контекста.
// Паникует, если вызван вне RequireAuth — это программная ошибка, не runtime.
func mustUser(c *gin.Context) *domain.User {
	v, _ := c.Get(ctxUser)
	return v.(*domain.User)
}
