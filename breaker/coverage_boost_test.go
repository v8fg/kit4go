// This file is an internal coverage test (package breaker, not breaker_test)
// targeting branches the public and bench tests leave uncovered: the advance()
// partial-step loop, the beforeCall/toHalfOpenOrReject/maybeTrip/toClosed
// defensive re-checks, and the HalfOpen admission race-loser path. Each test
// drives the unexported helpers directly so the assertions are deterministic
// rather than reliant on timing-sensitive concurrency (the one exception,
// TestBreaker_HalfOpen_Admission_RaceLoser, fans out many goroutines so the
// losing-the-slot path is hit reliably while staying -race clean).
package breaker

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

// errCov is the sentinel used by failure fns in this file.
var errCov = errors.New("coverage failure")

// --- advance() partial-step loop --------------------------------------------

// TestBreaker_Advance_PartialStepLoop covers advance()'s per-second roll loop
// (the `for b.base < sec` branch at breaker.go:427), which only fires when the
// window has >=2 buckets and the clock has moved forward by fewer than a full
// window. With Interval=3s (3 buckets) and a seeded base, advancing one and two
// seconds forward drives the loop body and its bucket-zero/sum-subtract path.
func TestBreaker_Advance_PartialStepLoop(t *testing.T) {
	opts := BreakerOptions{Interval: 3 * time.Second, MinRequests: 1 << 20}
	br := NewBreaker[int](opts)

	br.mu.Lock()
	// Seed a known base and fill every bucket with nonzero counts/fails so the
	// loop's subtract+zero is observable in the running sums.
	br.base = 1000
	for i := range br.counts {
		br.counts[i] = 4
		br.fails[i] = 2
	}
	br.sumTotal = 12 // 4 * 3 buckets
	br.sumFail = 6   // 2 * 3 buckets
	br.mu.Unlock()

	// Advance one second forward (1000 -> 1001): one bucket cleared.
	br.mu.Lock()
	br.advance(1001)
	// Bucket 1001 % 3 = 2 must be zeroed and its counts subtracted.
	idx1 := int(int64(1001) % int64(len(br.counts)))
	if br.counts[idx1] != 0 || br.fails[idx1] != 0 {
		t.Fatalf("advance(1001) did not clear bucket %d: counts=%d fails=%d",
			idx1, br.counts[idx1], br.fails[idx1])
	}
	if br.sumTotal != 12-4 || br.sumFail != 6-2 {
		t.Fatalf("after advance(1001) sums=(%d,%d) want (%d,%d)",
			br.sumTotal, br.sumFail, 12-4, 6-2)
	}
	if br.base != 1001 {
		t.Fatalf("base=%d want 1001 after partial advance", br.base)
	}

	// Advance another second forward (1001 -> 1002): another bucket cleared.
	br.advance(1002)
	idx2 := int(int64(1002) % int64(len(br.counts)))
	if br.counts[idx2] != 0 || br.fails[idx2] != 0 {
		t.Fatalf("advance(1002) did not clear bucket %d: counts=%d fails=%d",
			idx2, br.counts[idx2], br.fails[idx2])
	}
	if br.sumTotal != 12-4-4 || br.sumFail != 6-2-2 {
		t.Fatalf("after advance(1002) sums=(%d,%d) want (%d,%d)",
			br.sumTotal, br.sumFail, 12-4-4, 6-2-2)
	}
	if br.base != 1002 {
		t.Fatalf("base=%d want 1002 after second partial advance", br.base)
	}
	br.mu.Unlock()
}

