package backoff

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNoneIsExponential(t *testing.T) {
	b := New(WithJitter(JitterNone), WithBase(10*time.Millisecond), WithFactor(2), WithMax(10*time.Second))
	var got []time.Duration
	for range 5 {
		d, ok := b.Next()
		require.True(t, ok)
		got = append(got, d)
	}
	// 10ms, 20ms, 40ms, 80ms, 160ms (no jitter).
	require.Equal(t, []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond, 160 * time.Millisecond}, got)
}

func TestCapped(t *testing.T) {
	b := New(WithJitter(JitterNone), WithBase(1*time.Second), WithFactor(10), WithMax(5*time.Second))
	for range 4 {
		d, _ := b.Next()
		require.LessOrEqual(t, d, 5*time.Second)
	}
	last, _ := b.Next()
	require.Equal(t, 5*time.Second, last) // saturated at cap
}

func TestMaxAttempts(t *testing.T) {
	b := New(WithMaxAttempts(3), WithJitter(JitterNone))
	_, ok := b.Next()
	require.True(t, ok)
	_, ok = b.Next()
	require.True(t, ok)
	_, ok = b.Next()
	require.True(t, ok)
	_, ok = b.Next()
	require.False(t, ok) // 4th attempt blocked
	require.Equal(t, 3, b.Attempt())
}

func TestReset(t *testing.T) {
	b := New(WithJitter(JitterNone), WithBase(10*time.Millisecond), WithFactor(2))
	b.Next()
	b.Next()
	require.Equal(t, 2, b.Attempt())
	b.Reset()
	require.Equal(t, 0, b.Attempt())
	d, _ := b.Next()
	require.Equal(t, 10*time.Millisecond, d)
}

func TestFullJitterBounds(t *testing.T) {
	b := New(WithJitter(JitterFull), WithBase(100*time.Millisecond), WithFactor(2), WithMax(10*time.Second))
	cur := 100 * time.Millisecond
	for range 6 {
		d, ok := b.Next()
		require.True(t, ok)
		require.LessOrEqual(t, d, cur, "full jitter must be <= exp")
		require.GreaterOrEqual(t, d, time.Duration(0))
		cur *= 2
		if cur > 10*time.Second {
			cur = 10 * time.Second
		}
	}
}

func TestEqualJitterBounds(t *testing.T) {
	b := New(WithJitter(JitterEqual), WithBase(100*time.Millisecond), WithFactor(2), WithMax(10*time.Second))
	cur := 100 * time.Millisecond
	for range 5 {
		d, ok := b.Next()
		require.True(t, ok)
		// equal jitter: exp/2 <= d <= exp.
		require.GreaterOrEqual(t, d, cur/2)
		require.LessOrEqual(t, d, cur)
		cur *= 2
		if cur > 10*time.Second {
			cur = 10 * time.Second
		}
	}
}

func TestDecorrelatedBounds(t *testing.T) {
	b := New(WithJitter(JitterDecorrelated), WithBase(50*time.Millisecond), WithMax(5*time.Second))
	prev := 50 * time.Millisecond
	for range 6 {
		d, ok := b.Next()
		require.True(t, ok)
		// decorrelated: base <= d <= min(max, last*3).
		upper := prev * 3
		if upper > 5*time.Second {
			upper = 5 * time.Second
		}
		require.GreaterOrEqual(t, d, 50*time.Millisecond)
		require.LessOrEqual(t, d, upper)
		prev = d
	}
}

func TestWaitSleepsAndAdvances(t *testing.T) {
	b := New(WithJitter(JitterNone), WithBase(5*time.Millisecond), WithFactor(2))
	start := time.Now()
	require.NoError(t, b.Wait(context.Background()))
	require.GreaterOrEqual(t, time.Since(start), 4*time.Millisecond) // slept ~5ms
	require.Equal(t, 1, b.Attempt())
}

