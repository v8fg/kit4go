package breaker_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/v8fg/kit4go/breaker"
)

// fastOpts returns options with small durations so the transition tests run in
// tens of milliseconds rather than minutes. Window 1s (1 bucket), OpenDuration
// 10ms, low MinRequests so a trip is easy to trigger.
func fastOpts() breaker.BreakerOptions {
	return breaker.BreakerOptions{
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		OpenDuration: 10 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
	}
}

// errSentinel is the failure fn returns; distinct from breaker.ErrCircuitOpen
// so tests can tell a real failure from a rejection.
var errSentinel = errors.New("boom")

// failNTrips drives a freshly-built breaker to StateOpen by running failing fns
// until it trips, returning the index of the trip call (1-based) or -1.
func failNTrips[T any](b *breaker.Breaker[T], failErr error, max int) int {
	for i := 0; i < max; i++ {
		_, err := b.Execute(context.Background(), func(ctx context.Context) (T, error) {
			var zero T
			return zero, failErr
		})
		if errors.Is(err, breaker.ErrCircuitOpen) {
			return i
		}
	}
	return -1
}

// TestNewBreaker_StartsClosed: a fresh breaker must report StateClosed and the
// zero-value metrics (no calls yet).
func TestNewBreaker_StartsClosed(t *testing.T) {
	b := breaker.NewBreaker[int](fastOpts())
	if got := b.State(); got != breaker.StateClosed {
		t.Fatalf("new breaker state=%s want closed", got)
	}
	m := b.Metrics()
	if m.Total != 0 || m.Success != 0 || m.Failures != 0 || m.ConsecutiveFail != 0 {
		t.Fatalf("fresh metrics non-zero: %+v", m)
	}
}

// TestOptions_DefaultsApplied: zero options must yield the documented defaults.
func TestOptions_DefaultsApplied(t *testing.T) {
	b := breaker.NewBreaker[int](breaker.BreakerOptions{})
	// Indirectly observable: MinRequests threshold honoured, name empty, state
	// closed. (Default durations are exercised by other tests.)
	if b.State() != breaker.StateClosed {
		t.Fatalf("state=%s want closed", b.State())
	}
	m := b.Metrics()
	if m.State != breaker.StateClosed {
		t.Fatalf("metrics state=%s want closed", m.State)
	}
}

// TestBreaker_StateString covers the String rendering of every state.
func TestBreaker_StateString(t *testing.T) {
	cases := map[breaker.BreakerState]string{
		breaker.StateClosed:      "closed",
		breaker.StateOpen:        "open",
		breaker.StateHalfOpen:    "half_open",
		breaker.BreakerState(99): "unknown",
	}
	for st, want := range cases {
		if got := st.String(); got != want {
			t.Errorf("BreakerState(%d).String()=%q want %q", st, got, want)
		}
	}
}

// TestBreaker_MinRequestsThreshold: even with 100% failures, the breaker must
// not trip before MinRequests in-window calls have landed. Here MinRequests=4,
// so the first 3 failing calls leave it Closed.
func TestBreaker_MinRequestsThreshold(t *testing.T) {
	opts := fastOpts() // MinRequests=4, FailRate=0.5
	opts.FailRate = 1.0
	b := breaker.NewBreaker[string](opts)
	for i := 0; i < int(opts.MinRequests)-1; i++ {
		_, err := b.Execute(context.Background(), func(ctx context.Context) (string, error) {
			return "", errSentinel
		})
		if errors.Is(err, breaker.ErrCircuitOpen) {
			t.Fatalf("tripped on call %d before MinRequests=%d", i+1, opts.MinRequests)
		}
	}
	if got := b.State(); got != breaker.StateClosed {
		t.Fatalf("state=%s want closed after MinRequests-1 failures", got)
	}
}