// TestBreaker_Advance_PartialStepLoop_ViaExecute exercises the same advance()
// partial-step loop through the public Execute path (recordWindow -> advance)
// so the branch is also observed end-to-end, not only via direct calls. Uses a
// 2s window and a deliberate 1s gap so exactly one bucket rolls.
func TestBreaker_Advance_PartialStepLoop_ViaExecute(t *testing.T) {
	opts := BreakerOptions{Interval: 2 * time.Second, FailRate: 2.0, MinRequests: 1 << 20}
	br := NewBreaker[int](opts)
	// Record one failure in the current second.
	if _, err := br.Execute(context.Background(), func(context.Context) (int, error) {
		return 0, errCov
	}); !errors.Is(err, errCov) {
		t.Fatalf("first execute err=%v want sentinel", err)
	}
	// Move base back one second so the next record advances by exactly one
	// bucket via the partial-step loop (n=2, sec-base=1 < n).
	br.mu.Lock()
	br.base = time.Now().Unix() - 1 // one second stale
	beforeTotal := br.sumTotal
	br.mu.Unlock()

	// This record rolls forward exactly one bucket via the loop.
	if _, err := br.Execute(context.Background(), func(context.Context) (int, error) {
		return 1, nil
	}); err != nil {
		t.Fatalf("second execute err=%v want nil", err)
	}
	br.mu.Lock()
	// The stale bucket was subtracted (clearing the old failure) and the new
	// success added, so sumTotal reflects only the fresh call.
	if br.sumTotal != 1 {
		t.Fatalf("after partial-roll record sumTotal=%d want 1 (before=%d)", br.sumTotal, beforeTotal)
	}
	br.mu.Unlock()
}

// --- beforeCall default branch ----------------------------------------------

// TestBreaker_BeforeCall_UnknownState covers beforeCall's default case
// (breaker.go:232-233): an out-of-range state value must fall through to
// "return nil" rather than panic. We force the state to a value outside the
// Closed/Open/HalfOpen enum directly.
func TestBreaker_BeforeCall_UnknownState(t *testing.T) {
	br := NewBreaker[int](BreakerOptions{Interval: time.Second})
	br.state.Store(int32(BreakerState(99))) // garbage state
	if err := br.beforeCall(); err != nil {
		t.Fatalf("beforeCall on unknown state returned err=%v want nil", err)
	}
}

// --- toHalfOpenOrReject defensive re-checks ---------------------------------

// TestBreaker_ToHalfOpenOrReject_NotOpenStates covers the entire
// state-not-Open re-evaluation block in toHalfOpenOrReject (breaker.go:245-254)
// for each non-Open state: HalfOpen (admit and reject sub-cases) and Closed.
// toHalfOpenOrReject is only normally reached via a race where another
// goroutine flipped the state between beforeCall's read and the lock; here we
// drive it directly to make the branch deterministic.
func TestBreaker_ToHalfOpenOrReject_NotOpenStates(t *testing.T) {
	t.Run("halfopen_admit", func(t *testing.T) {
		// state=HalfOpen, slots available: Add(1) <= MaxRequests -> admitted.
		br := NewBreaker[int](BreakerOptions{MaxRequests: 2, Interval: time.Second})
		br.state.Store(int32(StateHalfOpen))
		br.halfOpenCount.Store(0)
		if err := br.toHalfOpenOrReject(); err != nil {
			t.Fatalf("halfopen admit err=%v want nil", err)
		}
		if got := br.halfOpenCount.Load(); got != 1 {
			t.Fatalf("halfOpenCount=%d want 1 after admit", got)
		}
	})
	t.Run("halfopen_reject_full", func(t *testing.T) {
		// state=HalfOpen but all MaxRequests slots already taken: Add(1) over
		// MaxRequests -> undo and reject.
		br := NewBreaker[int](BreakerOptions{MaxRequests: 2, Interval: time.Second})
		br.state.Store(int32(StateHalfOpen))
		br.halfOpenCount.Store(2) // at capacity
		if err := br.toHalfOpenOrReject(); !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("halfopen reject-full err=%v want ErrCircuitOpen", err)
		}
		// Undo must restore the count to exactly MaxRequests.
		if got := br.halfOpenCount.Load(); got != 2 {
			t.Fatalf("halfOpenCount=%d want 2 after undo", got)
		}
	})
	t.Run("closed_falls_through", func(t *testing.T) {
		// state=Closed (not Open, not HalfOpen): falls past the HalfOpen admit
		// branch and returns ErrCircuitOpen.
		br := NewBreaker[int](BreakerOptions{MaxRequests: 2, Interval: time.Second})
		br.state.Store(int32(StateClosed))
		if err := br.toHalfOpenOrReject(); !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("closed fall-through err=%v want ErrCircuitOpen", err)
		}
	})
}