func TestWaitContextCancel(t *testing.T) {
	b := New(WithJitter(JitterNone), WithBase(time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := b.Wait(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestWaitMaxAttempts(t *testing.T) {
	b := New(WithMaxAttempts(1), WithJitter(JitterNone), WithBase(time.Millisecond))
	require.NoError(t, b.Wait(context.Background()))
	require.ErrorIs(t, b.Wait(context.Background()), ErrMaxAttempts)
}

func TestDefaults(t *testing.T) {
	b := New() // default Full jitter, base 100ms, factor 2, max 10s
	d, ok := b.Next()
	require.True(t, ok)
	require.LessOrEqual(t, d, 100*time.Millisecond) // first exp value is base=100ms, full jitter <= that
}

func TestErrSentinel(t *testing.T) {
	require.True(t, errors.Is(ErrMaxAttempts, ErrMaxAttempts))
}

// TestDecorrelatedCapsAtMax covers the `if d > b.max { d = b.max }` branch in
// computeLocked (backoff.go:116-118). With base << max, last*3 quickly exceeds
// max, so randRange(base, last*3) routinely draws a value larger than max and
// the cap clamps it back down. This is reachable purely through the public API.
func TestDecorrelatedCapsAtMax(t *testing.T) {
	const max = 3 * time.Nanosecond
	b := New(WithJitter(JitterDecorrelated), WithBase(1), WithMax(max), WithFactor(2))
	for range 2000 {
		d, ok := b.Next()
		require.True(t, ok)
		require.LessOrEqual(t, d, max, "decorrelated delay must never exceed max")
	}
}

// TestFullJitterZeroBase covers the `if hi <= lo { return lo }` guard in
// randRange (backoff.go:186-188). With base=0 the first raw value is 0, so the
// Full-jitter call randRange(0, 0) hits hi<=lo and returns 0. Reachable via the
// public API.
func TestFullJitterZeroBase(t *testing.T) {
	b := New(WithBase(0), WithJitter(JitterFull), WithMax(time.Second))
	for range 5 {
		d, ok := b.Next()
		require.True(t, ok)
		require.Equal(t, time.Duration(0), d, "randRange(0,0) guard must return 0 while raw==0")
	}
}

// TestDecorrelatedGuardFloor covers the defensive `if hi < b.base { hi = b.base }`
// branch in computeLocked (backoff.go:112-114). This arm is unreachable through
// the public API: b.last is only ever assigned b.base (in New/Reset) or a
// returned delay d, and d >= b.base always holds (randRange(base, hi) with
// hi >= base yields d >= base), so the invariant b.last >= b.base makes
// b.last*3 >= b.base. The guard protects computeLocked against a corrupted
// internal state; exercise it here by forcing b.last below base/3 directly.
func TestDecorrelatedGuardFloor(t *testing.T) {
	const base = 30 * time.Nanosecond
	b := New(WithJitter(JitterDecorrelated), WithBase(base), WithMax(time.Second), WithFactor(2))
	// Break the invariant on purpose to reach the defensive floor.
	b.mu.Lock()
	b.last = 1 * time.Nanosecond // 1ns*3 = 3ns < base=30ns → forces hi = base
	b.mu.Unlock()
	// With hi clamped to base, randRange(base, base) yields exactly base. (Only
	// this first call hits the floor; the returned base restores the invariant,
	// so subsequent calls follow the normal path and are bounded only by max.)
	d, ok := b.Next()
	require.True(t, ok)
	require.Equal(t, base, d, "floor must clamp hi to base, yielding exactly base")
}

// TestNewFactorClamp is a regression test for the R19 P2 finding: New accepted
// any factor with no validation. A factor < 1 is nonsensical for a retry backoff
// (it would shrink or freeze delays). factor < 1 is now clamped to 1, which
// yields a constant delay sequence equal to base. factor=0 therefore produces
// base on every call (not 0 forever); factor=-2 is clamped to the same 1.
// On the old code this test FAILS: raw never advances because base*0==0, so the
// None-jitter delays collapse to 0 after the first advance.
func TestNewFactorClamp(t *testing.T) {
	t.Run("factor_zero_clamped_to_constant_base", func(t *testing.T) {
		const base = 50 * time.Millisecond
		b := New(WithBase(base), WithFactor(0), WithJitter(JitterNone), WithMax(time.Second))
		for range 5 {
			d, ok := b.Next()
			require.True(t, ok)
			require.Equal(t, base, d, "factor=0 must clamp to 1 → constant base delay")
			require.GreaterOrEqual(t, d, time.Duration(0), "delay must never be negative")
		}
	})
	t.Run("factor_negative_clamped_to_constant_base", func(t *testing.T) {
		const base = 50 * time.Millisecond
		b := New(WithBase(base), WithFactor(-2), WithJitter(JitterNone), WithMax(time.Second))
		for range 5 {
			d, ok := b.Next()
			require.True(t, ok)
			require.Equal(t, base, d, "factor=-2 must clamp to 1 → constant base delay")
		}
	})
}

// TestNewNegativeBaseClamped is a regression test for the R19 P2 finding: New
// accepted a negative base, which would then propagate negative delays through
// the public API. base < 0 is now clamped to 0. On the old code the Full-jitter
// first delay could be negative when base was negative.
func TestNewNegativeBaseClamped(t *testing.T) {
	b := New(WithBase(-time.Second), WithFactor(2), WithJitter(JitterNone), WithMax(time.Second))
	require.Equal(t, time.Duration(0), b.base, "negative base must clamp to 0")
	for range 5 {
		d, ok := b.Next()
		require.True(t, ok)
		require.GreaterOrEqual(t, d, time.Duration(0), "delay must never be negative after base clamp")
	}
}

// TestNewMaxBelowBaseRaised is a regression test for the R19 P2 finding: New
// allowed max < base, a contradictory configuration where the cap sat below the
// floor. max is now raised to at least base so the [base, max] interval is
// always valid.
func TestNewMaxBelowBaseRaised(t *testing.T) {
	const base = 100 * time.Millisecond
	b := New(WithBase(base), WithMax(10*time.Millisecond), WithJitter(JitterNone), WithFactor(2))
	require.GreaterOrEqual(t, b.max, b.base, "max must be >= base")
	require.Equal(t, base, b.max, "max below base must be raised to base")
	for range 5 {
		d, ok := b.Next()
		require.True(t, ok)
		require.GreaterOrEqual(t, d, time.Duration(0))
	}
}

// TestRandRangeNoOverflowAtMaxInt64 is a regression test for the R19 P2 finding:
// randRange(lo, hi) computed hi-lo+1, which overflows int64 to a negative when hi
// is near math.MaxInt64 — and rand.Int64N panics on a negative span. The guard
// clamps hi so the span is exactly MaxInt64. WithMax(time.Duration(MaxInt64))
// must not panic on any jitter mode. On the old code the decorrelated path
// (randRange(base, base*3) with base huge) or a huge Full span panicked.
func TestRandRangeNoOverflowAtMaxInt64(t *testing.T) {
	// Direct unit test of the overflow guard: span = hi-lo+1 must never wrap.
	t.Run("direct_huge_hi_no_panic", func(t *testing.T) {
		lo := time.Duration(0)
		hi := time.Duration(math.MaxInt64) // lo..hi span = MaxInt64+1 → overflows unguarded
		// Must not panic; result stays within [lo, hi].
		d := randRange(lo, hi)
		require.GreaterOrEqual(t, d, lo)
		require.LessOrEqual(t, d, hi)
	})

	// End-to-end: every jitter mode survives WithMax(MaxInt64) without panicking.
	jitters := []Jitter{JitterNone, JitterFull, JitterEqual, JitterDecorrelated}
	for _, j := range jitters {
		require.NotPanics(t, func() {
			b := New(
				WithBase(time.Duration(math.MaxInt64/4)), // large base → huge spans
				WithMax(time.Duration(math.MaxInt64)),
				WithFactor(2),
				WithJitter(j),
			)
			for range 10 {
				d, ok := b.Next()
				require.True(t, ok)
				require.GreaterOrEqual(t, d, time.Duration(0), "delay must never be negative")
			}
		}, "jitter %d must not panic at WithMax(MaxInt64)", j)
	}
}