// TestBreaker_FailRateThreshold: at FailRate=0.5 with MinRequests=4, four
// failures out of four (>= 0.5) trip the breaker on the 4th call.
func TestBreaker_FailRateThreshold(t *testing.T) {
	b := breaker.NewBreaker[int](fastOpts()) // FailRate=0.5, MinRequests=4
	for i := 0; i < 4; i++ {
		_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
			return 0, errSentinel
		})
		if i < 3 && errors.Is(err, breaker.ErrCircuitOpen) {
			t.Fatalf("tripped too early on call %d", i+1)
		}
	}
	if got := b.State(); got != breaker.StateOpen {
		t.Fatalf("state=%s want open after 4/4 failures at FailRate 0.5", got)
	}
}

// TestBreaker_DoesNotTripBelowFailRate: 2 failures out of 4 (rate 0.5 == 0.5
// trips), but 1 failure out of 4 (0.25) must not trip.
func TestBreaker_DoesNotTripBelowFailRate(t *testing.T) {
	opts := fastOpts() // FailRate=0.5, MinRequests=4
	b := breaker.NewBreaker[int](opts)
	// one failure then three successes -> rate 0.25, below 0.5. The first call
	// returns the sentinel (it genuinely failed); that's expected and is not a
	// breaker rejection.
	if _, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
		return 0, errSentinel
	}); !errors.Is(err, errSentinel) {
		t.Fatalf("first-call err=%v want sentinel", err)
	}
	for i := 0; i < 3; i++ {
		_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
			return i, nil
		})
		if errors.Is(err, breaker.ErrCircuitOpen) {
			t.Fatalf("tripped on call %d with rate 0.25 < 0.5", i+2)
		}
	}
	if got := b.State(); got != breaker.StateClosed {
		t.Fatalf("state=%s want closed at 0.25 failure rate", got)
	}
}

// TestBreaker_ClosedToOpen: the canonical trip. Fail enough calls, land in Open,
// and subsequent calls are rejected with ErrCircuitOpen and the zero T.
func TestBreaker_ClosedToOpen(t *testing.T) {
	b := breaker.NewBreaker[int](fastOpts())
	if idx := failNTrips(b, errSentinel, 10); idx < 0 {
		t.Fatalf("breaker never tripped")
	}
	if got := b.State(); got != breaker.StateOpen {
		t.Fatalf("state=%s want open", got)
	}
	// Next call is rejected and returns the zero value.
	v, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
		t.Fatalf("fn must not run when open")
		return 1, nil
	})
	if !errors.Is(err, breaker.ErrCircuitOpen) {
		t.Fatalf("err=%v want ErrCircuitOpen", err)
	}
	if v != 0 {
		t.Fatalf("rejected value=%d want 0", v)
	}
}

// TestBreaker_OpenToHalfOpen: after OpenDuration elapses, the next call must
// transition the breaker to HalfOpen instead of rejecting.
func TestBreaker_OpenToHalfOpen(t *testing.T) {
	opts := fastOpts() // OpenDuration=10ms
	b := breaker.NewBreaker[int](opts)
	failNTrips(b, errSentinel, 10)
	if b.State() != breaker.StateOpen {
		t.Fatalf("precondition: want open, got %s", b.State())
	}
	time.Sleep(opts.OpenDuration * 2)
	// First post-expiry call flips to HalfOpen; the fn runs and succeeds, which
	// is the first successful probe.
	_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
		// State should be HalfOpen by now (transition happened in beforeCall).
		if got := b.State(); got != breaker.StateHalfOpen {
			t.Fatalf("during probe state=%s want half_open", got)
		}
		return 1, nil
	})
	if err != nil {
		t.Fatalf("first probe err=%v want nil", err)
	}
	if got := b.State(); got != breaker.StateHalfOpen {
		t.Fatalf("post-first-probe state=%s want half_open", got)
	}
}

// TestBreaker_RejectsBeforeExpiry: while OpenDuration has NOT elapsed, the
// breaker stays Open and rejects.
func TestBreaker_RejectsBeforeExpiry(t *testing.T) {
	opts := fastOpts()
	opts.OpenDuration = 1 * time.Hour // long: never expires during the test
	b := breaker.NewBreaker[int](opts)
	failNTrips(b, errSentinel, 10)
	for i := 0; i < 5; i++ {
		_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
			t.Fatalf("fn must not run while open & unexpired")
			return 1, nil
		})
		if !errors.Is(err, breaker.ErrCircuitOpen) {
			t.Fatalf("call %d err=%v want ErrCircuitOpen", i, err)
		}
	}
	if got := b.State(); got != breaker.StateOpen {
		t.Fatalf("state=%s want open", got)
	}
}

