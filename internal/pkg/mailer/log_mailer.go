package mailer

import (
	"context"
	"log/slog"
)

// LogMailer — dev-реализация: письмо пишется в лог, SMTP не нужен.
// Подходит для локальной разработки и CI.
type LogMailer struct {
	log *slog.Logger
}

func NewLogMailer(log *slog.Logger) *LogMailer {
	return &LogMailer{log: log.With(slog.String("component", "mailer"))}
}

func (m *LogMailer) Send(_ context.Context, to, subject, _, textBody string) error {
	m.log.Info("email (dev/log mode)",
		slog.String("to", to),
		slog.String("subject", subject),
		slog.Int("text_len", len(textBody)),
	)
	return nil
}
