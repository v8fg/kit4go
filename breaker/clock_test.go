// This file is an internal test (package breaker, not breaker_test) that
// replaces the time.Sleep-based state-transition assertions in breaker_test.go
// with deterministic fake-clock advancement. The production Breaker reads the
// wall clock only through its injectable now func field; here we substitute a
// fakeClock so OpenDuration expiry and sliding-window ageing are observed
// instantly and deterministically instead of via real time.Sleep (which was
// flaky under CPU contention — see E5). Tests that use Sleep purely for
// goroutine synchronisation are left untouched in their original files.
package breaker

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeClock is a controllable clock for deterministic time-window tests. It is
// not safe for concurrent mutation of t: tests that use it drive the breaker
// from a single goroutine (or advance the clock only while no Execute is in
// flight), matching how the production code reads now on the hot path.
type fakeClock struct {
	t time.Time
}

// now returns the clock's current time, satisfying the func() time.Time seam on
// Breaker.now.
func (f *fakeClock) now() time.Time { return f.t }

// add advances the fake clock by d, simulating the passage of wall-clock time
// without any real waiting.
func (f *fakeClock) add(d time.Duration) { f.t = f.t.Add(d) }

// newFakeBreaker builds a Breaker[int] whose clock is a fakeClock seeded at the
// given options, returning both so the test can advance time. The breaker's
// sliding-window base is re-anchored to the fake clock's start second so window
// accounting is consistent with subsequent fake advances.
func newFakeBreaker(opts BreakerOptions) (*Breaker[int], *fakeClock) {
	opts = opts.withDefaults()
	clock := &fakeClock{t: time.Unix(1_000_000, 0)}
	b := &Breaker[int]{
		opts:   opts,
		counts: make([]int, int(opts.Interval.Seconds())),
		fails:  make([]int, int(opts.Interval.Seconds())),
		base:   clock.t.Unix(),
		now:    clock.now,
	}
	b.state.Store(int32(StateClosed))
	return b, clock
}

// failNTrips drives a breaker to StateOpen by running failing fns until it
// trips. Mirrors the breaker_test.failNTrips helper but operates on the
// unexported *Breaker[int] so it can be used from this internal package.
func failNTrips(b *Breaker[int], failErr error, max int) int {
	for i := range max {
		_, err := b.Execute(context.Background(), func(context.Context) (int, error) {
			return 0, failErr
		})
		if errors.Is(err, ErrCircuitOpen) {
			return i
		}
	}
	return -1
}

// --- OpenDuration expiry transitions ------------------------------------------