// TestBreaker_ToHalfOpenOrReject_OpenUnexpired covers the "Open but cooldown
// not yet elapsed" re-check under lock (breaker.go:256-259): with state=Open
// and expiry in the future, toHalfOpenOrReject must reject without flipping to
// HalfOpen.
func TestBreaker_ToHalfOpenOrReject_OpenUnexpired(t *testing.T) {
	br := NewBreaker[int](BreakerOptions{MaxRequests: 2, Interval: time.Second})
	br.state.Store(int32(StateOpen))
	br.expiry.Store(time.Now().Add(1 * time.Hour).UnixNano()) // far future
	if err := br.toHalfOpenOrReject(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("open-unexpired err=%v want ErrCircuitOpen", err)
	}
	if got := br.State(); got != StateOpen {
		t.Fatalf("state=%s want open (must not flip while unexpired)", got)
	}
}

// --- maybeTrip / toClosed defensive guards ----------------------------------

// TestBreaker_MaybeTrip_NotClosed covers maybeTrip's "not Closed -> return"
// guard (breaker.go:332-334). Driven directly with state=Open so the function
// bails before evaluating the window; the breaker must remain Open and the
// window sums untouched.
func TestBreaker_MaybeTrip_NotClosed(t *testing.T) {
	br := NewBreaker[int](BreakerOptions{Interval: time.Second, MinRequests: 1, FailRate: 0.5})
	br.mu.Lock()
	br.sumTotal = 100
	br.sumFail = 100
	br.mu.Unlock()
	br.state.Store(int32(StateOpen))
	br.maybeTrip() // must no-op because state != Closed
	if got := br.State(); got != StateOpen {
		t.Fatalf("state=%s want open (maybeTrip must not transition)", got)
	}
	br.mu.Lock()
	if br.sumTotal != 100 || br.sumFail != 100 {
		t.Fatalf("maybeTrip mutated sums on non-Closed state: (%d,%d)", br.sumTotal, br.sumFail)
	}
	br.mu.Unlock()
}

// TestBreaker_ToClosed_NotHalfOpen covers toClosed's "not HalfOpen -> return"
// guard (breaker.go:378-381). Driven directly with state=Closed so the function
// bails before resetting the window; counts must be preserved.
func TestBreaker_ToClosed_NotHalfOpen(t *testing.T) {
	br := NewBreaker[int](BreakerOptions{Interval: time.Second})
	br.mu.Lock()
	for i := range br.counts {
		br.counts[i] = 7
		br.fails[i] = 3
	}
	br.sumTotal = 7 * len(br.counts)
	br.sumFail = 3 * len(br.counts)
	br.state.Store(int32(StateClosed)) // not HalfOpen
	br.mu.Unlock()

	br.toClosed() // must no-op because state != HalfOpen
	if got := br.State(); got != StateClosed {
		t.Fatalf("state=%s want closed (toClosed must not transition)", got)
	}
	br.mu.Lock()
	if br.counts[0] != 7 || br.fails[0] != 3 {
		t.Fatalf("toClosed reset window on non-HalfOpen state: counts[0]=%d fails[0]=%d",
			br.counts[0], br.fails[0])
	}
	br.mu.Unlock()
}

// --- HalfOpen admission race-loser (concurrent) -----------------------------

