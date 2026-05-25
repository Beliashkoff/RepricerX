// Package main — точка входа RepricerX API.
//
//	@title			RepricerX API
//	@version		1.0
//	@description	REST API сервиса автоматического управления ценами на маркетплейсах Wildberries и Ozon.
//	@description
//	@description	## Аутентификация
//	@description	После успешного входа (POST /api/auth/login) сервер устанавливает HttpOnly-cookie `rx_session`.
//	@description	Браузер отправляет его автоматически; при работе через curl/Postman передавайте `-b "rx_session=<token>"`.
//	@description
//	@description	## CSRF-защита
//	@description	Все мутирующие эндпоинты (POST/PATCH/DELETE), кроме `/api/auth/register`, `/api/auth/login`,
//	@description	`/api/auth/verification/resend`, `/api/auth/password/forgot` и `/api/auth/password/reset`, проверяют заголовок `Origin`.
//	@description	Он должен совпадать с одним из разрешённых источников (настраивается через `ALLOWED_ORIGINS`).
//	@description	Swagger UI выставляет `Origin` автоматически.
//
//	@contact.name	Akim Zuev
//	@contact.email	akim.zuev.86@gmail.com
//
//	@host		localhost:8080
//	@BasePath	/
//
//	@securityDefinitions.apikey	SessionCookie
//	@in							cookie
//	@name						rx_session
//	@description				Сессионный cookie, выставляемый при логине (POST /api/auth/login).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/Beliashkoff/RepricerX/docs"
	"github.com/Beliashkoff/RepricerX/internal/config"
	"github.com/Beliashkoff/RepricerX/internal/domain"
	"github.com/Beliashkoff/RepricerX/internal/integration"
	"github.com/Beliashkoff/RepricerX/internal/integration/mock"
	"github.com/Beliashkoff/RepricerX/internal/integration/oauth"
	"github.com/Beliashkoff/RepricerX/internal/integration/oauth/vkid"
	"github.com/Beliashkoff/RepricerX/internal/integration/oauth/yandex"
	"github.com/Beliashkoff/RepricerX/internal/integration/ozon"
	"github.com/Beliashkoff/RepricerX/internal/integration/wildberries"
	"github.com/Beliashkoff/RepricerX/internal/pkg/auditlog"
	"github.com/Beliashkoff/RepricerX/internal/pkg/logger"
	"github.com/Beliashkoff/RepricerX/internal/pkg/mailer"
	"github.com/Beliashkoff/RepricerX/internal/pkg/oauthstate"
	"github.com/Beliashkoff/RepricerX/internal/pkg/ratelimit"
	"github.com/Beliashkoff/RepricerX/internal/pkg/redischeck"
	"github.com/Beliashkoff/RepricerX/internal/pkg/redislimit"
	"github.com/Beliashkoff/RepricerX/internal/repository"
	auditsvc "github.com/Beliashkoff/RepricerX/internal/service/audit"
	authsvc "github.com/Beliashkoff/RepricerX/internal/service/auth"
	competitorsvc "github.com/Beliashkoff/RepricerX/internal/service/competitor"
	dispatchersvc "github.com/Beliashkoff/RepricerX/internal/service/dispatcher"
	notifiersvc "github.com/Beliashkoff/RepricerX/internal/service/notifier"
	pricingsvc "github.com/Beliashkoff/RepricerX/internal/service/pricing"
	productsvc "github.com/Beliashkoff/RepricerX/internal/service/product"
	shopsvc "github.com/Beliashkoff/RepricerX/internal/service/shop"
	strategysvc "github.com/Beliashkoff/RepricerX/internal/service/strategy"
	transport "github.com/Beliashkoff/RepricerX/internal/transport/http"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Логгер ещё не готов — пишем в stderr напрямую.
		slog.Error("ошибка загрузки конфига", "error", err)
		os.Exit(1)
	}

	log := logger.New(cfg.Environment)

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Error("не удалось подключиться к Postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Error("Postgres недоступен", "error", err)
		os.Exit(1)
	}
	log.Info("Postgres подключён")

	if err := redischeck.Ping(context.Background(), cfg.RedisAddr); err != nil {
		log.Error("Redis недоступен", "addr", cfg.RedisAddr, "error", err)
		os.Exit(1)
	}
	log.Info("Redis подключён", "addr", cfg.RedisAddr)
	httpLimiter := redislimit.New(cfg.RedisAddr, "rx:rl:")

	usersRepo := repository.NewUsersRepository(pool)
	sessionsRepo := repository.NewSessionsRepository(pool)
	verRepo := repository.NewEmailVerificationsRepository(pool)
	resetRepo := repository.NewPasswordResetTokensRepository(pool)
	oauthIdentitiesRepo := repository.NewOAuthIdentitiesRepository(pool)
	shopsRepo := repository.NewShopsRepository(pool)
	productsRepo := repository.NewProductsRepository(pool)
	importLogRepo := repository.NewImportLogRepository(pool)
	jobsRepo := repository.NewBackgroundJobsRepository(pool)
	intLogRepo := repository.NewIntegrationLogRepository(pool)
	strategiesRepo := repository.NewStrategiesRepository(pool)
	assignmentsRepo := repository.NewStrategyAssignmentsRepository(pool)
	priceChangesRepo := repository.NewPriceChangesRepository(pool)
	competitorsRepo := repository.NewProductCompetitorsRepository(pool)
	notificationsRepo := repository.NewNotificationsRepository(pool)
	notificationPrefsRepo := repository.NewNotificationPreferencesRepository(pool)
	notificationDeliveriesRepo := repository.NewNotificationDeliveriesRepository(pool)
	channelSettingsRepo := repository.NewUserChannelSettingsRepository(pool)
	telegramLinksRepo := repository.NewTelegramLinksRepository(pool)
	webhooksRepo := repository.NewWebhooksRepository(pool)

	audit := auditlog.New(log)

	var m mailer.Mailer
	if cfg.MailerMode == "smtp" {
		m = mailer.NewSmtpMailer(cfg.SMTPHost, fmt.Sprintf("%d", cfg.SMTPPort), cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom)
	} else {
		m = mailer.NewLogMailer(log)
	}

	limiter := ratelimit.New(5.0)

	type marketplaceBuilder = func(shopID string, credsJSON []byte) (integration.Marketplace, error)
	var mkWB, mkOzon marketplaceBuilder
	if cfg.MockMarketplaces {
		log.Warn("⚠ MOCK_MARKETPLACES=true — реальные адаптеры WB/Ozon отключены, используются in-memory заглушки")
		mockStore := mock.NewStore()
		mkWB = mock.NewBuilder(mockStore, "wb")
		mkOzon = mock.NewBuilder(mockStore, "ozon")
	} else {
		mkWB = func(shopID string, b []byte) (integration.Marketplace, error) {
			return wildberries.NewClient(shopID, b, limiter)
		}
		mkOzon = func(shopID string, b []byte) (integration.Marketplace, error) {
			return ozon.NewClient(shopID, b, limiter)
		}
	}

	shopService := shopsvc.New(shopsRepo, intLogRepo, cfg.AppSecretKey, map[string]shopsvc.MarketplaceFactory{
		"wb":   mkWB,
		"ozon": mkOzon,
	})
	strategyService := strategysvc.New(strategiesRepo, assignmentsRepo)
	plansRepo := repository.NewPricePlansRepository(pool)
	pricingMarketplaceFactories := map[string]pricingsvc.MarketplaceFactory{
		"wb":   mkWB,
		"ozon": mkOzon,
	}
	// Этап 6: dispatcher (отправка цен в МП).
	dispatcherFactories := map[string]dispatchersvc.MarketplaceFactory{
		"wb":   mkWB,
		"ozon": mkOzon,
	}
	notifierService := notifiersvc.New(notifiersvc.Deps{
		Pool:          pool,
		Notifications: notificationsRepo,
		Preferences:   notificationPrefsRepo,
		Deliveries:    notificationDeliveriesRepo,
		ChannelSet:    channelSettingsRepo,
		TelegramRepo:  telegramLinksRepo,
		WebhooksRepo:  webhooksRepo,
		Jobs:          jobsRepo,
		Users:         usersRepo,
		Log:           log,
	})
	frontendURL := cfg.VerificationURLBase
	if u, err := url.Parse(cfg.VerificationURLBase); err == nil {
		frontendURL = u.Scheme + "://" + u.Host
	}

	notifierService.Register(notifiersvc.NewInAppChannel())
	notifierService.Register(notifiersvc.NewEmailChannel(m, usersRepo, frontendURL))
	notifierService.Register(notifiersvc.NewWebhookChannel(webhooksRepo))
	if cfg.TelegramBotToken != "" {
		notifierService.Register(notifiersvc.NewTelegramChannel(cfg.TelegramBotToken, telegramLinksRepo, usersRepo, frontendURL))
	}
	ozonLookup := competitorsvc.SelectOzonLookup(cfg.OzonPriceSource, cfg.MPStatsAPIKey)
	if cfg.OzonPriceSource == "html" {
		log.Warn("OZON_PRICE_SOURCE=html: HTML-парсинг ненадёжен на SPA, рекомендуется bff")
	}
	competitorService := competitorsvc.New(competitorsRepo, ozonLookup, competitorsvc.WithNotifier(notifierService))

	dispatcherService := dispatchersvc.New(
		plansRepo, productsRepo, priceChangesRepo, intLogRepo,
		shopsRepo, jobsRepo,
		cfg.AppSecretKey, dispatcherFactories,
		dispatchersvc.WithNotifier(notifierService),
	)

	pricingService := pricingsvc.New(productsRepo, strategiesRepo,
		pricingsvc.WithCompetitors(competitorsRepo),
		pricingsvc.WithPlans(plansRepo),
		pricingsvc.WithJobs(jobsRepo),
		pricingsvc.WithShops(shopsRepo),
		pricingsvc.WithAssignments(assignmentsRepo),
		pricingsvc.WithPriceSync(cfg.AppSecretKey, pricingMarketplaceFactories, 60*time.Minute),
		pricingsvc.WithDispatcher(dispatcherService),
		pricingsvc.WithNotifier(notifierService),
	)
	auditService := auditsvc.New(priceChangesRepo)

	productService := productsvc.New(shopsRepo, productsRepo, importLogRepo, jobsRepo, cfg.AppSecretKey, map[string]productsvc.MarketplaceFactory{
		"wb":   mkWB,
		"ozon": mkOzon,
	}, productsvc.WithImportMaxAttempts(cfg.WorkerMaxAttempts), productsvc.WithNotifier(notifierService))

	svc := authsvc.New(usersRepo, sessionsRepo, verRepo, resetRepo, m, audit, authsvc.Config{
		IdleTTL:          cfg.SessionIdleTTL,
		AbsoluteTTL:      cfg.SessionAbsoluteTTL,
		TrustProxy:       cfg.TrustProxyHeaders,
		VerificationURL:  cfg.VerificationURLBase,
		PasswordResetURL: cfg.PasswordResetURLBase,
		RateLimiter:      httpLimiter,
	})

	// OAuth-провайдеры (VK ID / Яндекс ID). Если client_id пуст — провайдер не
	// регистрируется, и хендлер вернёт 503. БД-таблица oauth_identities
	// существует независимо.
	oauthProviders := map[domain.OAuthProvider]oauth.Provider{}
	if cfg.OAuthVKClientID != "" && cfg.OAuthVKClientSecret != "" {
		oauthProviders[domain.OAuthProviderVK] = vkid.New(
			cfg.OAuthVKClientID, cfg.OAuthVKClientSecret,
			cfg.OAuthCallbackBaseURL+"/api/auth/oauth/vk/callback",
		)
		log.Info("OAuth: VK ID настроен")
	}
	if cfg.OAuthYandexClientID != "" && cfg.OAuthYandexClientSecret != "" {
		oauthProviders[domain.OAuthProviderYandex] = yandex.New(
			cfg.OAuthYandexClientID, cfg.OAuthYandexClientSecret,
			cfg.OAuthCallbackBaseURL+"/api/auth/oauth/yandex/callback",
		)
		log.Info("OAuth: Яндекс ID настроен")
	}
	if len(oauthProviders) > 0 {
		svc.AttachOAuth(oauthProviders, oauthstate.NewRedisStore(cfg.RedisAddr), oauthIdentitiesRepo)
	} else {
		log.Info("OAuth-провайдеры не настроены: пропускаем (OAUTH_VK_CLIENT_ID/OAUTH_YANDEX_CLIENT_ID пусты)")
	}

	if cfg.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	health := &healthHandlers{pool: pool, redisAddr: cfg.RedisAddr}
	r.GET("/healthz", health.healthz)
	r.GET("/ready", health.ready)

	transport.RegisterRoutes(r, transport.RouterConfig{
		AuthSvc:             svc,
		ShopSvc:             shopService,
		ProductSvc:          productService,
		CompetitorSvc:       competitorService,
		StrategySvc:         strategyService,
		PricingSvc:          pricingService,
		DispatcherSvc:       dispatcherService,
		AuditSvc:            auditService,
		NotifierSvc:         notifierService,
		UsersRepo:           usersRepo,
		Audit:               audit,
		AllowedOrigins:      cfg.AllowedOrigins,
		TrustProxy:          cfg.TrustProxyHeaders,
		SecureCookie:        cfg.IsProd(),
		FrontendURL:         frontendURL,
		OAuthFrontendURL:    cfg.OAuthFrontendBaseURL,
		TelegramBotStartURL: cfg.TelegramBotStartURL,
		RateLimiter:         httpLimiter,
		MaxBodyBytes:        cfg.MaxBodyBytes,
	})

	// Этап 7: cleanup переехал в cmd/scheduler (robfig/cron, тик "0 * * * *").
	// API больше не запускает cleanup-горутину.

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("HTTP сервер запущен", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("ListenAndServe", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	log.Info("завершение работы...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("принудительное завершение", "error", err)
	}
	log.Info("сервер остановлен")
}
