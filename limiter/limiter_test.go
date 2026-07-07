package limiter_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/v8fg/kit4go/limiter"
)

// --- options ----------------------------------------------------------------

// withDefaults is unexported, so we assert its observable behaviour through
// NewLimiter: an UNSET (empty) Algorithm defaults to token bucket; a non-empty
// unrecognised Algorithm is rejected (nil); Rate <= 0 returns nil.

func TestLimiterOptions_defaultsViaFactory(t *testing.T) {
	t.Run("empty still yields a working limiter", func(t *testing.T) {
		// LimiterOptions{} has Rate=0, which NewLimiter rejects. The defaults
		// only kick in once Rate is valid; verify that path gives a token bucket.
		lm := limiter.NewLimiter(limiter.LimiterOptions{}) // Rate=0 -> nil
		if lm != nil {
			t.Fatal("Rate=0 must yield nil even with defaults")
		}
	})
	t.Run("empty algorithm + valid rate defaults to token bucket", func(t *testing.T) {
		// Algorithm="" is treated as UNSET and defaulted to token bucket.
		lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: "", Rate: 100})
		if lm == nil {
			t.Fatal("empty algorithm + valid Rate should default to token bucket, not nil")
		}
		// Token bucket default Burst=1: first Allow() succeeds, second is denied
		// at rate=1/s (default) — but we set Rate=100 so refill is fast. Just
		// assert the first call works.
		if !lm.Allow() {
			t.Fatal("default token bucket should allow its first token")
		}
	})
	t.Run("non-empty unknown algorithm returns nil (no silent fallback)", func(t *testing.T) {
		// A typo like "bogus" must NOT silently degrade to token-bucket semantics.
		// Only the empty string is treated as "unset".
		if lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: "bogus", Rate: 100}); lm != nil {
			t.Fatal("non-empty unknown algorithm must return nil, got a limiter (silent fallback footgun)")
		}
	})
	t.Run("sliding window with zero window falls back to 1s", func(t *testing.T) {
		// Window=0 must default to 1s; we can't observe the duration directly,
		// but we can confirm it behaves as a 1s window (no panic, allows within
		// rate).
		lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmSlidingWindow, Rate: 3})
		defer lm.Close()
		if !lm.Allow() {
			t.Fatal("sliding window with default Window should allow within rate")
		}
	})
}

// --- factory ---------------------------------------------------------------

func TestNewLimiter_Factory(t *testing.T) {
	t.Run("token bucket", func(t *testing.T) {
		lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmTokenBucket, Rate: 100, Burst: 5})
		if lm == nil {
			t.Fatal("expected non-nil token bucket limiter")
		}
		if _, ok := lm.(interface{ Allow() bool }); !ok {
			t.Fatal("limiter does not satisfy Allow")
		}
	})
	t.Run("sliding window", func(t *testing.T) {
		lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmSlidingWindow, Rate: 100, Window: time.Second})
		if lm == nil {
			t.Fatal("expected non-nil sliding window limiter")
		}
	})
	t.Run("invalid rate returns nil", func(t *testing.T) {
		if limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmTokenBucket, Rate: 0}) != nil {
			t.Fatal("Rate=0 should yield nil")
		}
		if limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmTokenBucket, Rate: -1}) != nil {
			t.Fatal("Rate<0 should yield nil")
		}
	})
	t.Run("non-empty unknown algorithm returns nil", func(t *testing.T) {
		// A non-empty unrecognised algorithm now reaches the switch's default arm
		// (withDefaults only defaults the EMPTY string) and is rejected as nil.
		// This honours the documented contract and rejects typos like "nope".
		if lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: "nope", Rate: 10}); lm != nil {
			t.Fatal("non-empty unknown algorithm must return nil, got a limiter (silent fallback footgun)")
		}
	})
	t.Run("empty algorithm defaults to token bucket", func(t *testing.T) {
		// Algorithm="" is treated as UNSET and defaulted to token bucket.
		lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: "", Rate: 10})
		if lm == nil {
			t.Fatal("empty algorithm should default to token bucket, got nil")
		}
		if !lm.Allow() {
			t.Fatal("default token bucket should allow its first token")
		}
	})
}

// --- token bucket ----------------------------------------------------------

func TestTokenBucket_BurstCapacity(t *testing.T) {
	const burst = 10
	lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmTokenBucket, Rate: 1, Burst: burst})
	defer lm.Close()

	got := 0
	for range burst {
		if lm.Allow() {
			got++
		}
	}
	if got != burst {
		t.Fatalf("initial burst: allowed %d, want %d", got, burst)
	}
	// Bucket is now drained; the next call should be denied (rate is only 1/s).
	if lm.Allow() {
		t.Fatal("expected Allow() to be denied immediately after draining the burst")
	}
}

func TestTokenBucket_TryAcquire(t *testing.T) {
	t.Run("fits in burst", func(t *testing.T) {
		lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmTokenBucket, Rate: 1, Burst: 10})
		defer lm.Close()
		if !lm.TryAcquire(5) {
			t.Fatal("TryAcquire(5) within burst=10 should succeed")
		}
		if m := lm.Metrics(); m.Acquired != 5 || m.Allowed != 1 {
			t.Fatalf("metrics after TryAcquire(5) = %+v", m)
		}
	})
	t.Run("exceeds burst", func(t *testing.T) {
		lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmTokenBucket, Rate: 1, Burst: 3})
		defer lm.Close()
		if lm.TryAcquire(4) {
			t.Fatal("TryAcquire(4) with burst=3 should fail")
		}
		if m := lm.Metrics(); m.Denied != 1 {
			t.Fatalf("denied metric = %d, want 1", m.Denied)
		}
	})
	t.Run("zero is noop success", func(t *testing.T) {
		lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmTokenBucket, Rate: 1, Burst: 1})
		defer lm.Close()
		if !lm.TryAcquire(0) {
			t.Fatal("TryAcquire(0) should succeed without consuming tokens")
		}
	})
}

