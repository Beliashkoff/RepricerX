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
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                 getEnv("PORT", "8080"),
		Environment:          getEnv("ENVIRONMENT", "dev"),
		DatabaseURL:          mustEnv("DATABASE_URL"),
		RedisAddr:            getEnv("REDIS_ADDR", "localhost:6379"),
		AppSecretKey:         mustEnv("APP_SECRET_KEY"),
		MailerMode:           getEnv("MAILER_MODE", "log"),
		SMTPHost:             getEnv("SMTP_HOST", ""),
		SMTPUser:             getEnv("SMTP_USER", ""),
		SMTPPassword:         getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:             getEnv("SMTP_FROM", ""),
		VerificationURLBase:  getEnv("VERIFICATION_URL_BASE", "http://localhost:5173/verify"),
		PasswordResetURLBase: getEnv("PASSWORD_RESET_URL_BASE", "http://localhost:5173/reset-password"),
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

	if cfg.Environment == "prod" {
		if cfg.MailerMode != "smtp" {
			return nil, fmt.Errorf("MAILER_MODE=smtp обязателен в prod")
		}
		if cfg.SMTPHost == "" || cfg.SMTPUser == "" || cfg.SMTPPassword == "" || cfg.SMTPFrom == "" {
			return nil, fmt.Errorf("SMTP_HOST, SMTP_USER, SMTP_PASSWORD, SMTP_FROM обязательны в prod при MAILER_MODE=smtp")
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

// mustEnv возвращает значение переменной или паникует при старте — лучше узнать сразу.
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("обязательная переменная окружения %q не задана", key))
	}
	return v
}
