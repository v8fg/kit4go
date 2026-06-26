// This file is an internal benchmark/coverage test (package breaker, not
// breaker_test) so it can reach the unexported helpers — advance, withDefaults —
// and construct breakers that exercise branches the public tests don't hit
// (backward clock, HalfOpen overflow, the SetOnEvent hook). It also provides
// the hot-path benchmarks (Execute success/fail/parallel, State, Metrics) that
// quantify the overhead of the breaker's lock-free fast path.
package breaker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// benchOpts returns options sized for benchmarking: a 1s window keeps the ring
// tiny, MinRequests is high enough that the success path never trips, and
// OpenDuration is short so any failure-induced Open recovers promptly if a
// benchmark happens to fail.
func benchOpts() BreakerOptions {
	return BreakerOptions{
		Name:         "bench",
		MaxRequests:  5,
		Interval:     1 * time.Second,
		OpenDuration: 5 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  1 << 20, // huge: never trip on the success path
	}
}

// errBench is the sentinel returned by the always-fail benchmark fn.
var errBench = errors.New("bench failure")

// --- Benchmarks -------------------------------------------------------------

// BenchmarkBreaker_Execute_Success measures the closed-state success hot path:
// one Execute of an always-nil fn. This is the per-call overhead a healthy
// downstream pays (atomic total increment + recordSuccess + window record under
// mu).
func BenchmarkBreaker_Execute_Success(b *testing.B) {
	br := NewBreaker[int](benchOpts())
	ctx := context.Background()
	fn := func(context.Context) (int, error) { return 1, nil }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = br.Execute(ctx, fn)
	}
}

// BenchmarkBreaker_Execute_Fail measures the failure-recording path (Closed
// state, failure recorded in the window but no trip because MinRequests is
// huge). Quantifies the cost of the consecFail increment + window fail bucket.
func BenchmarkBreaker_Execute_Fail(b *testing.B) {
	br := NewBreaker[int](benchOpts())
	ctx := context.Background()
	fn := func(context.Context) (int, error) { return 0, errBench }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = br.Execute(ctx, fn)
	}
}

// BenchmarkBreaker_Execute_Parallel runs the success path from many goroutines
// to measure mu contention on the sliding-window record. Use -cpu to scale.
func BenchmarkBreaker_Execute_Parallel(b *testing.B) {
	br := NewBreaker[int](benchOpts())
	fn := func(context.Context) (int, error) { return 1, nil }
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()
		for pb.Next() {
			_, _ = br.Execute(ctx, fn)
		}
	})
}

// BenchmarkBreaker_State measures the lock-free atomic State() read. This is
// the cheapest possible observation and bounds the cost of a metrics scrape.
func BenchmarkBreaker_State(b *testing.B) {
	br := NewBreaker[int](benchOpts())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = br.State()
	}
}

// BenchmarkBreaker_Metrics measures the Metrics() snapshot: five atomic loads
// (state + four counters). Bounds the cost of a full metrics scrape.
func BenchmarkBreaker_Metrics(b *testing.B) {
	br := NewBreaker[int](benchOpts())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = br.Metrics()
	}
}

// --- Coverage boosters ------------------------------------------------------

// TestBreaker_HalfOpen_Rejects_Excess drives the breaker into HalfOpen, takes
// every probe slot, and confirms an extra caller is rejected with
// ErrCircuitOpen — exercising beforeCall's HalfOpen branch where
// halfOpenCount >= MaxRequests short-circuits before the Add(1) admission race.
// Unlike TestBreaker_HalfOpenMaxRequests (which blocks probes inside fn), this
// variant holds the slots by leaving the probes un-completed via a gate, then
// checks the rejection from the main goroutine.
func TestBreaker_HalfOpen_Rejects_Excess(t *testing.T) {
	opts := benchOpts()
	opts.MaxRequests = 2
	opts.MinRequests = 2
	opts.OpenDuration = 5 * time.Millisecond
	br := NewBreaker[int](opts)

	// Trip: two failures at FailRate 0.5 with MinRequests 2 -> rate 1.0 >= 0.5.
	failFn := func(context.Context) (int, error) { return 0, errBench }
	for i := 0; i < 2; i++ {
		_, _ = br.Execute(context.Background(), failFn)
	}
	if got := br.State(); got != StateOpen {
		t.Fatalf("precondition: state=%s want open", got)
	}
	time.Sleep(opts.OpenDuration * 3)

	// Take both probe slots and hold them inside fn so they don't complete.
	gate := make(chan struct{})
	started := make(chan struct{}, opts.MaxRequests)
	var wg sync.WaitGroup
	for i := 0; i < int(opts.MaxRequests); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = br.Execute(context.Background(), func(context.Context) (int, error) {
				started <- struct{}{}
				<-gate
				return 1, nil
			})
		}()
	}
	for i := 0; i < int(opts.MaxRequests); i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for probe %d to start", i)
		}
	}
	// Extra call: all slots taken, must be rejected without running fn.
	_, err := br.Execute(context.Background(), func(context.Context) (int, error) {
		t.Fatalf("fn must not run when all probe slots taken")
		return 1, nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("extra probe err=%v want ErrCircuitOpen", err)
	}
	if got := br.State(); got != StateHalfOpen {
		t.Fatalf("state=%s want half_open while probes held", got)
	}
	close(gate)
	wg.Wait()
}

