package auth

import "time"

// lockoutDuration возвращает длительность блокировки по количеству неудачных попыток.
// Пороги взяты из плана: 5→5 мин, 10→15 мин, 20→1 ч.
// nil означает «блокировки нет», счётчик увеличивается, но lockout_until не выставляется.
func lockoutDuration(failedCount int) *time.Duration {
	var d time.Duration
	switch {
	case failedCount >= 20:
		d = time.Hour
	case failedCount >= 10:
		d = 15 * time.Minute
	case failedCount >= 5:
		d = 5 * time.Minute
	default:
		return nil
	}
	return &d
}

// lockoutUntil вычисляет момент снятия блокировки.
func lockoutUntil(failedCount int, now time.Time) *time.Time {
	d := lockoutDuration(failedCount)
	if d == nil {
		return nil
	}
	t := now.Add(*d)
	return &t
}
