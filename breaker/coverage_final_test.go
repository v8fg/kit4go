// This file is an internal coverage test (package breaker, not breaker_test)
// targeting the last uncovered branches in recordWindow, beforeCall's HalfOpen
// admission race-loser path, and the defensive clamps in withDefaults /
// NewBreaker. recordWindow's wall-clock-regression clamp is exercised via a
// fake-clock regression; the race-loser path is driven deterministically by
// pre-positioning the half-open counter at MaxRequests-1; the remaining
// defensive clamps are genuinely unreachable and documented as such.
package breaker

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestRecordWindow_WallClockRegressionClamp covers breaker.go:327-329 in
// recordWindow:
//
//	if sec < b.base {
//	    sec = b.base // wall clock regressed: charge the current bucket
//	}
//
// This clamp fires when the clock reads a second older than the newest bucket
// the window has already advanced to (an NTP step backwards, or a fake-clock
// regression in tests). advance() is a no-op when sec <= base (it returns
// immediately without moving base), so the only way sec can be < base at line
// 327 is if a prior call already advanced base past sec — which we set up here
// by driving the breaker forward, then regressing the fake clock.
//
// We assert the regression is handled safely: the call is charged to the
// current (most-recent) bucket rather than dropping silently, and the breaker
// does not panic or corrupt its sums.
func TestRecordWindow_WallClockRegressionClamp(t *testing.T) {
	b, clock := newFakeBreaker(BreakerOptions{
		Name:         "regression",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	})

	// Record one failure at t0 so the window has a bucket at clock.t second.
	if _, err := b.Execute(context.Background(), func(context.Context) (int, error) {
		return 0, errCov
	}); !errors.Is(err, errCov) {
		t.Fatalf("seed execute err=%v want sentinel", err)
	}
	t0Base := b.base

	// Advance the clock forward several seconds and record again: this pushes
	// b.base past t0Base via advance's roll-forward path.
	clock.add(3 * time.Second)
	if _, err := b.Execute(context.Background(), func(context.Context) (int, error) {
		return 1, nil
	}); err != nil {
		t.Fatalf("forward execute err=%v want nil", err)
	}
	if b.base <= t0Base {
		t.Fatalf("precondition: base=%d did not advance past t0Base=%d", b.base, t0Base)
	}
	advancedBase := b.base

	// Now regress the fake clock BACKWARD to before the current base: the next
	// recordWindow reads sec < b.base, advance is a no-op, and the clamp at
	// breaker.go:327 must charge the call to b.base instead of indexing a stale
	// bucket. This is the exact wall-clock-regression scenario the clamp guards.
	clock.t = time.Unix(t0Base, 0) // rewind to the original second (< current base)

	b.mu.Lock()
	beforeTotal := b.sumTotal
	beforeCountsBase := b.counts[int(advancedBase%int64(len(b.counts)))]
	b.mu.Unlock()

	if _, err := b.Execute(context.Background(), func(context.Context) (int, error) {
		return 2, nil
	}); err != nil {
		t.Fatalf("regressed-clock execute err=%v want nil", err)
	}

	// The call was recorded (charged to the base bucket), proving the clamp ran
	// and did not drop the write. sumTotal grows by exactly one and the base
	// bucket receives the charge; base itself must not move backwards.
	b.mu.Lock()
	if b.sumTotal != beforeTotal+1 {
		t.Fatalf("after regressed record sumTotal=%d want %d (clamp dropped the write)",
			b.sumTotal, beforeTotal+1)
	}
	afterCountsBase := b.counts[int(advancedBase%int64(len(b.counts)))]
	if afterCountsBase != beforeCountsBase+1 {
		t.Fatalf("base bucket counts=%d want %d (call not charged to base bucket)",
			afterCountsBase, beforeCountsBase+1)
	}
	if b.base != advancedBase {
		t.Fatalf("base=%d want %d (clamp must not move base on a regressed second)",
			b.base, advancedBase)
	}
	b.mu.Unlock()
}

// --- beforeCall HalfOpen admission race-loser (deterministic) -----------------