// TestBreaker_Advance_BackwardClock drives the advance() ring roller with a
// timestamp older than base, exercising the clock-backward branch (it must
// clear only the target slot and not panic or corrupt the running sums).
func TestBreaker_Advance_BackwardClock(t *testing.T) {
	opts := benchOpts()
	opts.Interval = 5 * time.Second // 5 buckets
	br := NewBreaker[int](opts)

	br.mu.Lock()
	// Seed a known base and some bucket counts.
	br.base = 1000
	for i := range br.counts {
		br.counts[i] = 10
		br.fails[i] = 5
	}
	br.sumTotal = 50
	br.sumFail = 25
	br.mu.Unlock()

	// Now advance to a second BEFORE base: the backward branch must clear only
	// the target slot and lower the sums by exactly that slot's counts.
	br.mu.Lock()
	br.advance(997) // 997 < 1000 base
	idx := int(int64(997) % int64(len(br.counts)))
	wantTotal := 50 - 10
	wantFail := 25 - 5
	if br.counts[idx] != 0 || br.fails[idx] != 0 {
		t.Fatalf("backward clock did not clear target slot: counts[%d]=%d fails[%d]=%d",
			idx, br.counts[idx], idx, br.fails[idx])
	}
	if br.sumTotal != wantTotal || br.sumFail != wantFail {
		t.Fatalf("after backward advance sums=(%d,%d) want (%d,%d)",
			br.sumTotal, br.sumFail, wantTotal, wantFail)
	}
	if br.base != 997 {
		t.Fatalf("base=%d want 997 after backward advance", br.base)
	}
	br.mu.Unlock()

	// A same-second advance (sec == base) must be a no-op.
	br.mu.Lock()
	before := br.sumTotal
	br.advance(997)
	if br.sumTotal != before {
		t.Fatalf("same-second advance changed sum: %d -> %d", before, br.sumTotal)
	}
	br.mu.Unlock()
}

// TestBreaker_State_String_All re-asserts String() for every state plus the
// unknown sentinel. Mirrors the public test but lives here so the internal
// package covers the method directly.
func TestBreaker_State_String_All(t *testing.T) {
	cases := map[BreakerState]string{
		StateClosed:      "closed",
		StateOpen:        "open",
		StateHalfOpen:    "half_open",
		BreakerState(42): "unknown",
	}
	for st, want := range cases {
		if got := st.String(); got != want {
			t.Errorf("state %d String()=%q want %q", st, got, want)
		}
	}
}

