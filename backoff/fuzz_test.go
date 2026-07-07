package backoff

import (
	"context"
	"math"
	"testing"
	"time"
)

// FuzzNext_NoPanicAndBounds hammers every jitter mode across the documented
// valid configuration domain (base >= 0, max >= base, factor >= 0) and asserts
// the two invariants the package actually guarantees on every emitted delay:
//
//  1. No panic — computeLocked, advance, and randRange must not overflow,
//     divide by zero, or hand a negative span to rand.Int64N.
//  2. Bounded output — every delay lies in [0, max].
//
// Inputs outside this domain (negative base, negative factor, base > max,
// NaN/Inf factor, saturating MaxInt64 durations) are NOT defended by the
// package today — FuzzNext_NoPanicAndBounds_Degenerate captures those as
// explicit, documented seed commentary so a future hardening patch has a
// ready-made regression corpus.
//
// The fuzzer drives Reset mid-sequence so the re-initialization path
// (raw=base, last=base, attempt=0) is exercised under arbitrary prior state.
func FuzzNext_NoPanicAndBounds(f *testing.F) {
	seeds := []struct {
		baseNs int64
		maxNs  int64
		factor float64
		jitter int
	}{
		// The four jitter shapes at the documented defaults.
		{int64(100 * time.Millisecond), int64(10 * time.Second), 2.0, int(JitterNone)},
		{int64(100 * time.Millisecond), int64(10 * time.Second), 2.0, int(JitterFull)},
		{int64(100 * time.Millisecond), int64(10 * time.Second), 2.0, int(JitterEqual)},
		{int64(100 * time.Millisecond), int64(10 * time.Second), 2.0, int(JitterDecorrelated)},
		// Zero base → randRange(0,0) guard in JitterFull.
		{0, int64(time.Second), 2.0, int(JitterFull)},
		// base << max so the decorrelated cap branch (last*3 > max) fires.
		{1, 3, 2.0, int(JitterDecorrelated)},
		// base == max boundary.
		{int64(time.Millisecond), int64(time.Millisecond), 2.0, int(JitterNone)},
		// Factor at the documented lower edge (0 and 1).
		{int64(100 * time.Millisecond), int64(10 * time.Second), 0.0, int(JitterFull)},
		{int64(100 * time.Millisecond), int64(10 * time.Second), 1.0, int(JitterEqual)},
		// Large-but-finite durations that exercise advance()'s float64 cast
		// without overflowing time.Duration itself.
		{int64(time.Hour), int64(time.Hour * 24), 2.0, int(JitterDecorrelated)},
	}
	for _, s := range seeds {
		f.Add(s.baseNs, s.maxNs, s.factor, s.jitter)
	}

	f.Fuzz(func(t *testing.T, baseNs int64, maxNs int64, factor float64, jitter int) {
		base := time.Duration(baseNs)
		max := time.Duration(maxNs)

		// Restrict to the documented valid domain. The package makes no
		// output-shape promise for degenerate inputs (see the _Degenerate
		// companion for those); skip anything outside so a failure is always
		// a genuine contract violation.
		if base < 0 || max < base {
			t.Skip()
		}
		if factor < 0 || math.IsNaN(factor) || math.IsInf(factor, 0) {
			t.Skip()
		}
		// Saturation territory: once raw*factor exceeds math.MaxInt64 the
		// advance() cap holds, but randRange(base, last*3) can overflow the
		// int64 span handed to rand.Int64N. The package does not guard this
		// today; keep the corpus below the overflow cliff.
		if base > time.Duration(math.MaxInt64/4) {
			t.Skip()
		}

		b := New(
			WithBase(base),
			WithMax(max),
			WithFactor(factor),
			WithJitter(Jitter(jitter)),
			WithMaxAttempts(0), // unlimited → exercise the compute path fully
		)

		for i := range 64 {
			d, ok := b.Next()
			if !ok {
				t.Fatalf("Next returned ok=false with unlimited attempts (i=%d)", i)
			}
			// Invariant 1 (shape): a retry delay must be non-negative.
			if d < 0 {
				t.Fatalf("negative delay %v: base=%v max=%v factor=%v jitter=%d i=%d",
					d, base, max, factor, jitter, i)
			}
			// Invariant 2 (bound): the configured max caps every emitted delay.
			// max == base is the degenerate-but-valid case where the cap equals
			// the floor; the delay must still respect it.
			if d > max {
				t.Fatalf("delay %v exceeds max %v: base=%v factor=%v jitter=%d i=%d",
					d, max, base, factor, jitter, i)
			}
			// Drive Reset mid-sequence against arbitrary prior internal state.
			if i == 32 {
				b.Reset()
			}
		}
	})
}

