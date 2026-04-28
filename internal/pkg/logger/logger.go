// Package logger создаёт slog.Logger с настройками для каждого окружения.
//
// dev:  TextHandler, DEBUG+, с указанием файла/строки — удобно в консоли.
// prod: JSONHandler, WARN+ — только критические события; INFO/DEBUG отбрасываются
//
//	до записи, без аллокаций.
package logger

import (
	"log/slog"
	"os"
)

// New возвращает настроенный *slog.Logger.
// env="prod" → JSON, только WARN и ERROR.
// Любое другое значение → текст, DEBUG и выше (для локальной разработки).
func New(env string) *slog.Logger {
	if env == "prod" {
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelWarn,
		}))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	}))
}