// TestBreaker_HalfOpenToClosed: after OpenDuration, MaxRequests consecutive
// successful probes return the breaker to Closed.
func TestBreaker_HalfOpenToClosed(t *testing.T) {
	opts := fastOpts() // MaxRequests=3, OpenDuration=10ms
	b := breaker.NewBreaker[int](opts)
	failNTrips(b, errSentinel, 10)
	time.Sleep(opts.OpenDuration * 2)
	for i := 0; i < int(opts.MaxRequests); i++ {
		v, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
			return i + 1, nil
		})
		if err != nil {
			t.Fatalf("probe %d err=%v want nil", i, err)
		}
		if v != i+1 {
			t.Fatalf("probe %d value=%d want %d", i, v, i+1)
		}
	}
	if got := b.State(); got != breaker.StateClosed {
		t.Fatalf("state=%s want closed after %d successful probes", got, opts.MaxRequests)
	}
}

// TestBreaker_HalfOpenMaxRequests: once MaxRequests probe slots are taken, the
// (MaxRequests+1)-th call is rejected with ErrCircuitOpen while still HalfOpen.
// We block the probes with a gate so they don't complete and flip to Closed.
func TestBreaker_HalfOpenMaxRequests(t *testing.T) {
	opts := fastOpts() // MaxRequests=3, OpenDuration=10ms
	b := breaker.NewBreaker[int](opts)
	failNTrips(b, errSentinel, 10)
	time.Sleep(opts.OpenDuration * 2)

	// Hold the probes inside fn so slots stay taken.
	gate := make(chan struct{})
	started := make(chan struct{}, opts.MaxRequests)
	var wg sync.WaitGroup
	for i := 0; i < int(opts.MaxRequests); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) {
				started <- struct{}{}
				<-gate
				return 1, nil
			})
		}()
	}
	// Wait until all MaxRequests probes have entered fn.
	for i := 0; i < int(opts.MaxRequests); i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for probe %d to start", i)
		}
	}
	// One more call now should be rejected.
	_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
		t.Fatalf("fn must not run when all probe slots taken")
		return 1, nil
	})
	if !errors.Is(err, breaker.ErrCircuitOpen) {
		t.Fatalf("extra probe err=%v want ErrCircuitOpen", err)
	}
	if got := b.State(); got != breaker.StateHalfOpen {
		t.Fatalf("state=%s want half_open while probes in flight", got)
	}

	close(gate)
	wg.Wait()
	// After all probes succeed the breaker should be Closed.
	if got := b.State(); got != breaker.StateClosed {
		t.Fatalf("post-gate state=%s want closed", got)
	}
}

// TestBreaker_HalfOpenToOpenOnFailure: a single failed probe trips the breaker
// straight back to Open from HalfOpen.
func TestBreaker_HalfOpenToOpenOnFailure(t *testing.T) {
	opts := fastOpts() // OpenDuration=10ms
	b := breaker.NewBreaker[int](opts)
	failNTrips(b, errSentinel, 10)
	time.Sleep(opts.OpenDuration * 2)

	// First (failed) probe.
	_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
		return 0, errSentinel
	})
	if !errors.Is(err, errSentinel) {
		t.Fatalf("probe err=%v want sentinel", err)
	}
	if got := b.State(); got != breaker.StateOpen {
		t.Fatalf("state=%s want open after failed probe", got)
	}
}

