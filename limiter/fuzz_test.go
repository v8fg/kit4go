// This file is an internal fuzz test (package limiter, not limiter_test) so it
// can inject the shared fakeClock into the algorithm structs' unexported `now`
// field. Fuzzing the rate/burst space and an interleaving of clock advances +
// acquires exercises invariants the unit tests only spot-check:
//
//   - FuzzTokenBucketAllow: Allow never panics across rate/burst/clock inputs,
//     and the allow/deny decision is internally consistent with the configured
//     cap (a token-bucket never grants more than burst in zero elapsed time).
//   - FuzzGCRA: the Wait poll-interval helper nextDelay stays in its documented
//     valid range [0, time.Millisecond] regardless of emission/burst/TAT state.
//
// These run as ordinary tests (seed corpus only) under `go test -run='^Fuzz'`;
// pass `-fuzz=FuzzTokenBucketAllow` / `-fuzz=FuzzGCRA` to expand the corpus.
package limiter

import (
	"math"
	"testing"
	"time"
)

// rateFloor keeps rates away from zero so the algorithms stay well-defined
// (division by rate appears in both refill and emission math). Values at or
// below this are rejected by the seed/Oracle before construction.
const rateFloor = 1e-9

// FuzzTokenBucketAllow fuzzes the rate/burst configuration plus a small
// sequence of (advance-then-acquire) steps driven by a fake clock. It asserts:
//
//  1. Allow never panics for any input (including NaN/Inf rates, which the
//     constructor cannot reject — only NewLimiter clamps).
//  2. The decision is consistent with the burst cap: in zero elapsed time the
//     bucket grants at most `burst` tokens (refill is zero when the clock does
//     not advance). The single-goroutine fake clock removes CAS contention, so
//     the conservative deny-on-contention path never triggers spuriously.
//
// The first byte selects rate encoding; the second selects burst; the
// remainder is a step schedule encoded one byte per step (high bit = advance
// clock by low-7-bits ms, low bit = call Allow).
func FuzzTokenBucketAllow(f *testing.F) {
	// Seed with representative shapes: tiny/huge rates, burst clamping, no
	// advance, steady advance, and a full drain-then-refill cycle. Rates are
	// encoded via math.Float64bits so the intended value survives the
	// math.Float64frombits decode inside the target (passing the literal 100
	// would be read back as a denormal near zero).
	f.Add(math.Float64bits(100), uint8(5), []byte{0x01, 0x01, 0x80 | 10, 0x01})
	f.Add(math.Float64bits(1), uint8(1), []byte{0x01, 0x80 | 100, 0x01})
	f.Add(math.Float64bits(1_000_000), uint8(255), []byte{0x01, 0x01})
	f.Add(math.Float64bits(1e9), uint8(1<<7-1), []byte{0x80 | 1, 0x01})
	f.Add(math.Float64bits(0), uint8(0), []byte{0x01})                // zero rate/burst: constructor clamps
	f.Add(math.Float64bits(math.NaN()), uint8(3), []byte{0x01, 0x01}) // NaN rate: must not panic

	f.Fuzz(func(t *testing.T, rateBits uint64, burst uint8, steps []byte) {
		// Reconstruct the float64 rate the way NewLimiter cannot: directly from
		// bits, so NaN/Inf/negative reach the algorithm. The constructor
		// newTokenBucket does not validate rate (only burst<1), so this is the
		// honest input space for a fuzz target.
		rate := math.Float64frombits(rateBits)

		// Defer a panic guard so a divergence between the fuzz harness and the
		// production code surfaces as a test failure rather than crashing the
		// fuzz worker. (Allow is expected to be panic-free; this is belt-and-
		// braces.)
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Allow panicked: rate=%v burst=%d steps=%x recover=%v",
					rate, burst, steps, r)
			}
		}()

		clk := newFakeClock()
		tb := newTokenBucket(rate, int(burst))
		tb.now = clk.now
		// Re-base lastTime to the fake clock; the constructor read the real
		// wall clock, which would make the first refill delta meaningless.
		tb.lastTime.Store(clk.now().UnixNano())

		// Effective cap the bucket can grant in zero elapsed time. newTokenBucket
		// clamps burst<1 to 1, so mirror that here. A NaN/Inf rate makes the
		// refill term meaningless; for the consistency oracle we only count
		// grants that happen with no clock advance (refill == 0 when rate is
		// finite, or undefined when not — see below).
		burstCap := int(burst)
		if burstCap < 1 {
			burstCap = 1
		}
		rateFinite := !math.IsNaN(rate) && !math.IsInf(rate, 0)

		// grantsInZeroElapsed bounds how many Allow() calls may return true
		// while the clock is stationary. We reset it whenever we advance.
		grantsInZeroElapsed := 0

		for _, step := range steps {
			advance := time.Duration(step>>7) * time.Millisecond
			if advance > 0 {
				clk.add(advance)
				grantsInZeroElapsed = 0 // clock moved: refill may apply
			}
			if step&0x01 == 0 {
				continue // high-bit set with low bit clear: advance only, no Allow
			}

			got := tb.Allow()
			if !got {
				continue
			}
			// A grant happened. If the clock did not advance this iteration and
			// the rate is finite (so refill is a real, bounded number), the
			// grant must come out of the existing token count — which started at
			// burst and only decreases while the clock is still. Therefore the
			// running count of zero-elapsed grants cannot exceed burst.
			if advance == 0 && rateFinite && rate > 0 {
				grantsInZeroElapsed++
				if grantsInZeroElapsed > burstCap {
					t.Fatalf("bucket granted %d tokens in zero elapsed time, "+
						"exceeding burst cap %d (rate=%v steps=%x)",
						grantsInZeroElapsed, burstCap, rate, steps)
				}
			}
		}
	})
}