// TestBreaker_Options_Defaults exercises withDefaults directly: a zero
// BreakerOptions must yield the documented defaults, and partial overrides must
// be preserved while zeros are filled. The public test only observes this
// indirectly via NewBreaker; here we assert the struct fields.
func TestBreaker_Options_Defaults(t *testing.T) {
	d := defaultBreakerOptions()

	got := BreakerOptions{}.withDefaults()
	if got.MaxRequests != d.MaxRequests {
		t.Errorf("default MaxRequests=%d want %d", got.MaxRequests, d.MaxRequests)
	}
	if got.Interval != d.Interval {
		t.Errorf("default Interval=%v want %v", got.Interval, d.Interval)
	}
	if got.OpenDuration != d.OpenDuration {
		t.Errorf("default OpenDuration=%v want %v", got.OpenDuration, d.OpenDuration)
	}
	if got.FailRate != d.FailRate {
		t.Errorf("default FailRate=%v want %v", got.FailRate, d.FailRate)
	}
	if got.MinRequests != d.MinRequests {
		t.Errorf("default MinRequests=%d want %d", got.MinRequests, d.MinRequests)
	}

	// Partial override: only MaxRequests set; everything else defaults.
	partial := BreakerOptions{MaxRequests: 7}.withDefaults()
	if partial.MaxRequests != 7 {
		t.Errorf("partial MaxRequests=%d want 7 (preserved)", partial.MaxRequests)
	}
	if partial.FailRate != d.FailRate {
		t.Errorf("partial FailRate=%v want default %v (filled)", partial.FailRate, d.FailRate)
	}

	// Clamping: sub-1 MaxRequests/MinRequests clamp to 1.
	clamped := BreakerOptions{MaxRequests: 0, MinRequests: 0}.withDefaults()
	// 0 is "unset" so they become the default (5/10), not 1. To hit the clamp
	// we must pass an explicit sub-1 value that is NOT the zero value — but
	// uint32 has no negative, so the only sub-1 value is 0 which is treated as
	// unset. The clamp branch is therefore only reachable via overflow/truncation
	// in practice; assert the documented behaviour that defaults are applied.
	if clamped.MaxRequests < 1 {
		t.Errorf("clamped MaxRequests=%d want >=1", clamped.MaxRequests)
	}
	if clamped.MinRequests < 1 {
		t.Errorf("clamped MinRequests=%d want >=1", clamped.MinRequests)
	}

	// Sub-second Interval rounds up to 1s.
	round := BreakerOptions{Interval: 500 * time.Millisecond}.withDefaults()
	if round.Interval != time.Second {
		t.Errorf("sub-second Interval=%v want 1s", round.Interval)
	}

	// Negative OpenDuration falls back to the default.
	neg := BreakerOptions{OpenDuration: -1 * time.Second}.withDefaults()
	if neg.OpenDuration != d.OpenDuration {
		t.Errorf("negative OpenDuration=%v want default %v", neg.OpenDuration, d.OpenDuration)
	}
}

// TestBreaker_Execute_NilCtx confirms Execute tolerates a nil ctx (the breaker
// does not dereference ctx before calling fn, so fn receives whatever it is
// given; a nil ctx must not panic inside Execute itself). fn itself must be
// nil-safe, so we pass one that ignores ctx.
func TestBreaker_Execute_NilCtx(t *testing.T) {
	br := NewBreaker[int](benchOpts())
	// fn ignores ctx entirely; Execute must not panic on nil ctx.
	v, err := br.Execute(nil, func(context.Context) (int, error) { return 42, nil })
	if err != nil {
		t.Fatalf("nil-ctx success err=%v want nil", err)
	}
	if v != 42 {
		t.Fatalf("nil-ctx value=%d want 42", v)
	}
	// And the failure path: fn returns an error, still no panic.
	_, err = br.Execute(nil, func(context.Context) (int, error) { return 0, errBench })
	if !errors.Is(err, errBench) {
		t.Fatalf("nil-ctx failure err=%v want sentinel", err)
	}
}

// --- SetOnEvent hook coverage ----------------------------------------------