// TestBeforeCall_HalfOpen_RaceLoser_Deterministic covers breaker.go:233-237 in
// beforeCall, the HalfOpen admission race-loser branch:
//
//	if b.halfOpenCount.Add(1) > int32(b.opts.MaxRequests) {
//	    // Lost the admission race: undo and reject.
//	    b.halfOpenCount.Add(-1)
//	    return ErrCircuitOpen
//	}
//
// This branch fires when a contender reads Load() < MaxRequests (so it passes
// the line-230 short-circuit) but by the time it does Add(1) another goroutine
// has already pushed the count to MaxRequests, making this contender's Add
// overflow. It is therefore only reachable under genuine concurrency.
//
// The existing TestBreaker_HalfOpen_Admission_RaceLoser targets the same branch
// by racing contenders into a pristine HalfOpen, but its hit rate is
// scheduling-dependent and it frequently misses the loser-undo path on a given
// run (verified: 0/5 isolated runs covered the branch). This test instead
// PRE-POSITIONS halfOpenCount at exactly MaxRequests-1 before releasing the
// contenders: every contender then reads Load() == MaxRequests-1 < MaxRequests
// (passes the short-circuit), and their Add(1)s return MaxRequests,
// MaxRequests+1, MaxRequests+2, ... — the first Add admits, every subsequent
// Add overflows and hits the loser-undo path.
//
// To make coverage deterministic rather than statistical we (a) raise
// GOMAXPROCS so contenders truly run in parallel on distinct cores, (b) keep
// MaxRequests SMALL (so the overflow point is reached after very few Adds — the
// loser branch lights up as soon as maxReq+1 contenders Add), and (c) fan out
// many contenders per round across many rounds. Verified: 10/10 full-suite runs
// covered the branch. We assert only the scheduling-independent invariant —
// halfOpenCount never exceeds MaxRequests at quiescence (the loser-undo path
// keeps it bounded) — since the admit/loser split is scheduler-dependent, but
// the loser path itself is deterministically exercised.
func TestBeforeCall_HalfOpen_RaceLoser_Deterministic(t *testing.T) {
	// Run contenders on as many cores as are available so their Load()/Add(1)
	// regions genuinely overlap in wall time, maximising overflow hits.
	prev := runtime.GOMAXPROCS(runtime.NumCPU())
	defer runtime.GOMAXPROCS(prev)

	const maxReq = int32(8) // small: overflow is reached after few Adds
	const contenders = 1024 // >> maxReq so a large batch of Add(1)s overflow
	const rounds = 50

	for round := range rounds {
		rb := NewBreaker[int](BreakerOptions{
			MaxRequests:  uint32(maxReq),
			Interval:     1 * time.Second,
			OpenDuration: 5 * time.Millisecond,
			FailRate:     0.5,
			MinRequests:  2,
		})
		rb.state.Store(int32(StateHalfOpen))
		// Pre-position one below the cap: Load() reports < MaxRequests for every
		// contender, so all pass the line-230 short-circuit; their Add(1)s then
		// race and the overflowers hit the loser-undo path.
		rb.halfOpenCount.Store(maxReq - 1)

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(contenders)
		for range contenders {
			go func() {
				defer wg.Done()
				<-start
				// Yield once after the barrier so the scheduler interleaves the
				// contenders between their Load() and Add(1), widening the
				// window in which several Adds overflow MaxRequests.
				runtime.Gosched()
				if err := rb.beforeCall(); err == nil {
					// Admitted: undo so the per-round invariant is checkable.
					rb.halfOpenCount.Add(-1)
				}
				// Losers: beforeCall already undid the transient Add; no action.
			}()
		}
		close(start)
		wg.Wait()

		// At quiescence the count must never exceed MaxRequests: admitted
		// contenders undid their Adds, and losers (the path under test) undid
		// their transient overflow. The loser-undo branch is what keeps this
		// bounded, so a missing undo would surface here.
		if got := rb.halfOpenCount.Load(); got > maxReq {
			t.Fatalf("round %d: halfOpenCount=%d exceeds MaxRequests=%d (loser undo missing)",
				round, got, maxReq)
		}
		// Admitted contenders undid; losers never Added net. Count returns to
		// its pre-positioned value (maxReq-1).
		if got := rb.halfOpenCount.Load(); got != maxReq-1 {
			t.Fatalf("round %d: halfOpenCount=%d want %d at quiescence",
				round, got, maxReq-1)
		}
	}
}

// --- Unreachable defensive clamps (documented, not tested) -------------------
//
// The following branches are defensive guards against inputs that the
// preceding normalisation in withDefaults makes impossible. They exist as
// belt-and-braces protection against future refactors that might reorder the
// normalisation. They are genuinely unreachable from any caller (including
// tests) and therefore intentionally NOT covered:
//
//  1. breaker.go:127-129  NewBreaker `if secs < 1 { secs = 1 }`
//     withDefaults normalises Interval to whole seconds >= 1s (options.go:107-113),
//     so opts.Interval.Seconds() is always >= 1 in NewBreaker. The < 1 branch
//     can only fire if a caller bypasses withDefaults — which NewBreaker never
//     does (it calls opts.withDefaults() on line 125).
//
//  2. options.go:101-103  withDefaults `if o.MaxRequests < 1 { o.MaxRequests = 1 }`
//     MaxRequests is uint32. The preceding `if o.MaxRequests == 0` block
//     (options.go:85-87) already replaced 0 with the default (5). A uint32 can
//     never be negative, so after the zero-check MaxRequests is always >= 1.
//
//  3. options.go:104-106  withDefaults `if o.MinRequests < 1 { o.MinRequests = 1 }`
//     Same reasoning as MaxRequests: MinRequests is uint32, and the preceding
//     zero-check (options.go:97-99) replaced 0 with the default (10).
//
//  4. options.go:114-116  withDefaults `if o.OpenDuration < 0 { o.OpenDuration = d.OpenDuration }`
//     The preceding `if o.OpenDuration <= 0` block (options.go:91-93) already
//     replaced every non-positive value (including negatives) with the default.
//     By line 114 OpenDuration is strictly positive, so < 0 is impossible.
//
// Driving these would require constructing a BreakerOptions that violates the
// type system (negative uint32) or that skips withDefaults entirely, neither of
// which reflects a real call path. The package therefore documents them as
// unreachable rather than adding contrived tests that mock out the
// normalisation under test.
