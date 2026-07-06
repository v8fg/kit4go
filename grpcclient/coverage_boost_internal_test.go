// This file is the internal package test suite (package grpcclient, not the
// external _test suite) so it can exercise the unexported helpers that the
// 80.6% coverage gap is concentrated in: observe (0%), withTimeout nil-ctx and
// has-deadline branches, retryDelay's zero/overflow/cap arms, codeNameOf's
// non-status error path, and SetOnEvent's nil-disable path. These helpers are
// pure (no gRPC transport), so each case is a direct call with table-driven
// inputs — no bufconn, no goroutines, no flakiness. The end-to-end Latency
// observer coverage lives in coverage_boost_test.go (external suite) because it
// needs the bufconn echo fixture.
package grpcclient

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- fake LatencyObserver (internal; mirrors httpclient/latency_test.go) ---

// fakeObserver records every Observe call. It is the in-package analogue of
// httpclient_test.fakeObserver, kept here so observe() can be driven both
// directly (TestObserve) and via the interceptors (the external test wires it
// through a real bufconn call).
type fakeObserver struct {
	calls   int
	last    time.Duration
	nonzero bool
}

func (f *fakeObserver) Observe(d time.Duration) {
	f.calls++
	f.last = d
	if d > 0 {
		f.nonzero = true
	}
}

// TestObserve drives Middleware.observe directly: the Latency hook must forward
// time.Since(start) to the configured observer. This is the 0%-coverage helper
// added with the latency collection hook; previously no test set
// ClientOptions.Latency, so the defer m.observe(start) branch in both
// interceptors was also cold.
func TestObserve(t *testing.T) {
	obs := &fakeObserver{}
	mw := NewMiddleware(ClientOptions{Latency: obs})
	start := time.Now()
	// Sleep a measurable amount so nonzero is reliable even on a noisy CI box.
	time.Sleep(2 * time.Millisecond)
	mw.observe(start)

	if obs.calls != 1 {
		t.Fatalf("observe calls = %d, want 1", obs.calls)
	}
	if !obs.nonzero {
		t.Fatalf("observed duration = %v, want > 0", obs.last)
	}
}

// TestSetOnEventNil covers the fn==nil branch of (*Client).SetOnEvent, which
// stores a nil pointer to disable a previously-installed hook. The external
// suite only ever installs a non-nil hook, so the disable arm is otherwise
// cold. Assert the stored pointer is nil by firing an event afterwards and
// observing no dispatch.
func TestSetOnEventNil(t *testing.T) {
	c := &Client{opts: ClientOptions{}}
	fired := false
	c.SetOnEvent(func(ClientEvent) { fired = true })
	c.fireEvent("request", "/m", "OK", 0)
	if !fired {
		t.Fatal("hook should fire when set")
	}
	// Disable and confirm the dispatch path collapses to the nil compare.
	c.SetOnEvent(nil)
	fired = false
	c.fireEvent("request", "/m", "OK", 0)
	if fired {
		t.Fatal("hook must not fire after SetOnEvent(nil)")
	}
	if p := c.onEvent.Load(); p != nil {
		t.Fatalf("onEvent = %v, want nil after disable", p)
	}
}

// TestWithTimeout covers all three arms of the helper:
//   - nil ctx is replaced by context.Background before the deadline check;
//   - a ctx lacking a deadline + positive RequestTimeout → WithTimeout child;
//   - a ctx that already has a deadline → WithCancel (RequestTimeout yields).
func TestWithTimeout(t *testing.T) {
	t.Run("nil_ctx_gets_timeout", func(t *testing.T) {
		mw := NewMiddleware(ClientOptions{RequestTimeout: 50 * time.Millisecond})
		ctx, cancel := mw.withTimeout(nil)
		defer cancel()
		if ctx == nil {
			t.Fatal("ctx = nil, want non-nil")
		}
		dl, ok := ctx.Deadline()
		if !ok {
			t.Fatal("nil ctx should gain a deadline from RequestTimeout")
		}
		if got := time.Until(dl); got > 50*time.Millisecond || got < 0 {
			t.Fatalf("deadline delta = %v, want within (0, 50ms]", got)
		}
	})

	t.Run("ctx_with_deadline_keeps_caller_deadline", func(t *testing.T) {
		// Caller-supplied deadline always wins: the WithTimeout arm must NOT
		// run, so withTimeout returns WithCancel and the original deadline is
		// the one observed.
		parent, parentCancel := context.WithDeadline(context.Background(), time.Unix(1<<60, 0))
		defer parentCancel()

		mw := NewMiddleware(ClientOptions{RequestTimeout: 50 * time.Millisecond})
		ctx, cancel := mw.withTimeout(parent)
		defer cancel()
		dl, ok := ctx.Deadline()
		if !ok {
			t.Fatal("caller deadline should be preserved (WithCancel keeps parent deadline)")
		}
		parentDL, _ := parent.Deadline()
		if !dl.Equal(parentDL) {
			t.Fatalf("deadline = %v, want parent's %v (caller deadline wins)", dl, parentDL)
		}
	})

	t.Run("ctx_without_deadline_and_zero_timeout_returns_cancel", func(t *testing.T) {
		// RequestTimeout == 0 → withDefaults leaves it 0 only if forced; here
		// we bypass defaults by setting it explicitly and using a raw Client
		// with opts already defaulted to a positive value. Instead, drive the
		// WithCancel arm via a ctx that has no deadline and a Middleware whose
		// RequestTimeout was zeroed post-default.
		mw := &Middleware{opts: ClientOptions{RequestTimeout: 0}}
		ctx, cancel := mw.withTimeout(context.Background())
		defer cancel()
		if _, hasDL := ctx.Deadline(); hasDL {
			t.Fatal("zero RequestTimeout must not attach a deadline (WithCancel arm)")
		}
	})
}

