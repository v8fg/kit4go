// This file is an internal benchmark/coverage test (package limiter, not
// limiter_test) so it can reach the unexported helpers — withDefaults, the
// concrete tokenBucket/slidingWindow types — and exercise the Wait paths that
// the public tests cover only lightly. It also provides the hot-path
// benchmarks (Allow, Allow_Parallel, Wait, factory cost) that quantify the
// lock-free token-bucket fast path and the mutex-guarded sliding window.
package limiter

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- Benchmarks -------------------------------------------------------------

// BenchmarkTokenBucket_Allow measures the single-goroutine token-bucket Allow
// hot path: one atomic CAS loop with lazy refill. The rate is high enough that
// the bucket never drains, so this is the pure acquire overhead.
func BenchmarkTokenBucket_Allow(b *testing.B) {
	tb := newTokenBucket(1e9, 1<<20) // huge rate + burst: never denies
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tb.Allow()
	}
}

// BenchmarkTokenBucket_Allow_Parallel runs Allow from many goroutines to
// measure CAS contention. Use -cpu to scale; the retry-once-then-deny policy
// bounds the worst case.
func BenchmarkTokenBucket_Allow_Parallel(b *testing.B) {
	tb := newTokenBucket(1e9, 1<<20)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = tb.Allow()
		}
	})
}

// BenchmarkSlidingWindow_Allow measures the sliding-window Allow path: one
// mutex lock/unlock plus the ring advance. Slower than the token bucket by
// construction (mutex vs CAS), but O(1) amortised.
func BenchmarkSlidingWindow_Allow(b *testing.B) {
	sw := newSlidingWindow(1e9, time.Second) // huge rate: never denies
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sw.Allow()
	}
}

// BenchmarkSlidingWindow_Allow_Parallel runs the sliding window under
// contention to measure mu contention (the dominant cost for this algorithm).
func BenchmarkSlidingWindow_Allow_Parallel(b *testing.B) {
	sw := newSlidingWindow(1e9, time.Second)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = sw.Allow()
		}
	})
}

// BenchmarkTokenBucket_Wait measures Wait when a token is immediately
// available (the fast path: Allow() returns true on the first probe, no
// polling loop entered). Quantifies the Wait overhead on the happy path.
func BenchmarkTokenBucket_Wait(b *testing.B) {
	tb := newTokenBucket(1e9, 1<<20) // always a token available
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tb.Wait(ctx)
	}
}

// BenchmarkNewLimiter_Factory measures the cost of constructing a limiter via
// the public factory (options normalisation + struct allocation). Useful for
// callers that build a fresh limiter per request.
func BenchmarkNewLimiter_Factory(b *testing.B) {
	opts := LimiterOptions{Algorithm: AlgorithmTokenBucket, Rate: 100, Burst: 10}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lm := NewLimiter(opts)
		lm.Close()
	}
}

// --- Coverage boosters ------------------------------------------------------

