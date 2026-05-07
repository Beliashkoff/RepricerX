package config

import (
	"strings"
	"testing"
	"time"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/repricer?sslmode=disable")
	t.Setenv("APP_SECRET_KEY", "test-secret-key-with-enough-length")
}

func TestLoad_DefaultsToLogMailer(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MailerMode != "log" {
		t.Fatalf("MailerMode = %q, want log", cfg.MailerMode)
	}
}

func TestLoad_ReadsWorkerMaxAttempts(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WORKER_MAX_ATTEMPTS", "9")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.WorkerMaxAttempts != 9 {
		t.Fatalf("WorkerMaxAttempts = %d, want 9", cfg.WorkerMaxAttempts)
	}
}

func TestLoad_ReadsSessionTTLs(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SESSION_IDLE_TTL", "2h")
	t.Setenv("SESSION_ABSOLUTE_TTL", "48h")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SessionIdleTTL != 2*time.Hour {
		t.Fatalf("SessionIdleTTL = %v, want 2h", cfg.SessionIdleTTL)
	}
	if cfg.SessionAbsoluteTTL != 48*time.Hour {
		t.Fatalf("SessionAbsoluteTTL = %v, want 48h", cfg.SessionAbsoluteTTL)
	}
}

func TestLoad_SMTPModeRequiresSettings(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MAILER_MODE", "smtp")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want SMTP validation error")
	}
	if !strings.Contains(err.Error(), "SMTP_HOST") {
		t.Fatalf("error = %q, want SMTP settings error", err.Error())
	}
}

func TestLoad_SMTPModeReadsSettings(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MAILER_MODE", " SMTP ")
	t.Setenv("SMTP_HOST", " smtp.example.com ")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USER", " user@example.com ")
	t.Setenv("SMTP_PASSWORD", "app-password")
	t.Setenv("SMTP_FROM", " RepricerX <user@example.com> ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MailerMode != "smtp" {
		t.Fatalf("MailerMode = %q, want smtp", cfg.MailerMode)
	}
	if cfg.SMTPHost != "smtp.example.com" || cfg.SMTPPort != 587 || cfg.SMTPUser != "user@example.com" {
		t.Fatalf("SMTP settings not normalized: %#v", cfg)
	}
	if cfg.SMTPFrom != "RepricerX <user@example.com>" {
		t.Fatalf("SMTPFrom = %q", cfg.SMTPFrom)
	}
}

func TestLoad_InvalidMailerMode(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MAILER_MODE", "mail")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want mailer mode validation error")
	}
	if !strings.Contains(err.Error(), "MAILER_MODE") {
		t.Fatalf("error = %q, want MAILER_MODE validation error", err.Error())
	}
}
