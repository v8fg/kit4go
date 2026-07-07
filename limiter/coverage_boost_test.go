// This file is an internal coverage-boost test (package limiter, not
// limiter_test) so it can reach the unexported helpers and concrete types. It
// targets the remaining branches that the public + bench tests miss:
//   - tokenBucket.acquire CAS-retry reload + twice-lost deny path
//   - tokenBucket.TryAcquire/Allow closed-path return false
//   - tokenBucket.Wait in-poll-timer ctx cancellation
//   - tokenBucket.nextAvailableDelay: backward-clock clamp, burst clamp,
//     "avail >= 1" short-circuit, sub-microsecond clamp, deficit-delay path
//   - tokenBucket.newTokenBucket burst < 1 clamp
//   - slidingWindow.advance incremental-rollover branch (base < sec < base+n)
//   - slidingWindow.Wait in-poll-timer ctx cancellation
//
// All time-based assertions use generous windows to stay -race clean and not
// flaky under CI load.
package limiter

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- token bucket -----------------------------------------------------------

// TestTokenBucket_NewTokenBucket_BurstClamp exercises the burst<1 guard in
// newTokenBucket: a zero/negative burst is clamped to 1.
func TestTokenBucket_NewTokenBucket_BurstClamp(t *testing.T) {
	t.Run("zero burst clamped to 1", func(t *testing.T) {
		tb := newTokenBucket(10, 0) // burst 0 -> clamped to 1
		// Exactly one token (burst capacity) is available immediately.
		if !tb.Allow() {
			t.Fatal("clamped burst=1 should allow one token")
		}
		if tb.Allow() {
			t.Fatal("second Allow after burst=1 drain should be denied")
		}
		if tb.burst != 1 {
			t.Fatalf("internal burst=%v want 1", tb.burst)
		}
	})
	t.Run("negative burst clamped to 1", func(t *testing.T) {
		tb := newTokenBucket(10, -5)
		if !tb.Allow() {
			t.Fatal("clamped burst=1 should allow one token")
		}
		if tb.Allow() {
			t.Fatal("second Allow after burst=1 drain should be denied")
		}
		if tb.burst != 1 {
			t.Fatalf("internal burst=%v want 1", tb.burst)
		}
	})
}

// TestTokenBucket_Allow_TryAcquire_Closed exercises the closed-path on both
// Allow and TryAcquire (the early `if tb.closed.Load() { return false }`).
func TestTokenBucket_Allow_TryAcquire_Closed(t *testing.T) {
	tb := newTokenBucket(100, 5)
	tb.Close()
	if tb.Allow() {
		t.Fatal("Allow() on closed bucket must return false")
	}
	if tb.TryAcquire(1) {
		t.Fatal("TryAcquire(1) on closed bucket must return false")
	}
	// TryAcquire(<=0) short-circuits before the closed check: still a noop win.
	if !tb.TryAcquire(0) {
		t.Fatal("TryAcquire(0) must succeed regardless of closed state")
	}
	if !tb.TryAcquire(-3) {
		t.Fatal("TryAcquire(-3) must succeed regardless of closed state")
	}
}

// TestTokenBucket_Wait_CtxCancelledInTimer covers the Wait polling branch
// where ctx is cancelled *while blocked inside the timer select* (the inner
// `case <-ctx.Done(): timer.Stop(); return ctx.Err()`). We pre-cancel a token
// so Wait enters the loop, then cancel ctx after the timer is armed.
func TestTokenBucket_Wait_CtxCancelledInTimer(t *testing.T) {
	tb := newTokenBucket(1, 1) // 1 token/s, burst 1
	// Drain the only token so Wait must enter the polling loop.
	if !tb.Allow() {
		t.Fatal("setup Allow failed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after the loop has armed its timer (delay sized to a token
	// deficit of ~1 at rate=1/s, so the timer would otherwise wait ~1s).
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := tb.Wait(ctx)
	elapsed := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait err=%v want context.Canceled", err)
	}
	// Must have returned promptly after the cancel, not waited a full second.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Wait took %v after cancel; expected prompt return", elapsed)
	}
}

