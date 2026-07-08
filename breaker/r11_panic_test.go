// This file holds the regression tests for R11 F3: a panicking fn in
// StateHalfOpen leaked its probe slot (the slot was taken in beforeCall but
// recordSuccess/recordFailure never ran because the panic unwound past them).
// After MaxRequests such panics the breaker wedged in HalfOpen — every probe
// slot dead, unable to admit the successes that would recover it or the
// failures that would re-trip it — which inverts the breaker's do-no-harm
// contract (its own protection disabled by the failure mode it exists to
// contain).
//
// The fix: in StateHalfOpen, Execute defers a recover that runs recordFailure
// (which trips to Open and resets halfOpenCount — the self-healing path) and
// then re-panics, so the caller still observes the original panic value
// (raw-panic contract preserved per the kit-callback convention, since fn runs
// on the synchronous caller's goroutine). Non-HalfOpen states keep the
// pre-existing raw-panic behaviour.
package breaker

import (
	"context"
	"errors"
	"testing"
	"time"
)

// panicSentinel is the value the probe fns panic with so the test can assert it
// propagates unchanged through the breaker's recover/re-panic.
var panicSentinel = errors.New("probe panic")

// tripToHalfOpen drives a fresh fake-clock breaker into StateHalfOpen with no
// probe slots yet taken, returning it ready for probe injection. It is the
// shared precondition for the regression tests below.
func tripToHalfOpen(t *testing.T, opts BreakerOptions) (*Breaker[int], *fakeClock) {
	t.Helper()
	b, clock := newFakeBreaker(opts)
	if failNTrips(b, errCov, 10) < 0 {
		t.Fatalf("breaker never tripped")
	}
	if b.State() != StateOpen {
		t.Fatalf("precondition: state=%s want open", b.State())
	}
	clock.add(opts.OpenDuration * 2) // deterministic OpenDuration expiry
	// The first post-expiry call flips Open -> HalfOpen and takes slot 1.
	if err := b.beforeCall(); err != nil {
		t.Fatalf("admit first probe: %v", err)
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("precondition: state=%s want half_open", b.State())
	}
	return b, clock
}

