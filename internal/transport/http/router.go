package transport

import (
	"time"

	"github.com/Beliashkoff/RepricerX/internal/pkg/auditlog"
	"github.com/Beliashkoff/RepricerX/internal/pkg/redislimit"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	auditsvc "github.com/Beliashkoff/RepricerX/internal/service/audit"
	"github.com/Beliashkoff/RepricerX/internal/service/auth"
	competitorsvc "github.com/Beliashkoff/RepricerX/internal/service/competitor"
	"github.com/Beliashkoff/RepricerX/internal/service/dispatcher"
	notifiersvc "github.com/Beliashkoff/RepricerX/internal/service/notifier"
	pricingsvc "github.com/Beliashkoff/RepricerX/internal/service/pricing"
	productsvc "github.com/Beliashkoff/RepricerX/internal/service/product"
	shopsvc "github.com/Beliashkoff/RepricerX/internal/service/shop"
	strategysvc "github.com/Beliashkoff/RepricerX/internal/service/strategy"
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
	CompetitorSvc  *competitorsvc.Service
	StrategySvc    *strategysvc.Service
	PricingSvc     *pricingsvc.Service
	DispatcherSvc  *dispatcher.Service
	AuditSvc       *auditsvc.Service
	NotifierSvc    *notifiersvc.Service
	UsersRepo      repository.UsersRepository
	Audit          *auditlog.Logger
	AllowedOrigins []string
	TrustProxy     bool
	SecureCookie   bool   // true в prod
	FrontendURL    string // куда редиректить после email-verify
	// OAuthFrontendURL — base фронтенда для OAuth-редиректов (success → /dashboard,
	// link required → /link-oauth, ошибка → /login?oauth_error=...).
	// Если пусто — используется FrontendURL.
	OAuthFrontendURL string
	// TelegramBotStartURL — префикс «https://t.me/<bot>?start=» для UI;
	// пустая строка → notification handler вернёт 503 на запросы линковки.
	TelegramBotStartURL string
	RateLimiter         redislimit.Limiter
	MaxBodyBytes        int64
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
	r.Use(bodySizeLimit(cfg.MaxBodyBytes))

	authH := NewAuthHandler(cfg.AuthSvc, cfg.SecureCookie, cfg.FrontendURL)
	oauthFrontendURL := cfg.OAuthFrontendURL
	if oauthFrontendURL == "" {
		oauthFrontendURL = cfg.FrontendURL
	}
	oauthH := NewOAuthHandler(cfg.AuthSvc, cfg.SecureCookie, oauthFrontendURL)
	shopH := NewShopHandler(cfg.ShopSvc)
	productH := NewProductHandler(cfg.ProductSvc)
	competitorH := NewCompetitorHandler(cfg.CompetitorSvc)
	strategyH := NewStrategyHandler(cfg.StrategySvc)
	pricingH := NewPricingHandler(cfg.PricingSvc, cfg.DispatcherSvc)
	auditH := NewAuditHandler(cfg.AuditSvc)
	notificationH := NewNotificationHandler(cfg.NotifierSvc, cfg.TelegramBotStartURL)

	// Swagger UI: /swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Публичные auth-эндпоинты — без RequireAuth и без CSRF.
	public := r.Group("/api/auth")
	{
		public.POST("/register", authH.Register)
		public.POST("/login", rateLimit(cfg.RateLimiter,
			rateLimitSpec{Scope: "auth:login:ip", Limit: limitLoginIP, Window: time.Minute, Key: ipRateKey(cfg.TrustProxy)},
			rateLimitSpec{Scope: "auth:login:email", Limit: limitLoginEmail, Window: time.Minute, Key: jsonFieldRateKey("email")},
		), authH.Login)
		public.GET("/verify", authH.VerifyEmail)
		public.POST("/verification/resend", rateLimit(cfg.RateLimiter,
			rateLimitSpec{Scope: "auth:resend:ip", Limit: limitPasswordIP, Window: time.Minute, Key: ipRateKey(cfg.TrustProxy)},
			rateLimitSpec{Scope: "auth:resend:email", Limit: limitPasswordEmail, Window: time.Hour, Key: jsonFieldRateKey("email")},
		), authH.ResendVerification)
		public.POST("/password/forgot", rateLimit(cfg.RateLimiter,
			rateLimitSpec{Scope: "auth:forgot:ip", Limit: limitPasswordIP, Window: time.Minute, Key: ipRateKey(cfg.TrustProxy)},
			rateLimitSpec{Scope: "auth:forgot:email", Limit: limitPasswordEmail, Window: time.Hour, Key: jsonFieldRateKey("email")},
		), authH.ForgotPassword)
		public.POST("/password/reset", rateLimit(cfg.RateLimiter,
			rateLimitSpec{Scope: "auth:reset:ip", Limit: limitPasswordIP, Window: time.Minute, Key: ipRateKey(cfg.TrustProxy)},
			rateLimitSpec{Scope: "auth:reset:token", Limit: limitResetToken, Window: time.Minute, Key: jsonFieldRateKey("token")},
		), authH.ResetPassword)

		// OAuth: VK ID + Яндекс ID. /start и /callback — GET (state-токен исполняет
		// роль CSRF), /link — POST с собственным одноразовым link_token из Redis.
		public.GET("/oauth/:provider/start", rateLimit(cfg.RateLimiter,
			rateLimitSpec{Scope: "auth:oauth:start:ip", Limit: limitPasswordIP, Window: time.Minute, Key: ipRateKey(cfg.TrustProxy)},
		), oauthH.Start)
		public.GET("/oauth/:provider/callback", oauthH.Callback)
		public.POST("/oauth/link", rateLimit(cfg.RateLimiter,
			rateLimitSpec{Scope: "auth:oauth:link:ip", Limit: limitLoginIP, Window: time.Minute, Key: ipRateKey(cfg.TrustProxy)},
		), oauthH.Link)
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
		protected.GET("/products/:id/competitors", competitorH.List)
		protected.GET("/strategies", strategyH.List)
		protected.GET("/strategies/:id", strategyH.Get)
		protected.GET("/audit/price-changes", auditH.ListChanges)
		protected.GET("/audit/price-changes.csv", auditH.ExportCSV)
		protected.GET("/reports/summary", auditH.Summary)
		protected.GET("/price-plans", pricingH.ListPlans)
		protected.GET("/price-plans/:id", pricingH.GetPlan)
		importPollingLimit := rateLimit(cfg.RateLimiter,
			rateLimitSpec{Scope: "imports:poll:session", Limit: limitImportSession, Window: time.Minute, Key: sessionRateKey},
			rateLimitSpec{Scope: "imports:poll:user", Limit: limitImportUser, Window: time.Minute, Key: userRateKey},
		)
		competitorRefreshLimit := rateLimit(cfg.RateLimiter,
			rateLimitSpec{Scope: "competitors:refresh:session", Limit: limitCompetitorRefreshSession, Window: time.Minute, Key: sessionRateKey},
			rateLimitSpec{Scope: "competitors:refresh:user", Limit: limitCompetitorRefreshUser, Window: time.Hour, Key: userRateKey},
		)
		protected.GET("/imports/:id", importPollingLimit, productH.GetImport)
		protected.GET("/imports/:id/errors", importPollingLimit, productH.GetImportErrors)

		protected.GET("/notifications", notificationH.List)
		protected.GET("/notifications/unread-count", notificationH.UnreadCount)
		protected.GET("/notifications/preferences", notificationH.GetPreferences)
		protected.GET("/notifications/channel-settings", notificationH.ListChannelSettings)
		protected.GET("/notifications/telegram/status", notificationH.TelegramStatus)
		protected.GET("/notifications/webhooks", notificationH.ListWebhooks)
		protected.GET("/notifications/:id", notificationH.Get)

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
			mutating.POST("/products/:id/competitors", competitorH.Create)
			mutating.PATCH("/competitors/:competitorId", competitorH.Update)
			mutating.DELETE("/competitors/:competitorId", competitorH.Delete)
			mutating.POST("/competitors/:competitorId/refresh", competitorRefreshLimit, competitorH.Refresh)
			mutating.DELETE("/imports/:id", productH.CancelImport)

			mutating.POST("/strategies", strategyH.Create)
			mutating.PATCH("/strategies/:id", strategyH.Update)
			mutating.DELETE("/strategies/:id", strategyH.Delete)
			mutating.POST("/strategies/:id/assignments", strategyH.Assign)
			mutating.DELETE("/strategies/:id/assignments", strategyH.Unassign)
			mutating.POST("/pricing/simulate", pricingH.Simulate)
			mutating.POST("/pricing/recalculate", pricingH.Recalculate)
			mutating.POST("/price-plans/:id/dispatch", pricingH.Dispatch)
			mutating.POST("/price-plans/:id/cancel", pricingH.CancelPlan)
			mutating.POST("/shops/:id/run-now", pricingH.RunNow)

			mutating.PATCH("/notifications/:id/read", notificationH.MarkRead)
			mutating.POST("/notifications/read-all", notificationH.MarkAllRead)
			mutating.DELETE("/notifications/:id", notificationH.Delete)
			mutating.PUT("/notifications/preferences", notificationH.UpdatePreferences)
			mutating.PUT("/notifications/channel-settings/:channel", notificationH.UpdateChannelSettings)
			mutating.POST("/notifications/telegram/link-token", notificationH.IssueTelegramToken)
			mutating.DELETE("/notifications/telegram", notificationH.UnlinkTelegram)
			mutating.POST("/notifications/webhooks", notificationH.CreateWebhook)
			mutating.DELETE("/notifications/webhooks/:id", notificationH.DeleteWebhook)
			mutating.POST("/notifications/webhooks/:id/test", notificationH.TestWebhook)
		}
	}
}