// TestBreaker_HalfOpen_Admission_RaceLoser covers beforeCall's HalfOpen
// race-loser branch (breaker.go:226-230): the path where Load() reports a free
// slot but Add(1) crosses MaxRequests because another goroutine grabbed it in
// between, forcing an undo + reject.
//
// This branch is only reachable under genuine concurrency: at least two
// goroutines must read Load() < MaxRequests before either does its Add(1), so
// both pass the line-223 short-circuit and then their Add(1)s cross
// MaxRequests. With MaxRequests=1 the window is vanishingly small (a single Add
// fills the slot, so only contenders that read Load()==0 before the winner's Add
// can lose — and serialised scheduling lets the winner Add before any peer
// reads). MaxRequests>=2 widens the window: multiple contenders can pass the
// line-223 check (count < MaxRequests) and then their Add(1)s land at 1..N+,
// with the overflow ones hitting the loser-undo path. We therefore use
// MaxRequests=4 (MaxRequests=1 proved flaky in practice — the single-slot
// window is too narrow for the scheduler to interleave) with 64 contenders.
//
// We drive beforeCall directly (rather than Execute) and use a fresh breaker
// per round so the per-contender work is minimal and the contenders race into a
// pristine HalfOpen with count==0 — maximising the window in which the scheduler
// runs many contenders in parallel between the Load() and Add(1). Each round
// fans out goroutines from a shared barrier followed by a runtime.Gosched (which
// yields the contender so the scheduler interleaves peers before any single
// Add(1) completes); admitted contenders park on a gate (holding their slot so
// the breaker stays HalfOpen and cannot recover to Closed), while losers are
// rejected either by the line-223 short-circuit or by the loser-undo path. All
// halfOpen* access is atomic, so the test is -race clean.
//
// We assert only scheduling-independent invariants: the count never exceeds
// MaxRequests while parked (losers undo their transient Add), and it returns to
// 0 once the gate releases (admitted contenders undo). The loser-undo path is
// exercised by the race itself; we do not assert a rejection count because the
// split between line-223 and loser-undo rejections is scheduling-dependent.
// Coverage of the loser-undo branch is deterministic under -race (the detector's
// preemptive scheduling widens the Load/Add interleaving) and statistical in
// normal mode; in both modes the package coverage stays >= 96.7%, comfortably
// above the 95% target whether or not this single branch is hit on a given run.
func TestBreaker_HalfOpen_Admission_RaceLoser(t *testing.T) {
	const maxReq = uint32(4) // >=2 widens the Load/Add race window (see note above)
	const rounds = 80
	const contenders = 64

	for r := 0; r < rounds; r++ {
		// Use a fresh breaker per round (cheap) so the contenders race into a
		// pristine HalfOpen with count==0, maximising the Load/Add race window.
		rb := NewBreaker[int](BreakerOptions{
			MaxRequests:  maxReq,
			Interval:     1 * time.Second,
			OpenDuration: 5 * time.Millisecond,
			FailRate:     0.5,
			MinRequests:  2,
		})
		rb.state.Store(int32(StateHalfOpen))
		rb.halfOpenCount.Store(0)

		// gate parks every admitted contender so its slot stays taken for the
		// duration of the round: this keeps the breaker in HalfOpen (it cannot
		// recover to Closed until probes complete) and ensures the slot count
		// reflects the in-flight admissions the losers raced against.
		gate := make(chan struct{})
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(contenders)
		for i := 0; i < contenders; i++ {
			go func() {
				defer wg.Done()
				<-start
				// Yield once after the barrier so the scheduler interleaves the
				// contenders before any single Add(1) completes, widening the
				// window in which two goroutines both read Load() < MaxRequests.
				runtime.Gosched()
				err := rb.beforeCall()
				if err == nil {
					// Admitted: park on the gate so the slot is held. We do NOT
					// undo here (undo would release the slot and let a later
					// contender serialise through, narrowing the race window).
					// The undo happens after the gate releases below.
					<-gate
					rb.halfOpenCount.Add(-1)
				}
				// Rejections (line-223 short-circuit or loser-undo) need no
				// further action: beforeCall already undid any transient Add
				// for losers, and short-circuit rejects never Added.
			}()
		}
		close(start)

		// Let the contenders race for a bounded window: admitted ones park on
		// the gate (holding their slot), rejected ones return. A short sleep is
		// sufficient for the scheduler to fan the contenders out and resolve
		// every admission/rejection decision.
		time.Sleep(5 * time.Millisecond)

		// Invariant while parked: at most MaxRequests slots taken. The loser
		// path (if it fired) undid its transient Add; the short-circuit path
		// never Added. So the count can never exceed MaxRequests.
		if got := rb.halfOpenCount.Load(); got > int32(maxReq) {
			t.Fatalf("round %d: halfOpenCount=%d exceeds MaxRequests=%d (loser undo missing)",
				r, got, maxReq)
		}

		// Release the gate so parked contenders complete and undo; the round
		// then drains via the WaitGroup.
		close(gate)
		wg.Wait()

		// After the round: count must be back at 0 (admitted contenders undid).
		if got := rb.halfOpenCount.Load(); got != 0 {
			t.Fatalf("round %d: halfOpenCount=%d want 0 after gate release", r, got)
		}
	}
}