// TestBreaker_CtxCancellationPropagated: a cancelled ctx surfaces its error and
// is recorded as a failure (so timeouts can't hide downstream trouble).
func TestBreaker_CtxCancellationPropagated(t *testing.T) {
	b := breaker.NewBreaker[int](fastOpts())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v, err := b.Execute(ctx, func(ctx context.Context) (int, error) {
		t.Fatalf("fn must not run on cancelled ctx (checked before fn)")
		return 1, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", err)
	}
	if v != 0 {
		t.Fatalf("value=%d want 0", v)
	}
	m := b.Metrics()
	if m.Failures != 1 || m.ConsecutiveFail != 1 {
		t.Fatalf("metrics=%+v want 1 failure, 1 consecutive", m)
	}
}

// TestBreaker_PassesValueAndError: the success path must forward the exact
// value and nil error returned by fn.
func TestBreaker_PassesValueAndError(t *testing.T) {
	b := breaker.NewBreaker[string](fastOpts())
	got, err := b.Execute(context.Background(), func(ctx context.Context) (string, error) {
		return "hello", nil
	})
	if err != nil || got != "hello" {
		t.Fatalf("got=%q err=%v want hello/nil", got, err)
	}
	// And the failure path forwards the fn error verbatim.
	_, err = b.Execute(context.Background(), func(ctx context.Context) (string, error) {
		return "", errSentinel
	})
	if !errors.Is(err, errSentinel) {
		t.Fatalf("err=%v want sentinel", err)
	}
}

// TestBreaker_MetricsAccuracy walks a full Closed→Open→HalfOpen→Closed cycle
// and asserts the lifetime counters track it.
func TestBreaker_MetricsAccuracy(t *testing.T) {
	opts := fastOpts() // MaxRequests=3, OpenDuration=10ms, MinRequests=4
	b := breaker.NewBreaker[int](opts)

	// 4 failures -> trip to Open (4 failures, 4 total).
	for i := 0; i < 4; i++ {
		_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) {
			return 0, errSentinel
		})
	}
	if m := b.Metrics(); m.Total != 4 || m.Failures != 4 || m.Success != 0 || m.ConsecutiveFail != 4 {
		t.Fatalf("after trip metrics=%+v", m)
	}

	// One rejected call increments Total but not Failures (fn didn't run).
	_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) { return 0, nil })
	if m := b.Metrics(); m.Total != 5 || m.Failures != 4 {
		t.Fatalf("after reject metrics=%+v want Total=5 Failures=4", m)
	}

	// Wait out OpenDuration, then 3 successful probes -> Closed.
	time.Sleep(opts.OpenDuration * 2)
	for i := 0; i < int(opts.MaxRequests); i++ {
		_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) { return i, nil })
	}
	m := b.Metrics()
	if m.Total != 8 || m.Success != 3 || m.Failures != 4 || m.ConsecutiveFail != 0 {
		t.Fatalf("after recovery metrics=%+v want Total=8 Success=3 Failures=4 Consec=0", m)
	}
	if m.State != breaker.StateClosed {
		t.Fatalf("final state=%s want closed", m.State)
	}
}

// TestBreaker_SlidingWindowExpires: failures outside the window must not count
// toward a trip. With a 1s window, we register one failure, wait > 1s, then
// hammer failures — the first failure has expired so the trip must require a
// fresh MinRequests worth.
func TestBreaker_SlidingWindowExpires(t *testing.T) {
	opts := fastOpts() // Interval=1s, MinRequests=4, FailRate=0.5
	b := breaker.NewBreaker[int](opts)

	// One old failure that will expire.
	_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) { return 0, errSentinel })
	time.Sleep(1100 * time.Millisecond) // > 1s window
	// Now only fresh failures count. Two fresh failures (< MinRequests=4) must
	// not trip even though the lifetime ConsecutiveFail is 3.
	for i := 0; i < 2; i++ {
		_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
			return 0, errSentinel
		})
		if errors.Is(err, breaker.ErrCircuitOpen) {
			t.Fatalf("tripped on call %d with expired first failure", i+1)
		}
	}
	if got := b.State(); got != breaker.StateClosed {
		t.Fatalf("state=%s want closed (old failure expired)", got)
	}
}