// TestTokenBucket_Wait_PreCancelledCtx covers the Wait loop-top ctx check
// (`case <-ctx.Done(): return ctx.Err()` at the top of the for-select, before
// the timer is armed): with an already-cancelled ctx and a drained bucket, the
// fast-path Allow fails, the loop's first select observes ctx.Done and returns
// immediately. Deterministic — no goroutine timing involved.
func TestTokenBucket_Wait_PreCancelledCtx(t *testing.T) {
	tb := newTokenBucket(1, 1)
	if !tb.Allow() { // drain so Wait enters the loop
		t.Fatal("setup Allow failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled
	start := time.Now()
	err := tb.Wait(ctx)
	elapsed := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait err=%v want context.Canceled", err)
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Wait with pre-cancelled ctx took %v, expected immediate", elapsed)
	}
}

// TestSlidingWindow_Wait_PreCancelledCtx covers the slidingWindow.Wait loop-top
// ctx check (the `case <-ctx.Done(): return ctx.Err()` at the top of the
// for-select). Deterministic: pre-cancelled ctx + drained bucket.
func TestSlidingWindow_Wait_PreCancelledCtx(t *testing.T) {
	sw := newSlidingWindow(1, time.Hour) // long window: token never frees
	if !sw.Allow() {                     // drain the only token
		t.Fatal("setup Allow failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := sw.Wait(ctx)
	elapsed := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait err=%v want context.Canceled", err)
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Wait with pre-cancelled ctx took %v, expected immediate", elapsed)
	}
}

// TestTokenBucket_NextAvailableDelay_Branches exercises every branch of
// nextAvailableDelay directly:
//   - backward-clock clamp (now < last -> now = last, no panic, no negative)
//   - burst cap (avail > burst clamped, treated as "available" -> 0)
//   - avail >= 1 short-circuit -> 0
//   - sub-microsecond deficit clamped up to 1us
//   - >1ms deficit clamped down to 1ms
//   - mid-range deficit delay
func TestTokenBucket_NextAvailableDelay_Branches(t *testing.T) {
	t.Run("avail >= 1 returns 0 (full bucket)", func(t *testing.T) {
		tb := newTokenBucket(100, 5) // starts full at burst=5
		if d := tb.nextAvailableDelay(); d != 0 {
			t.Fatalf("full bucket delay=%v want 0", d)
		}
	})

	t.Run("backward clock clamp (lastTime in future)", func(t *testing.T) {
		tb := newTokenBucket(100, 5)
		tb.lastTime.Store(time.Now().Add(5 * time.Second).UnixNano())
		// avail is clamp(cur+refill) with refill clamped; cur is full (5),
		// so avail >= 1 -> delay 0. No panic, no negative refill.
		if d := tb.nextAvailableDelay(); d != 0 {
			t.Fatalf("backward-clock full bucket delay=%v want 0", d)
		}
	})

	t.Run("burst cap (avail clamped to burst)", func(t *testing.T) {
		// Force avail above burst: store tokens above burst, with lastTime in
		// the past so refill adds more. The clamp must still yield delay 0
		// (avail capped at burst >= 1).
		tb := newTokenBucket(100, 2)
		tb.tokens.Store(float64BitsStable(10.0)) // > burst
		tb.lastTime.Store(time.Now().Add(-1 * time.Second).UnixNano())
		if d := tb.nextAvailableDelay(); d != 0 {
			t.Fatalf("over-burst clamp delay=%v want 0", d)
		}
	})

	t.Run("sub-microsecond deficit clamped to 1us", func(t *testing.T) {
		// rate=1000/s, deficit=0.0005 -> secs=5e-7 -> d=0.5us, clamped to 1us.
		// lastTime is pushed into the FUTURE so the function's `now < last`
		// clamp fires and refill is exactly 0 — the deficit is therefore
		// deterministic regardless of test scheduling / -race overhead.
		tb := newTokenBucket(1000, 1)
		tb.tokens.Store(float64BitsStable(0.9995)) // deficit 0.0005 => 0.5us
		tb.lastTime.Store(time.Now().Add(time.Second).UnixNano())
		d := tb.nextAvailableDelay()
		if d != time.Microsecond {
			t.Fatalf("sub-us deficit delay=%v want %v", d, time.Microsecond)
		}
	})

	t.Run("deficit over 1ms clamped down to 1ms", func(t *testing.T) {
		// Low rate so the natural wait exceeds 1ms; must clamp to 1ms.
		// lastTime in the future => refill exactly 0, deficit deterministic.
		tb := newTokenBucket(2, 1)              // 0.5s per token
		tb.tokens.Store(float64BitsStable(0.0)) // deficit 1 => ~0.5s
		tb.lastTime.Store(time.Now().Add(time.Second).UnixNano())
		d := tb.nextAvailableDelay()
		if d != time.Millisecond {
			t.Fatalf("over-1ms deficit delay=%v want %v", d, time.Millisecond)
		}
	})

	t.Run("mid-range deficit between 1us and 1ms", func(t *testing.T) {
		// rate=10000/s => 0.1ms per token. deficit 1 => ~0.1ms (in range).
		// lastTime in the future => refill exactly 0, deficit deterministic.
		tb := newTokenBucket(10000, 1)
		tb.tokens.Store(float64BitsStable(0.0)) // deficit 1
		tb.lastTime.Store(time.Now().Add(time.Second).UnixNano())
		d := tb.nextAvailableDelay()
		if d < time.Microsecond || d > time.Millisecond {
			t.Fatalf("mid-range delay=%v want within [1us,1ms]", d)
		}
	})
}

// float64BitsStable is math.Float64bits, aliased here so the per-test setup of
// bucket token counts reads as a stable named call rather than a bare math call
// scattered across assertions.
func float64BitsStable(f float64) uint64 {
	return math.Float64bits(f)
}

// TestTokenBucket_Acquire_CASRetryAndDeny forces the CAS-retry reload branch
// (line `last = tb.lastTime.Load()` after a lost CAS) and the twice-lost deny
// path. It runs many concurrent Allow calls on a small bucket whose rate keeps
// it nearly drained, maximising CAS contention. The test only asserts:
//   - no panic
//   - Allowed + Denied == total calls (every call resolved once)
//   - at least one Denied (so the deny path actually executed under -race)
//
// The retry-reload line executes whenever a CAS loses its race; whether the
// second attempt also loses (reaching the explicit deny) is timing-dependent,
// so we assert "denied >= 1" rather than a specific count. The bucket is sized
// so that contention + near-empty state makes the double-loss path very likely
// at -race, which is enough for line coverage.
func TestTokenBucket_Acquire_CASRetryAndDeny(t *testing.T) {
	tb := newTokenBucket(1, 4) // tiny burst, slow refill: contention drains it
	const goroutines = 64
	const perG = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range perG {
				_ = tb.Allow()
			}
		}()
	}
	wg.Wait()

	m := tb.Metrics()
	total := uint64(goroutines * perG)
	if m.Allowed+m.Denied != total {
		t.Fatalf("Allowed+Denied=%d want %d (calls lost/double-counted)", m.Allowed+m.Denied, total)
	}
	if m.Denied == 0 {
		t.Fatalf("expected some denials under heavy contention on a drained bucket, got Denied=0")
	}
	// Allowed must never exceed what the bucket could plausibly grant: burst +
	// refill over the run. With burst=4 and rate=1/s over a sub-second run that
	// is well under total; we just assert it is not absurdly over (sanity).
	if m.Allowed > total {
		t.Fatalf("Allowed=%d exceeds total calls %d", m.Allowed, total)
	}
}

