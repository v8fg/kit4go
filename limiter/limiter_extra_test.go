package limiter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// allAlgorithms returns one Limiter per algorithm, configured for the same
// steady rate. Used by table-driven tests that assert behavior holds across all.
func allAlgorithms(rate float64, burst int, window time.Duration) map[string]Limiter {
	return map[string]Limiter{
		"token_bucket":   NewLimiter(LimiterOptions{Algorithm: AlgorithmTokenBucket, Rate: rate, Burst: burst}),
		"sliding_window": NewLimiter(LimiterOptions{Algorithm: AlgorithmSlidingWindow, Rate: rate, Window: window}),
		"fixed_window":   NewLimiter(LimiterOptions{Algorithm: AlgorithmFixedWindow, Rate: rate, Window: window}),
		"leaky_bucket":   NewLimiter(LimiterOptions{Algorithm: AlgorithmLeakyBucket, Rate: rate, Burst: burst}),
		"gcra":           NewLimiter(LimiterOptions{Algorithm: AlgorithmGCRA, Rate: rate, Burst: burst}),
	}
}

func TestAllAlgorithmsNotNil(t *testing.T) {
	for name, l := range allAlgorithms(100, 10, time.Second) {
		require.NotNil(t, l, "%s should build a limiter", name)
	}
}

func TestAllAlgorithmsAllowAtLeastOnce(t *testing.T) {
	for name, l := range allAlgorithms(100, 10, time.Second) {
		require.True(t, l.Allow(), "%s: first Allow should succeed", name)
	}
}

func TestAllAlgorithmsRespectsBurst(t *testing.T) {
	// rate=1000/s, burst=5 → first 5 should pass, 6th should fail (within the
	// same instant). Sliding window and fixed window ignore burst, so skip them.
	for name, l := range allAlgorithms(1000, 5, time.Second) {
		switch name {
		case "sliding_window", "fixed_window":
			continue // these use rate-per-window, not burst
		}
		for i := range 5 {
			require.True(t, l.Allow(), "%s: burst token %d should pass", name, i)
		}
		// 6th within the same instant → denied (or at most a few more due to
		// time elapsed). We assert that it's NOT all-1000-allowed.
		denied := false
		for range 100 {
			if !l.Allow() {
				denied = true
				break
			}
		}
		require.True(t, denied, "%s: should deny after burst", name)
	}
}

func TestAllAlgorithmsTryAcquire(t *testing.T) {
	for name, l := range allAlgorithms(1000, 100, time.Second) {
		require.True(t, l.TryAcquire(3), "%s: TryAcquire(3) should pass", name)
		require.True(t, l.TryAcquire(0), "%s: TryAcquire(0) is a no-op success", name)
		require.True(t, l.TryAcquire(-1), "%s: TryAcquire(-1) is a no-op success", name)
	}
}

func TestAllAlgorithmsWaitAcquires(t *testing.T) {
	for name, l := range allAlgorithms(100, 1, time.Second) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err := l.Wait(ctx)
		require.NoError(t, err, "%s: Wait should acquire within timeout", name)
	}
}

func TestAllAlgorithmsCloseDisables(t *testing.T) {
	for name, l := range allAlgorithms(100, 10, time.Second) {
		l.Close()
		require.False(t, l.Allow(), "%s: Allow after Close should return false", name)
		require.False(t, l.TryAcquire(1), "%s: TryAcquire after Close should return false", name)
	}
}

func TestAllAlgorithmsMetrics(t *testing.T) {
	for name, l := range allAlgorithms(1000, 100, time.Second) {
		l.Allow()
		l.Allow()
		l.TryAcquire(3)
		m := l.Metrics()
		require.Positive(t, m.Allowed, "%s: Allowed > 0", name)
		require.Positive(t, m.Acquired, "%s: Acquired > 0", name)
	}
}

func TestAllAlgorithmsConcurrency(t *testing.T) {
	for name, l := range allAlgorithms(10000, 1000, time.Second) {
		var wg sync.WaitGroup
		const g = 8
		wg.Add(g)
		for range g {
			go func() {
				defer wg.Done()
				for range 100 {
					l.Allow()
				}
			}()
		}
		wg.Wait()
		// Must not panic or deadlock. Metrics reflect the run.
		require.Positive(t, l.Metrics().Acquired, "%s: concurrent run should have acquired", name)
	}
}

// --- Algorithm-specific tests ---

func TestFixedWindowRateCap(t *testing.T) {
	// rate=5 per 1s window: first 5 pass, 6th denied (same window).
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmFixedWindow, Rate: 5, Window: time.Second})
	for i := range 5 {
		require.True(t, l.Allow(), "token %d", i)
	}
	require.False(t, l.Allow(), "6th should be denied in same window")
}

func TestLeakyBucketStartsEmpty(t *testing.T) {
	// Leaky bucket starts empty → first requests pass until capacity is full.
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmLeakyBucket, Rate: 1000, Burst: 3})
	require.True(t, l.Allow())
	require.True(t, l.Allow())
	require.True(t, l.Allow())
	// 4th in the same instant → bucket full → deny.
	require.False(t, l.Allow())
}

func TestGCRABurstAndSteady(t *testing.T) {
	// rate=1000, burst=5 → first 5 pass instantly, then denied until time passes.
	l := NewLimiter(LimiterOptions{Algorithm: AlgorithmGCRA, Rate: 1000, Burst: 5})
	for i := range 5 {
		require.True(t, l.Allow(), "burst %d", i)
	}
	// 6th instantly → denied.
	require.False(t, l.Allow())
}

func TestUnknownAlgorithmDefaultsToTokenBucket(t *testing.T) {
	// Only the EMPTY algorithm is defaulted to token_bucket. A non-empty but
	// unrecognised algorithm is rejected (nil) — no silent fallback.
	l := NewLimiter(LimiterOptions{Algorithm: "", Rate: 100, Burst: 10})
	require.NotNil(t, l)
	require.True(t, l.Allow())

	// Non-empty unknown algorithm must return nil, not silently fall back.
	require.Nil(t, NewLimiter(LimiterOptions{Algorithm: "nonexistent", Rate: 100, Burst: 10}))
}
