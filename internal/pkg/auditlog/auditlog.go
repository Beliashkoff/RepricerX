// Package auditlog пишет структурированные события безопасности.
// Все события — только в логи, наружу (в HTTP-ответы) не попадают.
package auditlog

import (
	"strconv"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Logger — тонкая обёртка над zap, фиксирует контракт полей для audit-событий.
type Logger struct {
	log *zap.Logger
}

func New(log *zap.Logger) *Logger {
	return &Logger{log: log.Named("audit")}
}

// SessionCreated — новая сессия после успешного логина.
func (l *Logger) SessionCreated(userID, sessionID uuid.UUID, ipPrefix, userAgent string) {
	l.log.Info("session_created",
		zap.String("user_id", userID.String()),
		zap.String("session_id", sessionID.String()),
		zap.String("ip_prefix", ipPrefix),
		zap.String("user_agent", truncate(userAgent, 100)),
	)
}

// SessionRefreshed — idle TTL продлён (не на каждый запрос, только при фактическом обновлении).
func (l *Logger) SessionRefreshed(userID, sessionID uuid.UUID) {
	l.log.Info("session_refreshed",
		zap.String("user_id", userID.String()),
		zap.String("session_id", sessionID.String()),
	)
}

// SessionRevoked — сессия уничтожена (logout, blocked, password change).
func (l *Logger) SessionRevoked(userID, sessionID uuid.UUID, reason string) {
	l.log.Info("session_revoked",
		zap.String("user_id", userID.String()),
		zap.String("session_id", sessionID.String()),
		zap.String("reason", reason),
	)
}

// AuthFailed — неудачная попытка входа. reason: no_user, bad_password, locked_out, not_verified.
// email не логируем целиком из соображений GDPR — только первые 3 символа + длина.
func (l *Logger) AuthFailed(email, reason, ipPrefix string) {
	l.log.Warn("auth_failed",
		zap.String("email_hint", emailHint(email)),
		zap.String("reason", reason),
		zap.String("ip_prefix", ipPrefix),
	)
}

// BlockedLoginAttempt — попытка входа от заблокированного пользователя.
func (l *Logger) BlockedLoginAttempt(userID uuid.UUID, ipPrefix string) {
	l.log.Warn("blocked_login_attempt",
		zap.String("user_id", userID.String()),
		zap.String("ip_prefix", ipPrefix),
	)
}

// SessionFingerprintMismatch — ip или user_agent сессии изменились (мягкое предупреждение).
func (l *Logger) SessionFingerprintMismatch(userID, sessionID uuid.UUID, field, stored, current string) {
	l.log.Warn("session_fingerprint_mismatch",
		zap.String("user_id", userID.String()),
		zap.String("session_id", sessionID.String()),
		zap.String("field", field),
		zap.String("stored", stored),
		zap.String("current", current),
	)
}

// CSRFBlocked — Origin/Referer не совпал с разрешённым.
func (l *Logger) CSRFBlocked(origin, path, ipPrefix string) {
	l.log.Warn("csrf_blocked",
		zap.String("origin", origin),
		zap.String("path", path),
		zap.String("ip_prefix", ipPrefix),
	)
}

// EmailVerificationSent — письмо подтверждения отправлено.
func (l *Logger) EmailVerificationSent(userID uuid.UUID) {
	l.log.Info("email_verification_sent", zap.String("user_id", userID.String()))
}

// EmailSendFailed — SMTP-ошибка при отправке; пользователь уже создан.
func (l *Logger) EmailSendFailed(userID uuid.UUID, err error) {
	l.log.Error("email_send_failed",
		zap.String("user_id", userID.String()),
		zap.Error(err),
	)
}

// EmailVerificationUsed — верификационный токен успешно использован.
func (l *Logger) EmailVerificationUsed(userID uuid.UUID) {
	l.log.Info("email_verification_used", zap.String("user_id", userID.String()))
}

// truncate обрезает строку до maxLen символов (не байт).
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return s
}

// emailHint возвращает первые 3 символа email + общую длину — достаточно для дебага,
// не раскрывает адрес полностью.
func emailHint(email string) string {
	if len(email) <= 3 {
		return "***"
	}
	return email[:3] + "..." + "(" + itoa(len(email)) + ")"
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