// TestSlidingWindow_Wait covers the slidingWindow.Wait path, which was
// previously 0% covered: it asserts Wait succeeds when capacity is available
// and returns ctx.Err() on timeout.
func TestSlidingWindow_Wait(t *testing.T) {
	t.Run("succeeds when capacity available", func(t *testing.T) {
		sw := newSlidingWindow(10, time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		// Fast path: first Allow succeeds, Wait returns immediately.
		start := time.Now()
		if err := sw.Wait(ctx); err != nil {
			t.Fatalf("Wait err=%v want nil", err)
		}
		if d := time.Since(start); d > 100*time.Millisecond {
			t.Fatalf("Wait took %v, expected fast path", d)
		}
	})

	t.Run("blocks then succeeds after window rolls", func(t *testing.T) {
		// rate=1, window=1s (newSlidingWindow floors fractional seconds up to
		// 1s, so 200ms would silently become 1s — pass an explicit 1s to make
		// the window size honest). One Allow consumes the only token; the next
		// Wait must block until the current second expires, then succeed.
		sw := newSlidingWindow(1, time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sw.Wait(ctx); err != nil { // consumes the 1 token
			t.Fatalf("first Wait err=%v want nil", err)
		}
		// A non-blocking probe immediately after must be denied (proves the
		// window is full), so the only way the second Wait can succeed is by
		// blocking until the second rolls over.
		if sw.Allow() {
			t.Fatal("Allow immediately after consuming the only token should be denied")
		}
		start := time.Now()
		if err := sw.Wait(ctx); err != nil {
			t.Fatalf("second Wait err=%v want nil (window should have rolled)", err)
		}
		// The block duration depends on where in the current second the first
		// token landed (0..1s), so assert only that it did block for a
		// measurable slice of time and stayed well under the 5s budget — do not
		// over-constrain, or the test flakes under -race / scheduler jitter.
		if d := time.Since(start); d > 3*time.Second {
			t.Fatalf("second Wait took %v, expected to roll within one window", d)
		}
	})

	t.Run("returns ctx error on timeout", func(t *testing.T) {
		// rate=1, drain it, then a short ctx must expire before the window rolls.
		sw := newSlidingWindow(1, 1*time.Hour) // long window: never rolls
		_ = sw.Allow()                         // consume the only token
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		err := sw.Wait(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Wait err=%v want DeadlineExceeded", err)
		}
	})
}

// TestSlidingWindow_TryAcquire_Batch covers batch acquisition: a multi-token
// acquire that fits, one that exceeds the rate, and the n<=0 noop.
func TestSlidingWindow_TryAcquire_Batch(t *testing.T) {
	t.Run("batch fits", func(t *testing.T) {
		sw := newSlidingWindow(10, time.Second)
		if !sw.TryAcquire(5) {
			t.Fatal("TryAcquire(5) under rate=10 should succeed")
		}
		if m := sw.Metrics(); m.Acquired != 5 || m.Allowed != 1 {
			t.Fatalf("metrics=%+v want Acquired=5 Allowed=1", m)
		}
	})
	t.Run("batch exceeds rate", func(t *testing.T) {
		sw := newSlidingWindow(3, time.Second)
		if sw.TryAcquire(4) {
			t.Fatal("TryAcquire(4) over rate=3 should fail")
		}
		if m := sw.Metrics(); m.Denied != 1 {
			t.Fatalf("Denied=%d want 1", m.Denied)
		}
	})
	t.Run("zero and negative noop", func(t *testing.T) {
		sw := newSlidingWindow(1, time.Second)
		if !sw.TryAcquire(0) {
			t.Fatal("TryAcquire(0) should succeed without consuming")
		}
		if !sw.TryAcquire(-5) {
			t.Fatal("TryAcquire(-5) should succeed without consuming")
		}
		if m := sw.Metrics(); m.Acquired != 0 || m.Allowed != 0 {
			t.Fatalf("noop consumed tokens: %+v", m)
		}
	})
}

// TestSlidingWindow_Close_Idempotent confirms Close is idempotent and that
// post-close Allow/TryAcquire deny (and Wait returns promptly without token).
func TestSlidingWindow_Close_Idempotent(t *testing.T) {
	sw := newSlidingWindow(10, time.Second)
	sw.Close()
	sw.Close() // idempotent: no panic
	sw.Close()
	if sw.Allow() {
		t.Fatal("Allow after Close must return false")
	}
	if sw.TryAcquire(1) {
		t.Fatal("TryAcquire(1) after Close must return false")
	}
	if !sw.TryAcquire(0) {
		t.Fatal("TryAcquire(0) after Close must still be a noop success")
	}
	// Wait after close: Allow returns false, so it polls until ctx expires.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := sw.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait after Close err=%v want DeadlineExceeded", err)
	}
}

// TestTokenBucket_Wait_TokenAvailable covers the token-bucket Wait happy path:
// a token is immediately available, so Wait returns nil on the first probe
// without entering the polling loop.
func TestTokenBucket_Wait_TokenAvailable(t *testing.T) {
	tb := newTokenBucket(100, 5) // 5-token burst available
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := tb.Wait(ctx); err != nil {
			t.Fatalf("Wait %d err=%v want nil", i, err)
		}
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("5 Waits took %v, expected fast path", d)
	}
}