// TestR11F3_PanickingProbeReleasesHalfOpenSlot is the core regression: a
// panicking fn in HalfOpen must not leak its probe slot. On the old code the
// panic unwound before recordSuccess/recordFailure, leaving halfOpenCount
// bumped and the breaker still HalfOpen; after MaxRequests such panics the
// breaker was wedged (halfOpenCount stuck at MaxRequests, every subsequent
// probe rejected with ErrCircuitOpen).
//
// With the fix, recordFailure runs on the panic path — tripping to Open and
// resetting halfOpenCount — so the panicking probe frees its slot and moves the
// breaker to Open. This test would FAIL on the old code (which left the breaker
// in HalfOpen with halfOpenCount == 1 and no trip).
func TestR11F3_PanickingProbeReleasesHalfOpenSlot(t *testing.T) {
	opts := BreakerOptions{
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, _ := tripToHalfOpen(t, opts)

	got := catchPanicValue(func() {
		_, _ = b.Execute(context.Background(), func(context.Context) (int, error) {
			panic(panicSentinel)
		})
	})
	if got != panicSentinel {
		t.Fatalf("panic value = %#v, want panicSentinel (raw-panic contract)", got)
	}

	// Self-healing assertion: recordFailure should have tripped the breaker to
	// Open and reset halfOpenCount to 0. On the buggy code the breaker stays
	// HalfOpen with halfOpenCount == 1 (the leaked slot).
	if got := b.State(); got != StateOpen {
		t.Fatalf("after panicking probe state=%s want open (recordFailure should trip)", got)
	}
	if got := b.halfOpenCount.Load(); got != 0 {
		t.Fatalf("halfOpenCount=%d want 0 (slot must be released on panic)", got)
	}
}

// TestR11F3_BreakerNotWedgedAfterPanickingProbes reproduces the wedge the bug
// report describes: fire MaxRequests panicking probes (the case that fully
// saturates HalfOpen), then prove the breaker has not been wedged — healthy
// probes admitted after the cooldown must drive it back to Closed.
//
// On the old code, every one of the MaxRequests panicking probes leaked its
// slot, so halfOpenCount saturated at MaxRequests and the breaker was stuck in
// HalfOpen; every healthy probe was rejected with ErrCircuitOpen forever.
// With the fix, the first panicking probe trips to Open, so the breaker is
// never wedged and recovers cleanly once healthy traffic returns.
//
// This test would FAIL on the old code: the breaker ends up stuck in HalfOpen
// (halfOpenCount == MaxRequests) and never returns Closed.
func TestR11F3_BreakerNotWedgedAfterPanickingProbes(t *testing.T) {
	opts := BreakerOptions{
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, clock := tripToHalfOpen(t, opts)

	// Fire MaxRequests panicking probes. On buggy code these collectively wedge
	// the breaker; on fixed code the first trips to Open and the rest are
	// rejected (fn does not run) — caught either way.
	for i := 0; i < int(opts.MaxRequests); i++ {
		_ = catchPanicValue(func() {
			_, _ = b.Execute(context.Background(), func(context.Context) (int, error) {
				panic(panicSentinel)
			})
		})
	}

	// The breaker must NOT be wedged in HalfOpen. With the fix it tripped to
	// Open on the first panicking probe; on buggy code it is stuck HalfOpen.
	if got := b.State(); got == StateHalfOpen && b.halfOpenCount.Load() == int32(opts.MaxRequests) {
		t.Fatalf("breaker wedged: state=half_open halfOpenCount=%d (all slots leaked by panicking probes)",
			b.halfOpenCount.Load())
	}

	// Advance the cooldown (harmless if already past it) and recover via healthy
	// probes. On buggy code these are all rejected and the breaker stays
	// HalfOpen; on fixed code the breaker returns to Closed.
	clock.add(opts.OpenDuration * 2)
	for i := 0; i < int(opts.MaxRequests); i++ {
		v, err := b.Execute(context.Background(), func(context.Context) (int, error) {
			return i + 1, nil
		})
		if err != nil {
			t.Fatalf("healthy probe %d err=%v want nil (breaker wedged?)", i, err)
		}
		if v != i+1 {
			t.Fatalf("healthy probe %d value=%d want %d", i, v, i+1)
		}
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("after healthy probes state=%s want closed", got)
	}
}

// TestR11F3_ClosedPanicPropagatesRaw asserts the fix is scoped to HalfOpen
// only: in StateClosed a panicking fn must propagate raw with no recover (the
// pre-existing behaviour), and must NOT trip the breaker (window accounting is
// skipped, exactly as before the fix).
func TestR11F3_ClosedPanicPropagatesRaw(t *testing.T) {
	opts := BreakerOptions{
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, _ := newFakeBreaker(opts)
	if b.State() != StateClosed {
		t.Fatalf("precondition: state=%s want closed", b.State())
	}

	got := catchPanicValue(func() {
		_, _ = b.Execute(context.Background(), func(context.Context) (int, error) {
			panic(panicSentinel)
		})
	})
	if got != panicSentinel {
		t.Fatalf("closed panic value = %#v, want panicSentinel (raw propagation)", got)
	}
	// State stays Closed: the panic unwound before recordFailure touched the
	// window, which is the pre-fix behaviour the kit-callback convention
	// requires for the synchronous caller.
	if got := b.State(); got != StateClosed {
		t.Fatalf("after closed panic state=%s want closed (no trip)", got)
	}
}

// catchPanicValue runs fn, returns the recovered panic value (or nil if fn did
// not panic). Used so each probe's panic is contained for the assertion while
// the breaker's own recover/re-panic still runs.
func catchPanicValue(fn func()) (recovered any) {
	defer func() { recovered = recover() }()
	fn()
	return nil
}