// TestRetryDelay exercises every arm of retryDelay:
//   - minWait<=0 or maxWait<=0 → 0 (immediate retry, backoff disabled);
//   - normal exponential growth capped at maxWait;
//   - overflow clamp (minWait<<attempt wraps) clamps to maxWait.
//
// All cases also assert the jitter band [0.5*base, base).
func TestRetryDelay(t *testing.T) {
	t.Run("zero_waits_disable_backoff", func(t *testing.T) {
		cases := []struct{ minW, maxW time.Duration }{
			{0, time.Second},
			{time.Millisecond, 0},
			{-1, -1},
		}
		for _, c := range cases {
			if got := retryDelay(3, c.minW, c.maxW); got != 0 {
				t.Fatalf("retryDelay(min=%v max=%v) = %v, want 0", c.minW, c.maxW, got)
			}
		}
	})

	t.Run("exponential_capped_at_max", func(t *testing.T) {
		minW, maxW := 10*time.Millisecond, 100*time.Millisecond
		// attempt large enough that minW*2^attempt would blow past maxW; the
		// result must land in [0.5*maxW, maxW) after jitter.
		for i := 0; i < 50; i++ {
			got := retryDelay(20, minW, maxW)
			if got < 50*time.Millisecond || got >= 100*time.Millisecond {
				t.Fatalf("retryDelay(capped) = %v, want within [50ms, 100ms)", got)
			}
		}
	})

	t.Run("overflow_clamps_to_max", func(t *testing.T) {
		// Construct inputs so the left-shift inside the backoff loop actually
		// overflows (wraps to a non-positive value), exercising the
		// `if next <= backoff { backoff = maxWait; break }` guard. For the loop
		// body to run, backoff must START below maxWait; for the shift to wrap,
		// backoff must be near math.MaxInt64. So set both min and max to huge
		// positive values with min < max, and min large enough that min<<1
		// overflows a signed int64.
		minW := time.Duration(int64(1) << 62) // 2^62, positive; <<1 sets the sign bit (wraps)
		maxW := time.Duration(math.MaxInt64)  // huge, > minW so the loop body runs
		got := retryDelay(2, minW, maxW)
		// After the overflow guard fires, backoff is clamped to maxW; jitter
		// then yields a value in [0.5*maxW, maxW). Assert the result is huge
		// and positive (proving the clamp to maxWait, not a wrapped-negative).
		if got <= 0 {
			t.Fatalf("retryDelay(overflow) = %v, want > 0 (clamp must not leak a negative duration)", got)
		}
		if got < time.Duration(int64(1)<<62) {
			t.Fatalf("retryDelay(overflow) = %v, want >= 2^62 (clamped to maxWait then jittered)", got)
		}
	})

	t.Run("first_retry_within_jitter_band", func(t *testing.T) {
		// attempt=0 → backoff = minW (no doubling yet); jitter lands in
		// [0.5*minW, minW).
		minW, maxW := 20*time.Millisecond, time.Second
		for i := 0; i < 50; i++ {
			got := retryDelay(0, minW, maxW)
			if got < 10*time.Millisecond || got >= 20*time.Millisecond {
				t.Fatalf("retryDelay(0) = %v, want within [10ms, 20ms)", got)
			}
		}
	})
}

