// Package ratelimit реализует per-key token bucket лимитер без внешних зависимостей.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

type bucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	rps        float64
	lastRefill time.Time
}

func newBucket(rps float64, burst float64) *bucket {
	return &bucket{
		tokens:    burst,
		maxTokens: burst,
		rps:       rps,
		lastRefill: time.Now(),
	}
}

func (b *bucket) wait(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.rps
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now

	if b.tokens >= 1 {
		b.tokens--
		return nil
	}

	waitTime := time.Duration((1 - b.tokens) / b.rps * float64(time.Second))
	b.tokens = 0

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(waitTime):
		return nil
	}
}

// Registry создаёт и кэширует по одному token bucket на ключ (shop_id).
// Лимитер создаётся при первом обращении (lazy, thread-safe).
type Registry struct {
	limiters sync.Map
	rps      float64
	burst    float64
}

// New создаёт реестр с заданным RPS и burst=rps*2.
func New(rps float64) *Registry {
	burst := rps * 2
	if burst < 1 {
		burst = 1
	}
	return &Registry{rps: rps, burst: burst}
}

// Wait блокируется до получения токена для заданного ключа.
// Возвращает ctx.Err() если контекст отменён.
func (r *Registry) Wait(ctx context.Context, key string) error {
	b := r.getOrCreate(key)
	return b.wait(ctx)
}

func (r *Registry) getOrCreate(key string) *bucket {
	if v, ok := r.limiters.Load(key); ok {
		return v.(*bucket)
	}
	b := newBucket(r.rps, r.burst)
	actual, _ := r.limiters.LoadOrStore(key, b)
	return actual.(*bucket)
}