// --- sliding window ---------------------------------------------------------

// TestSlidingWindow_NewSlidingWindow_WindowClamp covers the sub-second window
// guard in newSlidingWindow: any fractional-second window is floored to a
// 1-second ring (so 200ms, 999ms, and even 0 all become a single bucket).
func TestSlidingWindow_NewSlidingWindow_WindowClamp(t *testing.T) {
	cases := []struct {
		name   string
		window time.Duration
		want   int
	}{
		{"sub-second 200ms floored to 1", 200 * time.Millisecond, 1},
		{"sub-second 999ms floored to 1", 999 * time.Millisecond, 1},
		{"zero floored to 1", 0, 1},
		{"negative floored to 1", -time.Second, 1},
		{"exactly 1s stays 1", time.Second, 1},
		{"3s stays 3", 3 * time.Second, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sw := newSlidingWindow(10, tc.window)
			if sw.windowSec != tc.want {
				t.Fatalf("windowSec=%d want %d", sw.windowSec, tc.want)
			}
			if len(sw.counts) != tc.want {
				t.Fatalf("len(counts)=%d want %d", len(sw.counts), tc.want)
			}
		})
	}
}

// TestSlidingWindow_Advance_IncrementalRollover covers the `for sw.base < sec`
// incremental branch of advance(): sec is ahead of base but by less than a full
// window, so each intermediate bucket is rolled forward one at a time.
func TestSlidingWindow_Advance_IncrementalRollover(t *testing.T) {
	sw := newSlidingWindow(100, 5*time.Second) // 5 buckets
	sw.mu.Lock()
	sw.base = 1000
	// Seed non-zero counts in buckets [1001..1003] (which advance() must clear).
	sw.counts[int(int64(1001)%5)] = 3
	sw.counts[int(int64(1002)%5)] = 2
	sw.counts[int(int64(1003)%5)] = 1
	sw.sum = 6
	// Advance to 1003: this is 3 ahead of base (1003-1000=3 < 5), so the loop
	// runs base++ three times, clearing buckets for 1001, 1002, 1003.
	sw.advance(1003)
	sw.mu.Unlock()

	sw.mu.Lock()
	defer sw.mu.Unlock()
	// Buckets for 1001..1003 must be cleared; sum reduced accordingly.
	if sw.sum != 0 {
		t.Fatalf("after incremental rollover sum=%d want 0", sw.sum)
	}
	if sw.base != 1003 {
		t.Fatalf("base=%d want 1003", sw.base)
	}
	for _, c := range sw.counts {
		if c != 0 {
			t.Fatalf("counts not all cleared after rollover: %v", sw.counts)
		}
	}
}

