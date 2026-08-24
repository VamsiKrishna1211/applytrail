// Package ratelimit bounds how fast a single credential can call the API.
package ratelimit

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Decision is the outcome of a rate limit check.
type Decision struct {
	Allowed bool
	// RetryAfter is how long the caller should wait. It is only meaningful
	// when Allowed is false.
	RetryAfter time.Duration
}

// Limiter decides whether a request may proceed.
//
// The port exists so a multi-instance deployment can swap the in-memory
// implementation for a shared one (Redis, or a platform-level limiter) without
// touching the middleware.
type Limiter interface {
	Allow(ctx context.Context, key string) (Decision, error)
}

// Config describes the budget applied to each key.
type Config struct {
	RequestsPerMinute int
	Burst             int
	// IdleTTL is how long an unused bucket is kept before eviction. Buckets
	// are tiny, but a long-lived process that has seen many keys would
	// otherwise grow without bound.
	IdleTTL time.Duration
}

// Memory is an in-process token bucket per key.
//
// It is correct for a single instance, which is the normal ApplyTrail
// deployment: one scale-to-zero container serving one person's agent. Running
// several replicas multiplies the effective budget by the replica count, which
// is why the port above exists.
type Memory struct {
	cfg Config

	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var _ Limiter = (*Memory)(nil)

// NewMemory returns an in-process limiter.
func NewMemory(cfg Config) *Memory {
	if cfg.RequestsPerMinute <= 0 {
		cfg.RequestsPerMinute = 120
	}
	if cfg.Burst <= 0 {
		cfg.Burst = cfg.RequestsPerMinute
	}
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = 30 * time.Minute
	}
	return &Memory{cfg: cfg, buckets: map[string]*bucket{}, now: time.Now}
}

// Allow consumes one token for key.
func (m *Memory) Allow(_ context.Context, key string) (Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	b, ok := m.buckets[key]
	if !ok {
		// Sweep before inserting. Sweeping afterwards would evict the bucket
		// just created, since its lastSeen is still the zero time, and every
		// caller would get a fresh full budget on every request.
		m.evictStaleLocked(now)

		b = &bucket{
			limiter:  rate.NewLimiter(rate.Limit(float64(m.cfg.RequestsPerMinute)/60.0), m.cfg.Burst),
			lastSeen: now,
		}
		m.buckets[key] = b
	}
	b.lastSeen = now

	reservation := b.limiter.ReserveN(now, 1)
	if !reservation.OK() {
		// Only possible if burst is zero, which the constructor prevents.
		return Decision{Allowed: false, RetryAfter: time.Minute}, nil
	}

	delay := reservation.DelayFrom(now)
	if delay == 0 {
		return Decision{Allowed: true}, nil
	}

	// The request is over budget: give the token back so a rejected call does
	// not also consume future capacity, and tell the caller when to return.
	reservation.CancelAt(now)
	return Decision{Allowed: false, RetryAfter: roundUpSecond(delay)}, nil
}

func (m *Memory) evictStaleLocked(now time.Time) {
	for key, b := range m.buckets {
		if now.Sub(b.lastSeen) > m.cfg.IdleTTL {
			delete(m.buckets, key)
		}
	}
}

// roundUpSecond keeps Retry-After honest: a sub-second wait rendered as "0"
// would invite an immediate retry that fails again.
func roundUpSecond(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	if rounded := d.Round(time.Second); rounded >= d {
		return rounded
	}
	return d.Round(time.Second) + time.Second
}
