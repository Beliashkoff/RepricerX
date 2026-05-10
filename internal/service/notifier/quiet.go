package notifier

import (
	"time"

	"github.com/Beliashkoff/RepricerX/internal/domain"
)

// IsInQuietHours возвращает true, если момент `at` (по UTC) попадает в
// окно тишины пользователя [start..end). Поддерживает «обёртки через
// полночь»: если start=22, end=8 — окно с 22:00 до 08:00 по UTC.
//
// Если start или end == nil — окно не задано, всегда возвращает false.
// Часы вне 0..23 — окно считается невалидным, false.
func IsInQuietHours(at time.Time, settings *domain.UserChannelSettings) bool {
	if settings == nil || settings.QuietHoursStart == nil || settings.QuietHoursEnd == nil {
		return false
	}
	start := *settings.QuietHoursStart
	end := *settings.QuietHoursEnd
	if start == end {
		return false
	}
	if start < 0 || start > 23 || end < 0 || end > 23 {
		return false
	}

	hour := at.UTC().Hour()
	if start < end {
		// Простой случай: 09:00..18:00.
		return hour >= start && hour < end
	}
	// Через полночь: 22:00..08:00.
	return hour >= start || hour < end
}