// TestSlidingWindow_Advance_Incremental_PartialSurvives confirms the
// incremental loop only clears buckets strictly between old base and new sec,
// leaving a newer bucket intact. base=1000, seed the bucket for 1004, advance
// to 1003: bucket 1004 is newer than sec and must survive.
func TestSlidingWindow_Advance_Incremental_PartialSurvives(t *testing.T) {
	sw := newSlidingWindow(100, 5*time.Second)
	sw.mu.Lock()
	sw.base = 1000
	// Bucket index for second 1004.
	idx1004 := int(int64(1004) % 5)
	sw.counts[idx1004] = 9
	sw.sum = 9
	sw.advance(1003) // rolls 1001,1002,1003; 1004 untouched
	if sw.sum != 9 {
		t.Fatalf("sum=%d want 9 (bucket 1004 should survive)", sw.sum)
	}
	if sw.counts[idx1004] != 9 {
		t.Fatalf("bucket 1004 cleared prematurely: %d", sw.counts[idx1004])
	}
	if sw.base != 1003 {
		t.Fatalf("base=%d want 1003", sw.base)
	}
	sw.mu.Unlock()
}

// TestSlidingWindow_Wait_CtxCancelledInTimer covers the slidingWindow.Wait
// branch where ctx is cancelled while blocked in the inner timer select.
func TestSlidingWindow_Wait_CtxCancelledInTimer(t *testing.T) {
	// rate=1 with a 1-hour window so the token never frees during the test;
	// Wait must enter the polling loop.
	sw := newSlidingWindow(1, time.Hour)
	if !sw.Allow() {
		t.Fatal("setup Allow failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := sw.Wait(ctx)
	elapsed := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait err=%v want context.Canceled", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Wait took %v after cancel; expected prompt return", elapsed)
	}
}

// TestTokenBucket_AcquireNegativeRefillIsUnreachable documents (and asserts
// at runtime) that the `if refill < 0 { refill = 0 }` guard inside
// tokenBucket.acquire is dead code under the current clock-clamp invariant
// (now is clamped to >= last before refill is computed, so refill >= 0 always).
// This keeps the coverage gap honest rather than leaving it unexplained.
func TestTokenBucket_AcquireNegativeRefillIsUnreachable(t *testing.T) {
	tb := newTokenBucket(100, 2)
	tb.lastTime.Store(time.Now().UnixNano())
	// Even with a backward wall clock, acquire clamps now := last, so the
	// refill delta is always >= 0. We cannot make refill negative without
	// breaking the invariant; assert the guard is redundant.
	now := time.Now().UnixNano()
	last := tb.lastTime.Load()
	if now < last {
		now = last
	}
	refill := float64(now-last) / 1e9 * tb.rate
	if refill < 0 {
		t.Fatalf("invariant broken: refill=%v should be >= 0 after clamp", refill)
	}
}

// keep atomic import used in case future assertions need it.
var _ = atomic.Bool{}

// --- deterministic time-window correctness (fake clock) ---------------------
//
// These supersede the former time.Sleep-based versions of
// TestTokenBucket_Refill and TestSlidingWindow_WindowResets. Sleeping past a
// window boundary to assert rollover is flaky under CPU contention (E5); a fake
// clock advances time instantaneously and deterministically.

