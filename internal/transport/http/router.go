package transport

import (
	"github.com/Beliashkoff/RepricerX/internal/pkg/auditlog"
	"github.com/Beliashkoff/RepricerX/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// RouterConfig — все зависимости, нужные для регистрации маршрутов.
type RouterConfig struct {
	AuthSvc        *auth.Service
	Audit          *auditlog.Logger
	AllowedOrigins []string
	TrustProxy     bool
	SecureCookie   bool    // true в prod
	FrontendURL    string  // куда редиректить после email-verify
}

// RegisterRoutes регистрирует все HTTP-маршруты приложения на переданном engine.
func RegisterRoutes(r *gin.Engine, cfg RouterConfig) {
	authH := NewAuthHandler(cfg.AuthSvc, cfg.SecureCookie, cfg.FrontendURL)

	requireAuth := RequireAuth(cfg.AuthSvc, cfg.Audit, cfg.TrustProxy, cfg.SecureCookie)
	requireCSRF := RequireSameOrigin(cfg.AllowedOrigins, cfg.Audit, cfg.TrustProxy)

	// Публичные auth-эндпоинты — без RequireAuth и без CSRF.
	public := r.Group("/api/auth")
	{
		public.POST("/register", authH.Register)
		public.POST("/login", authH.Login)
		public.GET("/verify", authH.VerifyEmail)
		public.POST("/verification/resend", authH.ResendVerification)
	}

	// Защищённые эндпоинты — обязательна валидная сессия.
	protected := r.Group("/api", requireAuth)
	{
		// GET не мутирует состояние — CSRF не нужен.
		protected.GET("/auth/me", authH.Me)

		// Mutating-эндпоинты — дополнительно проверяем Origin.
		mutating := protected.Group("", requireCSRF)
		{
			mutating.POST("/auth/logout", authH.Logout)
			mutating.PATCH("/auth/me", authH.UpdateMe)
		}
	}
}