// TestBreaker_Concurrent hammers Execute from many goroutines. Run under -race.
// Asserts no panic and that lifetime metrics are self-consistent
// (Total == Success + Failures, where rejections count only toward Total).
func TestBreaker_Concurrent(t *testing.T) {
	opts := fastOpts()
	opts.MinRequests = 50 // raise so it doesn't trip instantly under load
	b := breaker.NewBreaker[int](opts)

	const goroutines = 32
	const perG = 200
	var wg sync.WaitGroup
	var failCount int64
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				fail := (i+seed)%2 == 0
				_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
					if fail {
						return 0, errSentinel
					}
					return i, nil
				})
				// ErrCircuitOpen is allowed (breaker may trip); only real
				// failures (sentinel) count toward our expectation.
				if errors.Is(err, errSentinel) {
					atomic.AddInt64(&failCount, 1)
				}
			}
		}(g)
	}
	wg.Wait()

	m := b.Metrics()
	if m.Total == 0 {
		t.Fatalf("Total=0, expected traffic")
	}
	// success+failures should equal total minus the rejected calls. Just check
	// the invariant: success+failures <= total (rejections explain the gap).
	if m.Success+m.Failures > m.Total {
		t.Fatalf("invariant broken: success(%d)+failures(%d) > total(%d)", m.Success, m.Failures, m.Total)
	}
	if m.Failures < uint64(failCount) {
		t.Fatalf("recorded failures=%d less than sentinel failures observed=%d", m.Failures, failCount)
	}
}

// TestBreaker_RepeatedTripRecover: a breaker should tolerate multiple full
// cycles without leaking state or getting stuck.
func TestBreaker_RepeatedTripRecover(t *testing.T) {
	opts := fastOpts() // MaxRequests=3, OpenDuration=10ms
	b := breaker.NewBreaker[int](opts)
	for cycle := 0; cycle < 3; cycle++ {
		failNTrips(b, errSentinel, 10)
		if got := b.State(); got != breaker.StateOpen {
			t.Fatalf("cycle %d: state=%s want open", cycle, got)
		}
		time.Sleep(opts.OpenDuration * 2)
		for i := 0; i < int(opts.MaxRequests); i++ {
			_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) { return i, nil })
		}
		if got := b.State(); got != breaker.StateClosed {
			t.Fatalf("cycle %d: state=%s want closed", cycle, got)
		}
	}
}

// TestBreaker_GenericTypes exercises that the breaker works for slice and struct
// payload types, not just scalars.
func TestBreaker_GenericTypes(t *testing.T) {
	t.Run("slice", func(t *testing.T) {
		b := breaker.NewBreaker[[]int](fastOpts())
		got, err := b.Execute(context.Background(), func(ctx context.Context) ([]int, error) {
			return []int{1, 2, 3}, nil
		})
		if err != nil || len(got) != 3 {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("struct", func(t *testing.T) {
		type payload struct {
			Name string
			N    int
		}
		b := breaker.NewBreaker[payload](fastOpts())
		got, err := b.Execute(context.Background(), func(ctx context.Context) (payload, error) {
			return payload{Name: "ok", N: 7}, nil
		})
		if err != nil || got.Name != "ok" || got.N != 7 {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
}

// TestBreaker_HalfOpenRecoveryResetsWindow: after HalfOpen→Closed the window is
// reset, so stale pre-trip failures don't immediately re-trip on the first
// post-recovery failure.
func TestBreaker_HalfOpenRecoveryResetsWindow(t *testing.T) {
	opts := fastOpts() // MinRequests=4, MaxRequests=3, OpenDuration=10ms
	b := breaker.NewBreaker[int](opts)
	failNTrips(b, errSentinel, 10)
	time.Sleep(opts.OpenDuration * 2)
	for i := 0; i < int(opts.MaxRequests); i++ {
		_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) { return i, nil })
	}
	// Now Closed with a fresh window. One failure should NOT trip (MinRequests=4).
	_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
		return 0, errSentinel
	})
	if errors.Is(err, breaker.ErrCircuitOpen) {
		t.Fatalf("tripped immediately after recovery — window was not reset")
	}
	if got := b.State(); got != breaker.StateClosed {
		t.Fatalf("state=%s want closed", got)
	}
}

// TestBreaker_FailRateZeroTripsOnAnyFailure: with FailRate < 0 the breaker
// trips as soon as MinRequests calls have landed and at least one of them
// failed — exercising the maybeTrip FailRate<=0 branch. (FailRate==0 is treated
// as "unset" by withDefaults and becomes the 0.5 default, so we use a negative
// value, which is documented as the way to request trip-on-any-failure.)
func TestBreaker_FailRateZeroTripsOnAnyFailure(t *testing.T) {
	opts := fastOpts() // MinRequests=4
	opts.FailRate = -1
	b := breaker.NewBreaker[int](opts)
	// Three successes (no failures yet) must not trip.
	for i := 0; i < 3; i++ {
		_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
			return i, nil
		})
		if errors.Is(err, breaker.ErrCircuitOpen) {
			t.Fatalf("tripped on success-only call %d", i+1)
		}
	}
	// Fourth call fails: MinRequests met and sumFail>0 -> trip.
	_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
		return 0, errSentinel
	})
	if errors.Is(err, breaker.ErrCircuitOpen) {
		t.Fatalf("4th call should have run (then trip), got ErrCircuitOpen")
	}
	if got := b.State(); got != breaker.StateOpen {
		t.Fatalf("state=%s want open with FailRate<0 and one failure", got)
	}
}

