package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// Сервер
	Port        string
	Environment string // dev | prod

	// База данных
	DatabaseURL string

	// Redis
	RedisAddr string

	// Безопасность
	AppSecretKey string

	// Сессии
	SessionIdleTTL     time.Duration // скользящий TTL (24 ч)
	SessionAbsoluteTTL time.Duration // абсолютный TTL (7 дней)

	// CORS / CSRF
	AllowedOrigins    []string // список допустимых Origin
	TrustProxyHeaders bool     // доверять X-Forwarded-For

	// Почта
	MailerMode           string // smtp | log
	SMTPHost             string
	SMTPPort             int
	SMTPUser             string
	SMTPPassword         string
	SMTPFrom             string
	VerificationURLBase  string // frontend-маршрут верификации
	PasswordResetURLBase string // frontend-маршрут сброса пароля

	// Worker
	WorkerID              string
	WorkerConcurrency     int
	WorkerPollInterval    time.Duration
	WorkerLockTTL         time.Duration
	WorkerJobTimeout      time.Duration
	WorkerMaxAttempts     int
	WorkerShutdownTimeout time.Duration

	// Notifier
	TelegramBotToken    string // токен бота; пусто = TG-канал отключён
	TelegramBotStartURL string // префикс «https://t.me/<bot>?start=» для UI

	// OAuth (VK ID + Яндекс ID).
	// Пустые ClientID/Secret = провайдер недоступен; хендлер вернёт 503.
	OAuthVKClientID         string
	OAuthVKClientSecret     string
	OAuthYandexClientID     string
	OAuthYandexClientSecret string
	OAuthCallbackBaseURL    string // base для построения redirect_uri (например, https://app.example.ru)
	OAuthFrontendBaseURL    string // base для редиректов на фронтенд после callback'а

	// HTTP
	MaxBodyBytes int64 // лимит размера тела запроса в байтах

	// Mock-режим маркетплейсов (только dev). При true адаптеры WB/Ozon
	// заменяются на in-memory заглушки из internal/integration/mock.
	MockMarketplaces bool
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                 getEnv("PORT", "8080"),
		Environment:          getEnv("ENVIRONMENT", "dev"),
		DatabaseURL:          mustEnv("DATABASE_URL"),
		RedisAddr:            getEnv("REDIS_ADDR", "localhost:6379"),
		AppSecretKey:         mustEnv("APP_SECRET_KEY"),
		MailerMode:           strings.ToLower(strings.TrimSpace(getEnv("MAILER_MODE", "log"))),
		SMTPHost:             strings.TrimSpace(getEnv("SMTP_HOST", "")),
		SMTPUser:             strings.TrimSpace(getEnv("SMTP_USER", "")),
		SMTPPassword:         getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:             strings.TrimSpace(getEnv("SMTP_FROM", "")),
		VerificationURLBase:  getEnv("VERIFICATION_URL_BASE", "http://localhost:5173/verify"),
		PasswordResetURLBase: getEnv("PASSWORD_RESET_URL_BASE", "http://localhost:5173/reset-password"),
		WorkerID:             getEnv("WORKER_ID", ""),
	}

	// Разбираем список разрешённых Origin (через запятую)
	originsRaw := getEnv("ALLOWED_ORIGINS", "http://localhost:5173")
	for _, o := range strings.Split(originsRaw, ",") {
		if trimmed := strings.TrimSpace(o); trimmed != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, trimmed)
		}
	}

	// SMTP порт
	portStr := getEnv("SMTP_PORT", "465")
	smtpPort, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("SMTP_PORT должен быть числом: %w", err)
	}
	cfg.SMTPPort = smtpPort

	// TTL сессий
	cfg.SessionIdleTTL, err = time.ParseDuration(getEnv("SESSION_IDLE_TTL", "24h"))
	if err != nil {
		return nil, fmt.Errorf("SESSION_IDLE_TTL: %w", err)
	}
	cfg.SessionAbsoluteTTL, err = time.ParseDuration(getEnv("SESSION_ABSOLUTE_TTL", "168h"))
	if err != nil {
		return nil, fmt.Errorf("SESSION_ABSOLUTE_TTL: %w", err)
	}

	cfg.TrustProxyHeaders, _ = strconv.ParseBool(getEnv("TRUST_PROXY_HEADERS", "false"))

	cfg.WorkerConcurrency, err = parsePositiveIntEnv("WORKER_CONCURRENCY", 1)
	if err != nil {
		return nil, err
	}
	cfg.WorkerMaxAttempts, err = parsePositiveIntEnv("WORKER_MAX_ATTEMPTS", 5)
	if err != nil {
		return nil, err
	}
	cfg.WorkerPollInterval, err = time.ParseDuration(getEnv("WORKER_POLL_INTERVAL", "2s"))
	if err != nil {
		return nil, fmt.Errorf("WORKER_POLL_INTERVAL: %w", err)
	}
	cfg.WorkerLockTTL, err = time.ParseDuration(getEnv("WORKER_LOCK_TTL", "5m"))
	if err != nil {
		return nil, fmt.Errorf("WORKER_LOCK_TTL: %w", err)
	}
	cfg.WorkerJobTimeout, err = time.ParseDuration(getEnv("WORKER_JOB_TIMEOUT", "2m"))
	if err != nil {
		return nil, fmt.Errorf("WORKER_JOB_TIMEOUT: %w", err)
	}
	cfg.WorkerShutdownTimeout, err = time.ParseDuration(getEnv("WORKER_SHUTDOWN_TIMEOUT", "30s"))
	if err != nil {
		return nil, fmt.Errorf("WORKER_SHUTDOWN_TIMEOUT: %w", err)
	}

	cfg.TelegramBotToken = strings.TrimSpace(getEnv("TELEGRAM_BOT_TOKEN", ""))
	cfg.TelegramBotStartURL = strings.TrimSpace(getEnv("TELEGRAM_BOT_START_URL", ""))

	cfg.OAuthVKClientID = strings.TrimSpace(getEnv("OAUTH_VK_CLIENT_ID", ""))
	cfg.OAuthVKClientSecret = getEnv("OAUTH_VK_CLIENT_SECRET", "")
	cfg.OAuthYandexClientID = strings.TrimSpace(getEnv("OAUTH_YANDEX_CLIENT_ID", ""))
	cfg.OAuthYandexClientSecret = getEnv("OAUTH_YANDEX_CLIENT_SECRET", "")
	cfg.OAuthCallbackBaseURL = strings.TrimRight(strings.TrimSpace(getEnv("OAUTH_CALLBACK_BASE_URL", "http://localhost:8080")), "/")
	cfg.OAuthFrontendBaseURL = strings.TrimRight(strings.TrimSpace(getEnv("OAUTH_FRONTEND_BASE_URL", "http://localhost:5173")), "/")

	maxBodyStr := getEnv("MAX_BODY_BYTES", "1048576") // 1 MiB по умолчанию
	maxBody, err := strconv.ParseInt(maxBodyStr, 10, 64)
	if err != nil || maxBody <= 0 {
		return nil, fmt.Errorf("MAX_BODY_BYTES должен быть положительным числом")
	}
	cfg.MaxBodyBytes = maxBody

	cfg.MockMarketplaces, _ = strconv.ParseBool(getEnv("MOCK_MARKETPLACES", "false"))

	switch cfg.MailerMode {
	case "log":
	case "smtp":
		if cfg.SMTPHost == "" || cfg.SMTPUser == "" || cfg.SMTPPassword == "" || cfg.SMTPFrom == "" {
			return nil, fmt.Errorf("SMTP_HOST, SMTP_USER, SMTP_PASSWORD, SMTP_FROM обязательны при MAILER_MODE=smtp")
		}
	default:
		return nil, fmt.Errorf("MAILER_MODE должен быть log или smtp")
	}

	if cfg.Environment == "prod" {
		if cfg.MockMarketplaces {
			return nil, fmt.Errorf("MOCK_MARKETPLACES=true запрещён при ENVIRONMENT=prod")
		}
		if cfg.MailerMode != "smtp" {
			return nil, fmt.Errorf("MAILER_MODE=smtp обязателен в prod")
		}
		if err := validateProdFrontendURL("VERIFICATION_URL_BASE", cfg.VerificationURLBase); err != nil {
			return nil, err
		}
		if err := validateProdFrontendURL("PASSWORD_RESET_URL_BASE", cfg.PasswordResetURLBase); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func (c *Config) IsProd() bool { return c.Environment == "prod" }

func validateProdFrontendURL(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return fmt.Errorf("%s должен быть HTTPS URL в prod", name)
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return fmt.Errorf("%s не должен указывать на localhost в prod", name)
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parsePositiveIntEnv(key string, fallback int) (int, error) {
	value, err := strconv.Atoi(getEnv(key, strconv.Itoa(fallback)))
	if err != nil || value < 1 {
		return 0, fmt.Errorf("%s должен быть положительным числом", key)
	}
	return value, nil
}

// mustEnv возвращает значение переменной или паникует при старте — лучше узнать сразу.
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("обязательная переменная окружения %q не задана", key))
	}
	return v
}
