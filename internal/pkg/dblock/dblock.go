// Package dblock — обёртка над PostgreSQL pg_advisory_lock для координации
// между repликами scheduler/worker. Используется в Этапе 7 для защиты
// глобальных cron-тиков (cleanup, competitor refresh) от двойного запуска.
//
// Принцип работы:
//   - pg_try_advisory_lock(key) — пытается взять exclusive lock на текущей
//     сессии. Возвращает true/false. Не блокирует.
//   - lock держится до pg_advisory_unlock или закрытия соединения.
//   - lock-id — int64; для предсказуемости используем константы.
//
// Важно: lock привязан к session, поэтому нужно держать одно и то же
// соединение на всё время от acquire до release. Используем pgxpool.Conn,
// удерживая его через release-замыкание.
package dblock

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Известные lock-id для глобальных cron-задач (Этап 7).
// Значения зафиксированы как константы, чтобы избежать коллизий между
// разными версиями приложения (что может случиться, если использовать hashtext).
const (
	LockIDCleanupHourly       int64 = 0x52704901 // "RepricerX cleanup-hourly"
	LockIDCompetitorRefresh   int64 = 0x52704902 // "RepricerX competitor-refresh"
	LockIDStalePlanCleanup    int64 = 0x52704903 // "RepricerX stale-plan-cleanup"
	LockIDDigestFlush         int64 = 0x52704904 // "RepricerX digest-flush"
)

// ReleaseFunc снимает lock и возвращает соединение в pool.
type ReleaseFunc func() error

// TryAcquire берёт session-level advisory lock с ключом lockID.
// Возвращает (true, release, nil) если успех; (false, noopRelease, nil) если
// другая сессия держит lock. release ВСЕГДА следует вызывать в defer
// (даже если acquired=false — он сделает no-op + release-conn).
func TryAcquire(ctx context.Context, pool *pgxpool.Pool, lockID int64) (bool, ReleaseFunc, error) {
	if pool == nil {
		return false, nopRelease, errors.New("dblock: nil pool")
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return false, nopRelease, fmt.Errorf("dblock: acquire conn: %w", err)
	}

	var got bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&got); err != nil {
		conn.Release()
		return false, nopRelease, fmt.Errorf("dblock: try lock: %w", err)
	}

	if !got {
		conn.Release()
		return false, nopRelease, nil
	}

	// Замыкание держит ссылку на conn до Release.
	released := false
	release := func() error {
		if released {
			return nil
		}
		released = true
		// Используем background context: release должен сработать даже после
		// отмены родительского ctx.
		_, err := conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockID)
		conn.Release()
		if err != nil {
			return fmt.Errorf("dblock: unlock: %w", err)
		}
		return nil
	}
	return true, release, nil
}

func nopRelease() error { return nil }