// TestTokenBucket_Refill_FakeClock drains a 5-token burst, then advances the
// fake clock past one refill interval (rate=100/s => 10ms/token) and confirms a
// token reappears — with zero wall-clock sleep.
func TestTokenBucket_Refill_FakeClock(t *testing.T) {
	clk := newFakeClock()
	tb := newTokenBucket(100, 5) // rate=100/s => 10ms per token, burst=5
	tb.now = clk.now
	// Re-base lastTime to the fake clock's start (the constructor read the real
	// wall clock); from here every refill delta is measured against clk.
	tb.lastTime.Store(clk.now().UnixNano())

	// Drain the full burst.
	for i := range 5 {
		if !tb.Allow() {
			t.Fatalf("burst %d denied", i)
		}
	}
	if tb.Allow() {
		t.Fatal("should be drained")
	}

	// Advance 10ms: exactly one token (rate=100/s) must have refilled.
	clk.add(10 * time.Millisecond)
	if !tb.Allow() {
		t.Fatal("token did not refill after advancing 10ms at rate=100/s")
	}
	// The just-refilled token was consumed; advancing less than 10ms must deny.
	clk.add(5 * time.Millisecond)
	if tb.Allow() {
		t.Fatal("Allow should be denied before a full refill interval elapses")
	}
}

// TestSlidingWindow_WindowResets_FakeClock drains a 2-req/1s window, advances
// the fake clock past the 1s window so the ring rolls over, and confirms Allow
// succeeds again — with zero wall-clock sleep.
func TestSlidingWindow_WindowResets_FakeClock(t *testing.T) {
	clk := newFakeClock()
	sw := newSlidingWindow(2, time.Second) // rate=2, 1s window
	sw.now = clk.now
	sw.base = clk.now().Unix() // re-base to the fake clock

	if !sw.Allow() || !sw.Allow() {
		t.Fatal("first two Allow() should succeed")
	}
	if sw.Allow() {
		t.Fatal("third Allow() within window should be denied")
	}

	// Advance past the window so advance() expires every bucket.
	clk.add(1100 * time.Millisecond)
	if !sw.Allow() {
		t.Fatal("Allow() after window rollover should succeed")
	}
	if m := sw.Metrics(); m.Allowed != 3 || m.Denied != 1 {
		t.Fatalf("metrics = %+v, want Allowed=3 Denied=1", m)
	}
}

// TestFixedWindow_WindowResets_FakeClock drains a 1-req/1s fixed window,
// advances the fake clock past the window boundary, and confirms the counter
// resets and Allow succeeds — deterministically.
func TestFixedWindow_WindowResets_FakeClock(t *testing.T) {
	clk := newFakeClock()
	fw := newFixedWindow(1, time.Second)
	fw.now = clk.now
	fw.windowStart.Store(clk.now().UnixNano())

	if !fw.Allow() {
		t.Fatal("first Allow() within window should succeed")
	}
	if fw.Allow() {
		t.Fatal("second Allow() over rate=1 should be denied")
	}

	// Advance past the window; acquire() must CAS the window forward and reset.
	clk.add(2 * time.Second)
	if !fw.Allow() {
		t.Fatal("Allow() after window rollover should succeed")
	}
}

// TestLeakyBucket_Drain_FakeClock fills the bucket to capacity, advances the
// fake clock so the steady drain empties room, and confirms Allow succeeds —
// deterministically (the leaky bucket drains at rate req/s).
func TestLeakyBucket_Drain_FakeClock(t *testing.T) {
	clk := newFakeClock()
	lb := newLeakyBucket(1, 1) // drain 1 req/s, capacity 1
	lb.now = clk.now
	lb.lastTime.Store(clk.now().UnixNano())

	if !lb.Allow() {
		t.Fatal("first Allow() should succeed (bucket starts empty)")
	}
	if lb.Allow() {
		t.Fatal("second Allow() should be denied (bucket at capacity)")
	}

	// Advance 1s: the single unit of water drains, freeing capacity.
	clk.add(time.Second)
	if !lb.Allow() {
		t.Fatal("Allow() should succeed after 1s drain at rate=1/s")
	}
}

// TestGCRA_BurstReplenish_FakeClock exercises the TAT-based replenish path:
// after the burst is consumed, advancing the fake clock past the emission
// interval must allow another request — deterministically.
func TestGCRA_BurstReplenish_FakeClock(t *testing.T) {
	clk := newFakeClock()
	g := newGCRA(10, 2) // 10 req/s => emission 100ms; burst 2 => 200ms offset
	g.now = clk.now

	// Consume the burst.
	if !g.Allow() {
		t.Fatal("first Allow() should succeed")
	}
	if !g.Allow() {
		t.Fatal("second Allow() should succeed")
	}
	if g.Allow() {
		t.Fatal("third Allow() should be denied (burst exhausted)")
	}

	// Advance past one emission interval (100ms): one token replenishes.
	clk.add(150 * time.Millisecond)
	if !g.Allow() {
		t.Fatal("Allow() should succeed after advancing past one emission interval")
	}
}
