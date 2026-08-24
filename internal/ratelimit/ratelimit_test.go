package ratelimit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/ratelimit"
)

func TestMemory_BurstThenReject(t *testing.T) {
	limiter := ratelimit.NewMemory(ratelimit.Config{RequestsPerMinute: 60, Burst: 3})

	for i := range 3 {
		decision, err := limiter.Allow(t.Context(), "key-a")
		require.NoError(t, err)
		assert.True(t, decision.Allowed, "request %d is within the burst", i)
	}

	decision, err := limiter.Allow(t.Context(), "key-a")
	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Positive(t, decision.RetryAfter, "a rejected caller must be told when to come back")
	assert.GreaterOrEqual(t, decision.RetryAfter, time.Second,
		"a sub-second Retry-After would invite an immediate retry that fails again")
}

func TestMemory_BudgetsArePerKey(t *testing.T) {
	limiter := ratelimit.NewMemory(ratelimit.Config{RequestsPerMinute: 60, Burst: 1})

	first, err := limiter.Allow(t.Context(), "key-a")
	require.NoError(t, err)
	assert.True(t, first.Allowed)

	exhausted, err := limiter.Allow(t.Context(), "key-a")
	require.NoError(t, err)
	assert.False(t, exhausted.Allowed)

	other, err := limiter.Allow(t.Context(), "key-b")
	require.NoError(t, err)
	assert.True(t, other.Allowed, "one noisy key must not throttle another")
}

// A rejected request must not consume capacity, or a caller that retries
// steadily would starve itself for longer than the budget implies.
func TestMemory_RejectionDoesNotConsumeFutureCapacity(t *testing.T) {
	limiter := ratelimit.NewMemory(ratelimit.Config{RequestsPerMinute: 6000, Burst: 1})

	allowed, err := limiter.Allow(t.Context(), "key")
	require.NoError(t, err)
	require.True(t, allowed.Allowed)

	for range 5 {
		rejected, err := limiter.Allow(t.Context(), "key")
		require.NoError(t, err)
		require.False(t, rejected.Allowed)
	}

	// At 6000/minute a token refills in 10ms; the five rejections above must
	// not have pushed that out.
	time.Sleep(50 * time.Millisecond)
	recovered, err := limiter.Allow(t.Context(), "key")
	require.NoError(t, err)
	assert.True(t, recovered.Allowed)
}