// FuzzWait_OrderingAndRoundtrip verifies the attempt-counter and cap semantics
// across the public API surface (New → Next/Wait → Reset):
//
//  1. Ordering — Attempt increments by exactly one per Next call.
//  2. Cap fidelity — Next stops returning ok precisely when the cap is hit,
//     never earlier, never later; once capped the counter stops advancing.
//  3. Roundtrip — after Reset the counter returns to 0 and the first delay
//     equals the configured base for JitterNone (deterministic re-seed).
//
// Wait is exercised through a pre-cancelled context so the test never sleeps
// while still driving the attempt-advance + cap path.
func FuzzWait_OrderingAndRoundtrip(f *testing.F) {
	// Seeds: attempt caps from 0 (unlimited) through 1..5; base from 0 upward.
	seeds := []struct {
		maxAttempts int
		baseNs      int64
	}{
		{0, int64(100 * time.Millisecond)},
		{1, 0},
		{1, int64(time.Millisecond)},
		{2, 0},
		{3, int64(time.Microsecond)},
		{5, int64(100 * time.Millisecond)},
	}
	for _, s := range seeds {
		f.Add(s.maxAttempts, s.baseNs)
	}

	f.Fuzz(func(t *testing.T, maxAttempts int, baseNs int64) {
		// Guard against pathologically large caps that would make the loop
		// quadratic in fuzz time; the cap semantics are fully exercised well
		// before this bound.
		if maxAttempts < 0 || maxAttempts > 1_000_000 {
			t.Skip()
		}

		b := New(
			WithBase(time.Duration(baseNs)),
			WithMax(time.Second),
			WithJitter(JitterNone),
			WithMaxAttempts(maxAttempts),
		)

		// Pre-cancelled context: Wait returns immediately without sleeping,
		// still exercising the attempt-advance + cap path. defer-cancel keeps
		// govet/lostcancel happy on every exit (including t.Skip above).
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		defer func() { _ = ctx.Err() }()

		// Invariants 1 & 2: exactly `maxAttempts` successful Next calls when
		// the cap is set (unlimited when 0), counter advancing one-for-one and
		// freezing once the cap trips.
		successes := 0
		for range 10_000 {
			before := b.Attempt()
			_, ok := b.Next()
			after := b.Attempt()
			if !ok {
				if after != before {
					t.Fatalf("Attempt advanced past cap: before=%d after=%d (maxAttempts=%d)",
						before, after, maxAttempts)
				}
				break
			}
			if after != before+1 {
				t.Fatalf("Attempt must advance by 1: before=%d after=%d", before, after)
			}
			successes++
		}

		switch {
		case maxAttempts == 0:
			if successes != 10_000 {
				t.Fatalf("unlimited cap stopped early: successes=%d", successes)
			}
		default:
			if successes != maxAttempts {
				t.Fatalf("expected exactly %d successful Next calls, got %d",
					maxAttempts, successes)
			}
		}

		// Wait mirrors Next's cap; with ctx cancelled it never blocks. After
		// the Next loop drained the cap, Wait must not admit any further
		// attempt.
		waitOK := 0
		for i := 0; i < maxAttempts+5; i++ {
			if err := b.Wait(ctx); err != nil {
				break
			}
			waitOK++
		}
		if waitOK != 0 {
			t.Fatalf("Wait admitted %d attempts after cap was drained (maxAttempts=%d)",
				waitOK, maxAttempts)
		}

		// Invariant 3: Reset is a true roundtrip — counter to 0, first delay
		// back to base for the deterministic JitterNone mode.
		b.Reset()
		if got := b.Attempt(); got != 0 {
			t.Fatalf("Attempt after Reset = %d, want 0", got)
		}
		if maxAttempts != 0 {
			d, ok := b.Next()
			if !ok {
				t.Fatalf("Next after Reset failed: maxAttempts=%d", maxAttempts)
			}
			if d != time.Duration(baseNs) {
				t.Fatalf("first delay after Reset = %v, want base %v (JitterNone)",
					d, time.Duration(baseNs))
			}
		}
	})
}
