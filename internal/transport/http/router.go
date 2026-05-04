package transport

import (
	"time"

	"github.com/Beliashkoff/RepricerX/internal/pkg/auditlog"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	"github.com/Beliashkoff/RepricerX/internal/service/auth"
	productsvc "github.com/Beliashkoff/RepricerX/internal/service/product"
	shopsvc "github.com/Beliashkoff/RepricerX/internal/service/shop"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// RouterConfig — все зависимости, нужные для регистрации маршрутов.
type RouterConfig struct {
	AuthSvc        *auth.Service
	ShopSvc        *shopsvc.Service
	ProductSvc     *productsvc.Service
	UsersRepo      repository.UsersRepository
	Audit          *auditlog.Logger
	AllowedOrigins []string
	TrustProxy     bool
	SecureCookie   bool   // true в prod
	FrontendURL    string // куда редиректить после email-verify
}

// RegisterRoutes регистрирует все HTTP-маршруты приложения на переданном engine.
func RegisterRoutes(r *gin.Engine, cfg RouterConfig) {
	// CORS для фронтенда (Vite dev server + prod).
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	authH := NewAuthHandler(cfg.AuthSvc, cfg.SecureCookie, cfg.FrontendURL)
	shopH := NewShopHandler(cfg.ShopSvc)
	productH := NewProductHandler(cfg.ProductSvc)

	// Swagger UI: /swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Публичные auth-эндпоинты — без RequireAuth и без CSRF.
	public := r.Group("/api/auth")
	{
		public.POST("/register", authH.Register)
		public.POST("/login", authH.Login)
		public.GET("/verify", authH.VerifyEmail)
		public.POST("/verification/resend", authH.ResendVerification)
		public.POST("/password/forgot", authH.ForgotPassword)
		public.POST("/password/reset", authH.ResetPassword)
	}

	requireAuth := RequireAuth(cfg.AuthSvc, cfg.Audit, cfg.TrustProxy, cfg.SecureCookie)
	requireCSRF := RequireSameOrigin(cfg.AllowedOrigins, cfg.Audit, cfg.TrustProxy)

	protected := r.Group("/api", requireAuth)
	{
		protected.GET("/auth/me", authH.Me)
		protected.GET("/shops", shopH.List)
		protected.GET("/shops/:id", shopH.Get)
		protected.GET("/products", productH.List)
		protected.GET("/products/export", productH.Export)
		protected.GET("/imports/:id", productH.GetImport)
		protected.GET("/imports/:id/errors", productH.GetImportErrors)

		mutating := protected.Group("", requireCSRF)
		{
			mutating.POST("/auth/logout", authH.Logout)
			mutating.PATCH("/auth/me", authH.UpdateMe)

			mutating.POST("/shops", shopH.Create)
			mutating.PATCH("/shops/:id", shopH.Update)
			mutating.DELETE("/shops/:id", shopH.Delete)
			mutating.POST("/shops/:id/test", shopH.TestConnection)
			mutating.POST("/shops/:id/products/import", productH.StartImport)
			mutating.POST("/shops/:id/products", productH.Create)
			mutating.PATCH("/products/:id", productH.Patch)
			mutating.DELETE("/products/:id", productH.Delete)
			mutating.POST("/products/bulk-patch", productH.BulkPatch)
			mutating.DELETE("/imports/:id", productH.CancelImport)
		}
	}
}
