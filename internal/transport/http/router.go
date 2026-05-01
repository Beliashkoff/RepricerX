package transport

import (
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/Beliashkoff/RepricerX/internal/pkg/auditlog"
	"github.com/Beliashkoff/RepricerX/internal/service/auth"
	shopsvc "github.com/Beliashkoff/RepricerX/internal/service/shop"
	"github.com/gin-gonic/gin"
)

// RouterConfig — все зависимости, нужные для регистрации маршрутов.
type RouterConfig struct {
	AuthSvc        *auth.Service
	ShopSvc        *shopsvc.Service
	Audit          *auditlog.Logger
	AllowedOrigins []string
	TrustProxy     bool
	SecureCookie   bool   // true в prod
	FrontendURL    string // куда редиректить после email-verify
}

// RegisterRoutes регистрирует все HTTP-маршруты приложения на переданном engine.
func RegisterRoutes(r *gin.Engine, cfg RouterConfig) {
	authH := NewAuthHandler(cfg.AuthSvc, cfg.SecureCookie, cfg.FrontendURL)
	shopH := NewShopHandler(cfg.ShopSvc)

	requireAuth := RequireAuth(cfg.AuthSvc, cfg.Audit, cfg.TrustProxy, cfg.SecureCookie)
	requireCSRF := RequireSameOrigin(cfg.AllowedOrigins, cfg.Audit, cfg.TrustProxy)

	// Swagger UI: /swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

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
		protected.GET("/shops", shopH.List)
		protected.GET("/shops/:id", shopH.Get)

		// Mutating-эндпоинты — дополнительно проверяем Origin.
		mutating := protected.Group("", requireCSRF)
		{
			mutating.POST("/auth/logout", authH.Logout)
			mutating.PATCH("/auth/me", authH.UpdateMe)

			mutating.POST("/shops", shopH.Create)
			mutating.PATCH("/shops/:id", shopH.Update)
			mutating.DELETE("/shops/:id", shopH.Delete)
			mutating.POST("/shops/:id/test", shopH.TestConnection)
		}
	}
}