// TestFakeClock_OpenToHalfOpen replaces TestBreaker_OpenToHalfOpen: after the
// fake clock advances past OpenDuration, the next call must transition the
// breaker to HalfOpen instead of rejecting.
func TestFakeClock_OpenToHalfOpen(t *testing.T) {
	opts := BreakerOptions{ // matches fastOpts()
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, clock := newFakeBreaker(opts)
	if failNTrips(b, errCov, 10) < 0 {
		t.Fatalf("breaker never tripped")
	}
	if b.State() != StateOpen {
		t.Fatalf("precondition: want open, got %s", b.State())
	}
	// Advance past OpenDuration: deterministic, no real waiting.
	clock.add(opts.OpenDuration * 2)
	_, err := b.Execute(context.Background(), func(context.Context) (int, error) {
		if got := b.State(); got != StateHalfOpen {
			t.Fatalf("during probe state=%s want half_open", got)
		}
		return 1, nil
	})
	if err != nil {
		t.Fatalf("first probe err=%v want nil", err)
	}
	if got := b.State(); got != StateHalfOpen {
		t.Fatalf("post-first-probe state=%s want half_open", got)
	}
}

// TestFakeClock_HalfOpenToClosed replaces TestBreaker_HalfOpenToClosed: after
// OpenDuration elapses, MaxRequests consecutive successful probes return the
// breaker to Closed.
func TestFakeClock_HalfOpenToClosed(t *testing.T) {
	opts := BreakerOptions{ // MaxRequests=3, OpenDuration=10ms
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, clock := newFakeBreaker(opts)
	failNTrips(b, errCov, 10)
	clock.add(opts.OpenDuration * 2)
	for i := 0; i < int(opts.MaxRequests); i++ {
		v, err := b.Execute(context.Background(), func(context.Context) (int, error) {
			return i + 1, nil
		})
		if err != nil {
			t.Fatalf("probe %d err=%v want nil", i, err)
		}
		if v != i+1 {
			t.Fatalf("probe %d value=%d want %d", i, v, i+1)
		}
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("state=%s want closed after %d successful probes", got, opts.MaxRequests)
	}
}

// TestFakeClock_HalfOpenToOpenOnFailure replaces TestBreaker_HalfOpenToOpenOnFailure:
// a single failed probe trips the breaker straight back to Open from HalfOpen.
func TestFakeClock_HalfOpenToOpenOnFailure(t *testing.T) {
	opts := BreakerOptions{ // OpenDuration=10ms
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, clock := newFakeBreaker(opts)
	failNTrips(b, errCov, 10)
	clock.add(opts.OpenDuration * 2)

	_, err := b.Execute(context.Background(), func(context.Context) (int, error) {
		return 0, errCov
	})
	if !errors.Is(err, errCov) {
		t.Fatalf("probe err=%v want sentinel", err)
	}
	if got := b.State(); got != StateOpen {
		t.Fatalf("state=%s want open after failed probe", got)
	}
}

// TestFakeClock_MetricsAccuracy replaces TestBreaker_MetricsAccuracy: walks a
// full Closed→Open→HalfOpen→Closed cycle and asserts the lifetime counters
// track it, with OpenDuration advanced via the fake clock.
func TestFakeClock_MetricsAccuracy(t *testing.T) {
	opts := BreakerOptions{ // MaxRequests=3, OpenDuration=10ms, MinRequests=4
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, clock := newFakeBreaker(opts)

	for range 4 {
		_, _ = b.Execute(context.Background(), func(context.Context) (int, error) { return 0, errCov })
	}
	if m := b.Metrics(); m.Total != 4 || m.Failures != 4 || m.Success != 0 || m.ConsecutiveFail != 4 {
		t.Fatalf("after trip metrics=%+v", m)
	}

	_, _ = b.Execute(context.Background(), func(context.Context) (int, error) { return 0, nil })
	if m := b.Metrics(); m.Total != 5 || m.Failures != 4 {
		t.Fatalf("after reject metrics=%+v want Total=5 Failures=4", m)
	}

	clock.add(opts.OpenDuration * 2)
	for i := 0; i < int(opts.MaxRequests); i++ {
		_, _ = b.Execute(context.Background(), func(context.Context) (int, error) { return i, nil })
	}
	m := b.Metrics()
	if m.Total != 8 || m.Success != 3 || m.Failures != 4 || m.ConsecutiveFail != 0 {
		t.Fatalf("after recovery metrics=%+v want Total=8 Success=3 Failures=4 Consec=0", m)
	}
	if m.State != StateClosed {
		t.Fatalf("final state=%s want closed", m.State)
	}
}

// TestFakeClock_RepeatedTripRecover replaces TestBreaker_RepeatedTripRecover: a
// breaker should tolerate multiple full cycles without leaking state, with each
// cooldown advanced via the fake clock.
func TestFakeClock_RepeatedTripRecover(t *testing.T) {
	opts := BreakerOptions{ // MaxRequests=3, OpenDuration=10ms
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, clock := newFakeBreaker(opts)
	for cycle := range 3 {
		failNTrips(b, errCov, 10)
		if got := b.State(); got != StateOpen {
			t.Fatalf("cycle %d: state=%s want open", cycle, got)
		}
		clock.add(opts.OpenDuration * 2)
		for i := 0; i < int(opts.MaxRequests); i++ {
			_, _ = b.Execute(context.Background(), func(context.Context) (int, error) { return i, nil })
		}
		if got := b.State(); got != StateClosed {
			t.Fatalf("cycle %d: state=%s want closed", cycle, got)
		}
	}
}

// TestFakeClock_HalfOpenRecoveryResetsWindow replaces
// TestBreaker_HalfOpenRecoveryResetsWindow: after HalfOpen→Closed the window is
// reset, so stale pre-trip failures don't immediately re-trip.
func TestFakeClock_HalfOpenRecoveryResetsWindow(t *testing.T) {
	opts := BreakerOptions{ // MinRequests=4, MaxRequests=3, OpenDuration=10ms
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, clock := newFakeBreaker(opts)
	failNTrips(b, errCov, 10)
	clock.add(opts.OpenDuration * 2)
	for i := 0; i < int(opts.MaxRequests); i++ {
		_, _ = b.Execute(context.Background(), func(context.Context) (int, error) { return i, nil })
	}
	_, err := b.Execute(context.Background(), func(context.Context) (int, error) {
		return 0, errCov
	})
	if errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("tripped immediately after recovery — window was not reset")
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("state=%s want closed", got)
	}
}

// --- Sliding-window expiry ----------------------------------------------------

// TestFakeClock_SlidingWindowExpires replaces TestBreaker_SlidingWindowExpires:
// failures outside the window must not count toward a trip. The 1s window is
// aged out via fake-clock advancement rather than a real 1.1s sleep.
func TestFakeClock_SlidingWindowExpires(t *testing.T) {
	opts := BreakerOptions{ // Interval=1s, MinRequests=4, FailRate=0.5
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, clock := newFakeBreaker(opts)

	_, _ = b.Execute(context.Background(), func(context.Context) (int, error) { return 0, errCov })
	clock.add(1100 * time.Millisecond) // > 1s window: the failure expires
	for i := range 2 {
		_, err := b.Execute(context.Background(), func(context.Context) (int, error) {
			return 0, errCov
		})
		if errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("tripped on call %d with expired first failure", i+1)
		}
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("state=%s want closed (old failure expired)", got)
	}
}

// TestFakeClock_WindowFullExpiryClearsStaleCounts replaces
// TestBreaker_WindowFullExpiryClearsStaleCounts: after the whole window elapses
// (advanced via the fake clock), a fresh burst starts from zero counts.
func TestFakeClock_WindowFullExpiryClearsStaleCounts(t *testing.T) {
	opts := BreakerOptions{ // Interval=1s, MinRequests=4
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, clock := newFakeBreaker(opts)
	for range 2 {
		_, _ = b.Execute(context.Background(), func(context.Context) (int, error) {
			return 0, errCov
		})
	}
	clock.add(1100 * time.Millisecond) // full window expiry
	for i := range 3 {
		_, err := b.Execute(context.Background(), func(context.Context) (int, error) {
			return 0, errCov
		})
		if errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("tripped on call %d: stale counts were not aged out", i+1)
		}
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("state=%s want closed after window aged out", got)
	}
}

// --- HalfOpen probe-slot saturation -----------------------------------------

// TestFakeClock_HalfOpenMaxRequests replaces TestBreaker_HalfOpenMaxRequests.
// It covers beforeCall's HalfOpen branch where halfOpenCount >= MaxRequests
// short-circuits before the Add(1) admission race: when all probe slots are
// taken, an extra call must be rejected with ErrCircuitOpen while the breaker
// stays HalfOpen.
//
// The OpenDuration expiry is advanced via the fake clock. The probe-slot
// saturation is set up directly through the internal halfOpenCount atomic
// (rather than dispatching goroutines that park inside fn), which makes the
// "all slots taken" precondition deterministic. The concurrent admission-race
// path itself is already exercised by TestBreaker_HalfOpen_Admission_RaceLoser
// in coverage_boost_test.go.
func TestFakeClock_HalfOpenMaxRequests(t *testing.T) {
	opts := BreakerOptions{ // MaxRequests=3, OpenDuration=10ms
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, clock := newFakeBreaker(opts)
	failNTrips(b, errCov, 10)
	if b.State() != StateOpen {
		t.Fatalf("precondition: state=%s want open", b.State())
	}
	clock.add(opts.OpenDuration * 2)

	// The first post-expiry call transitions Open -> HalfOpen and takes slot 1.
	if err := b.beforeCall(); err != nil {
		t.Fatalf("first probe beforeCall err=%v want nil", err)
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("state=%s want half_open after first probe admitted", b.State())
	}
	// Fill the remaining MaxRequests-1 probe slots without completing them, so
	// the breaker is HalfOpen with all slots taken.
	b.halfOpenCount.Store(int32(opts.MaxRequests))

	// Extra call: all slots taken, must be rejected without running fn.
	_, err := b.Execute(context.Background(), func(context.Context) (int, error) {
		t.Fatalf("fn must not run when all probe slots taken")
		return 1, nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("extra probe err=%v want ErrCircuitOpen", err)
	}
	if got := b.State(); got != StateHalfOpen {
		t.Fatalf("state=%s want half_open while slots saturated", got)
	}
}

// TestFakeClock_HalfOpenProbesRecover covers the positive HalfOpen path end to
// end: after expiry the probes are admitted and, on consecutive success, the
// breaker recovers to Closed. This complements the saturation test above by
// exercising the success-driven HalfOpen -> Closed transition through Execute
// (recordSuccess -> toClosed) under the fake clock, deterministically.
func TestFakeClock_HalfOpenProbesRecover(t *testing.T) {
	opts := BreakerOptions{ // MaxRequests=3, OpenDuration=10ms
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
	b, clock := newFakeBreaker(opts)
	failNTrips(b, errCov, 10)
	clock.add(opts.OpenDuration * 2)
	for i := 0; i < int(opts.MaxRequests); i++ {
		v, err := b.Execute(context.Background(), func(context.Context) (int, error) {
			return i + 1, nil
		})
		if err != nil {
			t.Fatalf("probe %d err=%v want nil", i, err)
		}
		if v != i+1 {
			t.Fatalf("probe %d value=%d want %d", i, v, i+1)
		}
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("post-probe state=%s want closed", got)
	}
}
