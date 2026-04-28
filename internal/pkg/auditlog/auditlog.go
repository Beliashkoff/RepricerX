// Package auditlog пишет структурированные security-события.
//
// Уровни намеренны:
//   - DEBUG  — очень частые служебные события (SessionRefreshed): только dev.
//   - INFO   — нормальный поток (SessionCreated, EmailVerificationSent): только dev.
//   - WARN   — security-события (AuthFailed, CSRF, fingerprint mismatch): видны в prod.
//   - ERROR  — сбои, требующие внимания (EmailSendFailed): видны в prod.
package auditlog

import (
	"log/slog"
	"strconv"

	"github.com/google/uuid"
)

// Logger — тонкая обёртка над slog, фиксирует контракт полей для audit-событий.
type Logger struct {
	log *slog.Logger
}

func New(log *slog.Logger) *Logger {
	return &Logger{log: log.With(slog.String("component", "audit"))}
}

// SessionCreated — новая сессия после успешного логина.
// INFO: нормальный поток, не нужен в prod.
func (l *Logger) SessionCreated(userID, sessionID uuid.UUID, ipPrefix, userAgent string) {
	l.log.Info("session_created",
		slog.String("user_id", userID.String()),
		slog.String("session_id", sessionID.String()),
		slog.String("ip_prefix", ipPrefix),
		slog.String("user_agent", truncate(userAgent, 100)),
	)
}

// SessionRefreshed — idle TTL продлён (не на каждый запрос, только при фактическом обновлении).
// DEBUG: очень частое событие — только для отладки.
func (l *Logger) SessionRefreshed(userID, sessionID uuid.UUID) {
	l.log.Debug("session_refreshed",
		slog.String("user_id", userID.String()),
		slog.String("session_id", sessionID.String()),
	)
}

// SessionRevoked — сессия уничтожена (logout, blocked, password change).
// INFO: нормальный поток.
func (l *Logger) SessionRevoked(userID, sessionID uuid.UUID, reason string) {
	l.log.Info("session_revoked",
		slog.String("user_id", userID.String()),
		slog.String("session_id", sessionID.String()),
		slog.String("reason", reason),
	)
}

// AuthFailed — неудачная попытка входа.
// WARN: security-событие, видно в prod.
// email не логируем целиком (GDPR) — только первые 3 символа + длина.
func (l *Logger) AuthFailed(email, reason, ipPrefix string) {
	l.log.Warn("auth_failed",
		slog.String("email_hint", emailHint(email)),
		slog.String("reason", reason),
		slog.String("ip_prefix", ipPrefix),
	)
}

// BlockedLoginAttempt — попытка входа от заблокированного пользователя.
// WARN: security-событие, видно в prod.
func (l *Logger) BlockedLoginAttempt(userID uuid.UUID, ipPrefix string) {
	l.log.Warn("blocked_login_attempt",
		slog.String("user_id", userID.String()),
		slog.String("ip_prefix", ipPrefix),
	)
}

// SessionFingerprintMismatch — ip или user_agent сессии изменились (мягкое предупреждение).
// WARN: потенциальный признак угона сессии, видно в prod.
func (l *Logger) SessionFingerprintMismatch(userID, sessionID uuid.UUID, field, stored, current string) {
	l.log.Warn("session_fingerprint_mismatch",
		slog.String("user_id", userID.String()),
		slog.String("session_id", sessionID.String()),
		slog.String("field", field),
		slog.String("stored", stored),
		slog.String("current", current),
	)
}

// CSRFBlocked — Origin/Referer не совпал с разрешённым.
// WARN: security-событие, видно в prod.
func (l *Logger) CSRFBlocked(origin, path, ipPrefix string) {
	l.log.Warn("csrf_blocked",
		slog.String("origin", origin),
		slog.String("path", path),
		slog.String("ip_prefix", ipPrefix),
	)
}

// EmailVerificationSent — письмо подтверждения отправлено.
// INFO: нормальный поток, не нужен в prod.
func (l *Logger) EmailVerificationSent(userID uuid.UUID) {
	l.log.Info("email_verification_sent",
		slog.String("user_id", userID.String()),
	)
}

// EmailSendFailed — SMTP-ошибка при отправке; пользователь уже создан.
// ERROR: требует внимания, видно в prod.
func (l *Logger) EmailSendFailed(userID uuid.UUID, err error) {
	l.log.Error("email_send_failed",
		slog.String("user_id", userID.String()),
		slog.Any("error", err),
	)
}

// EmailVerificationUsed — верификационный токен успешно использован.
// INFO: нормальный поток, не нужен в prod.
func (l *Logger) EmailVerificationUsed(userID uuid.UUID) {
	l.log.Info("email_verification_used",
		slog.String("user_id", userID.String()),
	)
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return s
}

// emailHint возвращает первые 3 символа email + общую длину.
// Достаточно для дебага, не раскрывает адрес полностью.
func emailHint(email string) string {
	if len(email) <= 3 {
		return "***"
	}
	return email[:3] + "...(" + strconv.Itoa(len(email)) + ")"
}
