package main

import (
	"errors"
	"log/slog"
	"os"

	"github.com/Beliashkoff/RepricerX/internal/config"
	"github.com/Beliashkoff/RepricerX/internal/pkg/logger"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("migrate: config", "error", err)
		os.Exit(1)
	}

	log := logger.New(cfg.Environment)
	if err = runMigrations(cfg.DatabaseURL, log); err != nil {
		log.Error("migrate: failed", "error", err)
		os.Exit(1)
	}
}

func runMigrations(databaseURL string, log *slog.Logger) error {
	m, err := migrate.New("file://migrations", databaseURL)
	if err != nil {
		return err
	}
	defer m.Close() //nolint:errcheck

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Info("migrate: no changes")
			return nil
		}
		return err
	}

	log.Info("migrate: applied")
	return nil
}