func TestTokenBucket_Wait_Success(t *testing.T) {
	// Rate=1000/s => 1ms per token. Drain, then Wait should return promptly.
	lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmTokenBucket, Rate: 1000, Burst: 2})
	defer lm.Close()
	lm.Allow()
	lm.Allow()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	if err := lm.Wait(ctx); err != nil {
		t.Fatalf("Wait returned %v, want nil", err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("Wait took %v, expected prompt refill", d)
	}
}

func TestTokenBucket_Wait_Timeout(t *testing.T) {
	// Rate=1/s with a 1-token burst; drain it, then a 20ms ctx must expire.
	lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmTokenBucket, Rate: 1, Burst: 1})
	defer lm.Close()
	lm.Allow()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := lm.Wait(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait err = %v, want DeadlineExceeded", err)
	}
}

// --- sliding window --------------------------------------------------------

func TestSlidingWindow_WithinAndOverRate(t *testing.T) {
	// 1-second window, allow 5 requests.
	lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmSlidingWindow, Rate: 5, Window: time.Second})
	defer lm.Close()

	allowed := 0
	for range 5 {
		if lm.Allow() {
			allowed++
		}
	}
	if allowed != 5 {
		t.Fatalf("within rate: allowed %d, want 5", allowed)
	}
	// 6th in the same second must be denied.
	if lm.Allow() {
		t.Fatal("over rate: 6th Allow() should be denied")
	}
	if m := lm.Metrics(); m.Allowed != 5 || m.Denied != 1 {
		t.Fatalf("metrics = %+v, want Allowed=5 Denied=1", m)
	}
}

func TestSlidingWindow_TryAcquire(t *testing.T) {
	t.Run("fits", func(t *testing.T) {
		lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmSlidingWindow, Rate: 10, Window: time.Second})
		defer lm.Close()
		if !lm.TryAcquire(3) {
			t.Fatal("TryAcquire(3) under rate=10 should succeed")
		}
		if m := lm.Metrics(); m.Acquired != 3 {
			t.Fatalf("Acquired = %d, want 3", m.Acquired)
		}
	})
	t.Run("over rate", func(t *testing.T) {
		lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmSlidingWindow, Rate: 2, Window: time.Second})
		defer lm.Close()
		if lm.TryAcquire(3) {
			t.Fatal("TryAcquire(3) over rate=2 should fail")
		}
	})
}

// --- concurrency / race detector ------------------------------------------

func TestConcurrentAllow_TokenBucket(t *testing.T) {
	lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmTokenBucket, Rate: 1000, Burst: 100})
	defer lm.Close()

	const goroutines = 100
	const perG = 50
	var total atomic.Uint64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range perG {
				if lm.Allow() {
					total.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	m := lm.Metrics()
	// Allowed + Denied must equal every call exactly once.
	if got := m.Allowed + m.Denied; got != goroutines*perG {
		t.Fatalf("allowed+denied = %d, want %d (calls lost or double-counted)", got, goroutines*perG)
	}
	if total.Load() != m.Allowed {
		t.Fatalf("race: local count %d != metrics Allowed %d", total.Load(), m.Allowed)
	}
}

func TestConcurrentAllow_SlidingWindow(t *testing.T) {
	lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmSlidingWindow, Rate: 10000, Window: time.Second})
	defer lm.Close()

	const goroutines = 100
	const perG = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range perG {
				lm.Allow()
			}
		}()
	}
	wg.Wait()

	m := lm.Metrics()
	if got := m.Allowed + m.Denied; got != goroutines*perG {
		t.Fatalf("allowed+denied = %d, want %d", got, goroutines*perG)
	}
}

// --- Close / Metrics -------------------------------------------------------

func TestClose_IdempotentAndBlocksAllow(t *testing.T) {
	lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmTokenBucket, Rate: 100, Burst: 5})
	lm.Close()
	lm.Close() // idempotent
	if lm.Allow() {
		t.Fatal("Allow() after Close() must return false")
	}
	if !lm.TryAcquire(0) {
		t.Fatal("TryAcquire(0) should still be a noop success after Close")
	}
}

func TestSlidingWindow_CloseBlocksAllow(t *testing.T) {
	lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmSlidingWindow, Rate: 100, Window: time.Second})
	lm.Close()
	if lm.Allow() {
		t.Fatal("Allow() after Close() must return false")
	}
}

func TestMetrics_Accurate_TokenBucket(t *testing.T) {
	lm := limiter.NewLimiter(limiter.LimiterOptions{Algorithm: limiter.AlgorithmTokenBucket, Rate: 2, Burst: 3})
	defer lm.Close()

	// 3 should succeed (burst), 2 should fail.
	for i := range 3 {
		if !lm.Allow() {
			t.Fatalf("Allow %d failed", i)
		}
	}
	lm.Allow()
	lm.Allow()

	m := lm.Metrics()
	if m.Allowed != 3 {
		t.Errorf("Allowed = %d, want 3", m.Allowed)
	}
	if m.Acquired != 3 {
		t.Errorf("Acquired = %d, want 3", m.Acquired)
	}
	if m.Denied != 2 {
		t.Errorf("Denied = %d, want 2", m.Denied)
	}
}