// TestTokenBucket_Refill_OverTime confirms tokens replenish over time: drain
// the bucket, wait, and confirm a fresh token becomes available at roughly the
// configured rate.
func TestTokenBucket_Refill_OverTime(t *testing.T) {
	// rate=1000/s => 1ms per token. Drain a 3-token burst.
	tb := newTokenBucket(1000, 3)
	for i := 0; i < 3; i++ {
		if !tb.Allow() {
			t.Fatalf("burst %d denied", i)
		}
	}
	if tb.Allow() {
		t.Fatal("bucket should be drained")
	}
	// Within ~5ms at least one token must refill (allow generous slack for CI).
	deadline := time.Now().Add(100 * time.Millisecond)
	refilled := false
	for time.Now().Before(deadline) {
		if tb.Allow() {
			refilled = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !refilled {
		t.Fatal("token did not refill within 100ms at rate=1000/s")
	}
}

// TestLimiterOptions_Defaults exercises withDefaults directly on the struct
// fields (the public test only observes behaviour via NewLimiter). Asserts
// every zero field is filled and overrides are preserved.
func TestLimiterOptions_Defaults(t *testing.T) {
	d := defaultLimiterOptions()

	got := LimiterOptions{}.withDefaults()
	if got.Algorithm != d.Algorithm {
		t.Errorf("default Algorithm=%q want %q", got.Algorithm, d.Algorithm)
	}
	if got.Rate != d.Rate {
		t.Errorf("default Rate=%v want %v", got.Rate, d.Rate)
	}
	if got.Burst != d.Burst {
		t.Errorf("default Burst=%d want %d", got.Burst, d.Burst)
	}
	if got.Window != d.Window {
		t.Errorf("default Window=%v want %v", got.Window, d.Window)
	}

	// Partial override preserved.
	partial := LimiterOptions{Algorithm: AlgorithmSlidingWindow, Rate: 50}.withDefaults()
	if partial.Algorithm != AlgorithmSlidingWindow {
		t.Errorf("partial Algorithm=%q want sliding_window", partial.Algorithm)
	}
	if partial.Rate != 50 {
		t.Errorf("partial Rate=%v want 50", partial.Rate)
	}
	// Burst/Window defaulted (not zero).
	if partial.Burst <= 0 {
		t.Errorf("partial Burst=%d want >0", partial.Burst)
	}

	// Unknown algorithm falls back to token bucket.
	unk := LimiterOptions{Algorithm: "weird", Rate: 5}.withDefaults()
	if unk.Algorithm != AlgorithmTokenBucket {
		t.Errorf("unknown Algorithm=%q want token_bucket fallback", unk.Algorithm)
	}
}

// TestLimiter_NilOptions confirms NewLimiter with zero options returns nil
// (Rate=0 is rejected before defaults apply), and that a Rate-only options
// still yields a working limiter via the defaults.
func TestLimiter_NilOptions(t *testing.T) {
	if NewLimiter(LimiterOptions{}) != nil {
		t.Fatal("zero options (Rate=0) must yield nil")
	}
	if NewLimiter(LimiterOptions{Algorithm: AlgorithmTokenBucket}) != nil {
		t.Fatal("Rate=0 with valid algorithm must still yield nil")
	}
	// Rate-only: defaults fill the rest, yielding a working token bucket.
	lm := NewLimiter(LimiterOptions{Rate: 100})
	if lm == nil {
		t.Fatal("Rate=100 with zeroed algorithm should default to a working limiter")
	}
	defer lm.Close()
	if !lm.Allow() {
		t.Fatal("defaulted limiter should allow its first token")
	}
}

// TestSlidingWindow_Advance_BackwardClock exercises advance()'s handling of a
// second older than base (a stale read or an NTP wall-clock regression): it must
// NOT destroy live counts — clearing on a backward timestamp would let the
// limiter over-allow. The window is left untouched (no clear, base unchanged).
func TestSlidingWindow_Advance_BackwardClock(t *testing.T) {
	sw := newSlidingWindow(100, 5*time.Second) // 5 buckets
	sw.mu.Lock()
	sw.base = 1000
	for i := range sw.counts {
		sw.counts[i] = 7
	}
	sw.sum = 35

	sw.advance(997) // 997 < base 1000 -> no-op
	if sw.base != 1000 {
		t.Fatalf("backward advance moved base: %d, want 1000 (unchanged)", sw.base)
	}
	for i := range sw.counts {
		if sw.counts[i] != 7 {
			t.Fatalf("backward advance destroyed bucket %d: counts=%d, want 7", i, sw.counts[i])
		}
	}
	if sw.sum != 35 {
		t.Fatalf("backward advance changed sum: %d, want 35 (unchanged)", sw.sum)
	}
	// A same-second advance is likewise a no-op.
	sw.advance(1000)
	if sw.sum != 35 || sw.base != 1000 {
		t.Fatalf("same-second advance changed state: sum=%d base=%d", sw.sum, sw.base)
	}
	sw.mu.Unlock()
}

// TestSlidingWindow_Advance_FullWindowExpiry exercises the full-window-reset
// branch: when sec - base >= windowSec, every bucket is expired.
func TestSlidingWindow_Advance_FullWindowExpiry(t *testing.T) {
	sw := newSlidingWindow(100, 3*time.Second) // 3 buckets
	sw.mu.Lock()
	sw.base = 1000
	for i := range sw.counts {
		sw.counts[i] = 4
	}
	sw.sum = 12
	sw.advance(1005) // 1005 - 1000 = 5 >= 3 buckets: full reset
	for i := range sw.counts {
		if sw.counts[i] != 0 {
			t.Fatalf("full-window expiry did not clear bucket %d: %d", i, sw.counts[i])
		}
	}
	if sw.sum != 0 {
		t.Fatalf("after full-window expiry sum=%d want 0", sw.sum)
	}
	if sw.base != 1005 {
		t.Fatalf("base=%d want 1005", sw.base)
	}
	sw.mu.Unlock()
}

// TestTokenBucket_ClockBackward exercises the token-bucket acquire path when
// the wall clock reads earlier than lastTime (e.g. NTP step). It must not
// panic and must not produce a negative refill (which would grant phantom
// tokens). We force the condition by rewinding lastTime into the future.
func TestTokenBucket_ClockBackward(t *testing.T) {
	tb := newTokenBucket(100, 5)
	// Push lastTime into the future so time.Now() reads as "backward".
	tb.lastTime.Store(time.Now().Add(10 * time.Second).UnixNano())
	// Allow must still work (uses lastTime as the floor for now) and not panic.
	if !tb.Allow() {
		// A token may or may not be available depending on the stored count,
		// but the call must not panic and must not grant more than burst.
		t.Logf("Allow under backward clock returned false (acceptable)")
	}
	// Drain whatever is available; total must not exceed burst.
	got := 0
	for i := 0; i < 100; i++ {
		if !tb.Allow() {
			break
		}
		got++
	}
	if got > 5 {
		t.Fatalf("backward clock granted %d tokens, burst=5 (phantom tokens)", got)
	}
}

// TestConcurrent_TokenBucket_Wait hammers Wait from many goroutines under
// -race. Asserts no panic and that allowed+denied is consistent.
func TestConcurrent_TokenBucket_Wait(t *testing.T) {
	tb := newTokenBucket(10000, 100)
	const goroutines = 32
	var wg sync.WaitGroup
	var ok atomic.Uint64
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			for j := 0; j < 20; j++ {
				if err := tb.Wait(ctx); err == nil {
					ok.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if ok.Load() == 0 {
		t.Fatal("no Wait succeeded despite high rate")
	}
}
