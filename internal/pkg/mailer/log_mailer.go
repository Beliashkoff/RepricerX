package mailer

import (
	"context"

	"go.uber.org/zap"
)

// LogMailer — dev-реализация: письмо пишется в лог, SMTP не нужен.
// Подходит для локальной разработки и CI.
type LogMailer struct {
	log *zap.Logger
}

func NewLogMailer(log *zap.Logger) *LogMailer {
	return &LogMailer{log: log.Named("mailer")}
}

func (m *LogMailer) Send(_ context.Context, to, subject, _, textBody string) error {
	m.log.Info("email (dev/log mode)",
		zap.String("to", to),
		zap.String("subject", subject),
		zap.String("body", textBody),
	)
	return nil
}