// TestBreaker_SetOnEvent_AllOutcomes walks a full Closed->Open->HalfOpen->
// Closed cycle plus a rejection and confirms the hook fires every event name
// at least once with the correct post-transition state. Event ordering between
// a transition and its triggering outcome (e.g. "trip" vs the "failure" that
// caused it) is not contractually fixed, so we assert presence + state rather
// than a brittle exact sequence. Also verifies nil disables the hook.
func TestBreaker_SetOnEvent_AllOutcomes(t *testing.T) {
	opts := benchOpts()
	opts.MaxRequests = 2
	opts.MinRequests = 2
	// Long OpenDuration so the reject-while-Open step is deterministic (the
	// breaker won't spontaneously recover to HalfOpen mid-test).
	opts.OpenDuration = 1 * time.Hour
	br := NewBreaker[int](opts)

	var mu sync.Mutex
	var seen []BreakerEvent
	br.SetOnEvent(func(evt BreakerEvent) {
		mu.Lock()
		seen = append(seen, evt)
		mu.Unlock()
	})

	succFn := func(context.Context) (int, error) { return 1, nil }
	failFn := func(context.Context) (int, error) { return 0, errBench }

	// Two failures with NO preceding success: sumTotal reaches MinRequests=2 on
	// the second, and 2/2 = 1.0 >= FailRate 0.5 -> trip on fail2. This yields
	// 2x "failure" + 1x "trip". (A preceding success would push sumTotal to 2
	// on fail1 and trip one call earlier; we avoid that for determinism.)
	_, _ = br.Execute(context.Background(), failFn)
	_, _ = br.Execute(context.Background(), failFn)
	if got := br.State(); got != StateOpen {
		t.Fatalf("after 2 failures state=%s want open", got)
	}
	// A rejection while Open -> "reject".
	_, _ = br.Execute(context.Background(), succFn)

	// Force recovery by arming a past expiry so the next call transitions
	// Open -> HalfOpen (we can't wait out a 1-hour OpenDuration).
	br.expiry.Store(time.Now().Add(-1 * time.Second).UnixNano())
	// 2 successful probes -> 2x "success" + a "recover" (HalfOpen -> Closed).
	_, _ = br.Execute(context.Background(), succFn)
	_, _ = br.Execute(context.Background(), succFn)
	if got := br.State(); got != StateClosed {
		t.Fatalf("after probes state=%s want closed", got)
	}

	mu.Lock()
	// Tally events by name.
	counts := map[string]int{}
	for _, e := range seen {
		counts[e.Name]++
	}
	// Expected minimum counts.
	wantMin := map[string]int{
		"success": 2, // 2 probes
		"failure": 2, // two failures that tripped
		"trip":    1, // Closed -> Open
		"reject":  1, // blocked while Open
		"recover": 1, // HalfOpen -> Closed
	}
	for name, min := range wantMin {
		if counts[name] < min {
			t.Errorf("event %q fired %d times, want >= %d (full: %+v)", name, counts[name], min, seen)
		}
	}
	// Verify transition events carried the correct post-transition state.
	for _, e := range seen {
		switch e.Name {
		case "trip":
			if e.State != StateOpen {
				t.Errorf("trip event state=%s want open", e.State)
			}
		case "recover":
			if e.State != StateClosed {
				t.Errorf("recover event state=%s want closed", e.State)
			}
		case "reject":
			if e.State != StateOpen {
				t.Errorf("reject event state=%s want open (was rejected by an Open breaker)", e.State)
			}
		}
	}
	preDisable := len(seen)
	mu.Unlock()

	// Disable the hook and confirm no further events fire.
	br.SetOnEvent(nil)
	_, _ = br.Execute(context.Background(), succFn)
	_, _ = br.Execute(context.Background(), failFn)

	mu.Lock()
	if len(seen) != preDisable {
		t.Fatalf("events after SetOnEvent(nil): got %d, want %d (hook not disabled)", len(seen), preDisable)
	}
	mu.Unlock()
}

// TestBreaker_SetOnEvent_NilByDefault confirms that with no hook installed,
// Execute still works and fires nothing (the nil path must be zero-overhead
// and must not panic). We can't observe "nothing fired" directly, but we can
// confirm a fresh breaker's onEvent pointer is nil and Execute is unaffected.
func TestBreaker_SetOnEvent_NilByDefault(t *testing.T) {
	br := NewBreaker[int](benchOpts())
	if p := br.onEvent.Load(); p != nil {
		t.Fatalf("fresh breaker onEvent=%v want nil", p)
	}
	// Execute must work normally with no hook.
	v, err := br.Execute(context.Background(), func(context.Context) (int, error) { return 7, nil })
	if err != nil || v != 7 {
		t.Fatalf("no-hook Execute: v=%d err=%v want 7/nil", v, err)
	}
}

// TestBreaker_SetOnEvent_Concurrent verifies the hook can be installed while
// traffic is in flight without racing (the atomic pointer makes SetOnEvent
// safe concurrent with Execute). Run under -race.
func TestBreaker_SetOnEvent_Concurrent(t *testing.T) {
	br := NewBreaker[int](benchOpts())
	var count atomic.Uint64
	br.SetOnEvent(func(BreakerEvent) { count.Add(1) })

	const goroutines = 16
	var wg sync.WaitGroup
	// Half the goroutines drive traffic; the other half flip the hook on/off.
	wg.Add(goroutines * 2)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for j := 0; j < 200; j++ {
				_, _ = br.Execute(ctx, func(context.Context) (int, error) { return j, nil })
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				br.SetOnEvent(func(BreakerEvent) {})
			}
		}()
	}
	wg.Wait()
	// The only assertion is "no race / no panic". The event count is
	// non-deterministic because the second set of goroutines replaces the hook.
	if count.Load() == 0 {
		// count captured by the original closure may be > 0; just ensure we
		// didn't deadlock. A zero is acceptable if the replacers won every
		// race, so this is informational, not fatal.
		t.Logf("original hook fired %d times (replacers may have won all races)", count.Load())
	}
}
