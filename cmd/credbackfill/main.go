package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	secret := os.Getenv("APP_SECRET_KEY")
	if secret == "" {
		slog.Error("APP_SECRET_KEY не задан")
		os.Exit(1)
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("DATABASE_URL не задан")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("подключение к БД", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	slog.Info("запуск backfill credentials")

	result, err := RunBackfill(ctx, pool, secret)
	if err != nil {
		slog.Error("backfill завершился с ошибкой", "error", err)
		os.Exit(1)
	}

	slog.Info("backfill завершён",
		"total", result.Total,
		"skipped", result.Skipped,
		"migrated", result.Migrated,
		"failed", result.Failed,
	)

	if result.Failed > 0 {
		slog.Error("некоторые магазины не были зашифрованы, проверьте логи выше")
		os.Exit(1)
	}
}