// TestCodeNameOf covers the three return arms:
//   - nil error → "";
//   - a plain (non-gRPC-status) error → codes.Unknown.String() ("Unknown");
//   - a real gRPC status → its code name (e.g. "Unavailable").
func TestCodeNameOf(t *testing.T) {
	if got := codeNameOf(nil); got != "" {
		t.Fatalf("codeNameOf(nil) = %q, want %q", got, "")
	}
	// Non-status error: status.FromError returns (codes.Unknown, false). This
	// covers the !ok branch that the existing tests never reach (every server
	// error in the bufconn suite is a proper gRPC status).
	if got := codeNameOf(errors.New("boom")); got != codes.Unknown.String() {
		t.Fatalf("codeNameOf(plain err) = %q, want %q", got, codes.Unknown.String())
	}
	injected := status.Error(codes.Unavailable, "down")
	if got := codeNameOf(injected); got != codes.Unavailable.String() {
		t.Fatalf("codeNameOf(unavailable) = %q, want %q", got, codes.Unavailable.String())
	}
}

// TestRetryableFalseCoversLoopExit drives retryable's "no match" return false
// arm directly (the interceptors reach it too, but this makes the unit-level
// assertion explicit and pins it against future refactors).
func TestRetryableFalseCoversLoopExit(t *testing.T) {
	o := ClientOptions{RetryCodes: []codes.Code{codes.Unavailable}}
	if o.retryable(codes.NotFound) {
		t.Fatal("retryable(NotFound) = true, want false")
	}
	if !o.retryable(codes.Unavailable) {
		t.Fatal("retryable(Unavailable) = false, want true")
	}
}

// TestUnaryInterceptorNegativeRetryMaxFallback covers the defensive `return err`
// at the end of UnaryClientInterceptor's retry loop (interceptors.go line 105).
//
// Why it is cold under the public API: NewMiddleware runs ClientOptions through
// withDefaults, which clamps RetryMax<=0 to the default (2). With RetryMax>=0
// the `for attempt := 0; attempt <= m.opts.RetryMax` body always runs at least
// once (attempt 0) and every iteration returns at one of the four exit points
// (success / ctx-err / non-retryable / attempts-exhausted). The post-loop
// `return err` can therefore only fire when RetryMax<0, i.e. a caller bypassed
// NewMiddleware and hand-built a Middleware with a negative RetryMax.
//
// Behaviour on that misconfigured path: the loop's `var err error` is never
// assigned (the body never runs), so line 105 returns the zero value — nil.
// The outer accounting block then sees a nil err and tallies the call as a
// success. This is a known quirk of the defensive guard (a negative RetryMax
// silently reads as success), which is why withDefaults clamps RetryMax before
// the public API ever sees it; the branch exists solely so the compiler keeps
// `err` in scope at the function tail rather than erroring on an unused
// variable / falling through. We exercise it here to pin the behaviour and
// keep the line covered against future refactors. No production code is
// changed.
func TestUnaryInterceptorNegativeRetryMaxFallback(t *testing.T) {
	calls := 0
	invoker := func(
		ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
		opts ...grpc.CallOption,
	) error {
		calls++
		return status.Error(codes.Unavailable, "injected")
	}

	// Hand-built Middleware bypasses withDefaults so RetryMax stays negative.
	// The retry loop's condition `attempt <= -1` is false from the start, so the
	// body never runs and control reaches the post-loop `return err`, which
	// surfaces the zero-value (nil) err.
	mw := &Middleware{
		opts:    ClientOptions{RetryMax: -1, RetryCodes: []codes.Code{codes.Unavailable}},
		metrics: &Client{opts: ClientOptions{RetryMax: -1, RetryCodes: []codes.Code{codes.Unavailable}}},
	}

	got := mw.UnaryClientInterceptor()(
		context.Background(), "/echo.Echo/Echo", nil, nil, nil, invoker,
	)
	if got != nil {
		t.Fatalf("surfaced err = %v, want nil (post-loop return of the unassigned err)", got)
	}
	if calls != 0 {
		t.Fatalf("invoker calls = %d, want 0 (loop body never ran under RetryMax<0)", calls)
	}
	// total is bumped at the top of the interceptor. Because the defensive
	// return surfaces a nil err, the outer block tallies the call as success and
	// fires the "success" event. Retried stays 0 (the loop never ran). This pins
	// the exact — and admittedly surprising — accounting of the defensive path,
	// which is precisely why withDefaults forbids a negative RetryMax at the
	// public API.
	m := mw.Metrics()
	if m.Total != 1 || m.Success != 1 || m.Failed != 0 || m.Retried != 0 {
		t.Fatalf("metrics = %+v, want total=1 success=1 failed=0 retried=0", m)
	}
}