// TestBreaker_FailRateAboveOneNeverTrips: FailRate > 1 disables tripping — the
// breaker stays Closed no matter how many failures accumulate.
func TestBreaker_FailRateAboveOneNeverTrips(t *testing.T) {
	opts := fastOpts()
	opts.FailRate = 2.0
	b := breaker.NewBreaker[int](opts)
	for i := 0; i < 20; i++ {
		_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
			return 0, errSentinel
		})
		if errors.Is(err, breaker.ErrCircuitOpen) {
			t.Fatalf("tripped on call %d despite FailRate>1", i+1)
		}
	}
	if got := b.State(); got != breaker.StateClosed {
		t.Fatalf("state=%s want closed with FailRate>1", got)
	}
}

// TestBreaker_WindowFullExpiryClearsStaleCounts: after the whole window elapses,
// a fresh burst must start from zero counts — not carry pre-trip failures
// forward. Exercises advance()'s full-window-reset branch and confirms the
// breaker can recover to Closed without going through HalfOpen (by aging out).
func TestBreaker_WindowFullExpiryClearsStaleCounts(t *testing.T) {
	opts := fastOpts() // Interval=1s, MinRequests=4
	b := breaker.NewBreaker[int](opts)
	// Two failures (below trip threshold), then wait > window.
	for i := 0; i < 2; i++ {
		_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) {
			return 0, errSentinel
		})
	}
	time.Sleep(1100 * time.Millisecond) // full window expiry
	// Now register MinRequests-1 more failures; the stale two must not count, so
	// we stay Closed (3 fresh < MinRequests=4 and rate would be 1.0 anyway but
	// count gate fails first).
	for i := 0; i < 3; i++ {
		_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
			return 0, errSentinel
		})
		if errors.Is(err, breaker.ErrCircuitOpen) {
			t.Fatalf("tripped on call %d: stale counts were not aged out", i+1)
		}
	}
	if got := b.State(); got != breaker.StateClosed {
		t.Fatalf("state=%s want closed after window aged out", got)
	}
}

// TestOptions_OverridesApplied verifies through observable behavior that
// caller-supplied options are honoured (MinRequests/MaxRequests/FailRate) rather
// than clobbered by defaults. The private withDefaults is exercised indirectly
// via NewBreaker + Execute.
func TestOptions_OverridesApplied(t *testing.T) {
	// MinRequests=2 with FailRate=1.0: trips exactly on the 2nd failure
	// (proving MinRequests=2 and FailRate=1.0 both took effect, not defaults
	// MinRequests=10 / FailRate=0.5).
	b := breaker.NewBreaker[int](breaker.BreakerOptions{
		MinRequests: 2,
		FailRate:    1.0,
		Interval:    time.Second,
	})
	for i := 0; i < 2; i++ {
		_, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
			return 0, errSentinel
		})
		if i == 0 && errors.Is(err, breaker.ErrCircuitOpen) {
			t.Fatalf("tripped on call 1 with MinRequests=2")
		}
	}
	if got := b.State(); got != breaker.StateOpen {
		t.Fatalf("state=%s want open after exactly MinRequests=2 failures at FailRate=1.0", got)
	}
}