// FuzzGCRA fuzzes the GCRA emission interval and burst, drives a step schedule
// of acquires + clock advances, and asserts the Wait poll helper nextDelay
// always lands in its documented valid range: exactly 0 (token available now)
// or clamped to [time.Microsecond, time.Millisecond]. This catches any input
// that would make the deficit math underflow, overflow, or escape the clamps.
func FuzzGCRA(f *testing.F) {
	f.Add(math.Float64bits(10), uint8(2), []byte{0x01, 0x80 | 50, 0x01, 0x01})
	f.Add(math.Float64bits(1), uint8(1), []byte{0x01, 0x01, 0x80 | 127, 0x01})
	f.Add(math.Float64bits(1e6), uint8(64), []byte{0x01, 0x01, 0x01})
	f.Add(math.Float64bits(1e9), uint8(1), []byte{0x80 | 5, 0x01})
	f.Add(math.Float64bits(0), uint8(0), []byte{0x01}) // emission divides by zero space

	f.Fuzz(func(t *testing.T, rateBits uint64, burst uint8, steps []byte) {
		rate := math.Float64frombits(rateBits)

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("GCRA panicked: rate=%v burst=%d steps=%x recover=%v",
					rate, burst, steps, r)
			}
		}()

		// nextDelay divides nothing by rate at runtime (emission is precomputed
		// once in the constructor), but a non-finite / non-positive rate yields
		// a meaningless emissionNs. Skip those: they cannot exercise the
		// documented range contract, and the constructor would panic on Inf
		// converted to int64 during acquire. We still fuzz finite-positive rates
		// aggressively below.
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= rateFloor {
			t.Skip("non-finite or non-positive rate: outside nextDelay's contract")
		}

		clk := newFakeClock()
		g := newGCRA(rate, int(burst))
		g.now = clk.now

		for _, step := range steps {
			advance := time.Duration(step>>7) * time.Millisecond
			if advance > 0 {
				clk.add(advance)
			}
			if step&0x01 != 0 {
				_ = g.Allow()
			}

			// After every step, probe the Wait poll helper. It must always be
			// in [0, time.Millisecond], and any non-zero value must clear the
			// lower clamp (>= time.Microsecond) — a sub-microsecond non-zero
			// delay would be a clamp bug.
			d := g.nextDelay()
			if d < 0 {
				t.Fatalf("nextDelay = %v < 0 (rate=%v burst=%d steps=%x)",
					d, rate, burst, steps)
			}
			if d > time.Millisecond {
				t.Fatalf("nextDelay = %v > 1ms, escaped upper clamp "+
					"(rate=%v burst=%d steps=%x)", d, rate, burst, steps)
			}
			if d > 0 && d < time.Microsecond {
				t.Fatalf("nextDelay = %v in (0, 1us), escaped lower clamp "+
					"(rate=%v burst=%d steps=%x)", d, rate, burst, steps)
			}
		}
	})
}
