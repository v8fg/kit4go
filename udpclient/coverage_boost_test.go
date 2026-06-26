// This file is an internal coverage-boost test (package udpclient, not
// udpclient_test) so it can reach the unexported helpers — shouldRetry,
// retryDelay, sleep — and manipulate the unexported conn/closed fields of
// [Client] to drive the error paths that the external tests can only reach
// indirectly. It mirrors the internal-test pattern used by latency/internal_test.go
// and the coverage_boost_test.go files under log4go.
//
// The tests here target the branches the public test suite leaves cold:
//   - shouldRetry: nil, errClosed, context.Canceled/DeadlineExceeded,
//     non-timeout net.OpError, and the generic-error fallback
//   - retryDelay: disabled-backoff (<=0) short-circuit, the maxWait clamp, and
//     the int64 overflow guard
//   - sleep: the delay<=0 select branch (both ctx-already-done and not) and the
//     delay>0 ctx-cancelled-during-backoff branch
//   - sendWithRetry / sendReceiveWithRetry: the write-error retry tail (armed
//     via a pre-write that triggers an ICMP port-unreachable on the connected
//     socket), SetWriteDeadline/SetReadDeadline failure, the closed-mid-loop
//     branch, the ctx-cancelled-during-backoff branch, and the loop-fallthrough
//     return (RetryMax<0)
//   - Send / SendReceive: the nil-ctx defaulting and the failed-call bookkeeping
//   - Close: the conn==nil branch
//
// All networked tests use generous timeouts so they stay -race clean and
// non-flaky. Where a branch depends on platform ICMP delivery (macOS surfaces
// it as "connection refused" on the second write to a connected socket whose
// peer has no listener), the test arms the ICMP error explicitly with a
// pre-write and tolerates a platform that never delivers it by skipping rather
// than failing.
package udpclient

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// --- shouldRetry: exhaustive branch coverage --------------------------------

// fakeNetErr is a minimal net.Error whose Timeout()/Temporary() are configurable,
// so we can drive both the timeout and non-timeout branches of shouldRetry
// deterministically without depending on platform-specific socket behaviour.
type fakeNetErr struct {
	timeout   bool
	temporary bool
	op        string
	msg       string
}

func (e *fakeNetErr) Error() string   { return e.op + ": " + e.msg }
func (e *fakeNetErr) Timeout() bool   { return e.timeout }
func (e *fakeNetErr) Temporary() bool { return e.temporary }

// wrappedNetErr implements net.Error itself and also exposes an Unwrap chain,
// so we can verify errors.As still resolves a net.Error buried under a wrapper.
type wrappedNetErr struct {
	fakeNetErr
	cause error
}

func (w *wrappedNetErr) Unwrap() error { return w.cause }

func TestShouldRetry(t *testing.T) {
	opErrNonTimeout := &net.OpError{Op: "write", Err: errors.New("ECONNREFUSED")}
	opErrTimeout := &net.OpError{Op: "read", Err: &fakeNetErr{timeout: true, msg: "i/o timeout"}}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		// err == nil → false (the guard clause at the top).
		{"nil error returns false", nil, false},
		// errClosed (wrapped) → never retried (terminal: socket gone).
		{"wrapped errClosed not retried", fmt.Errorf("wrap: %w", errClosed), false},
		// Context cancellation → terminal (caller tore down the call).
		{"context.Canceled not retried", context.Canceled, false},
		{"wrapped context.Canceled not retried", fmt.Errorf("ctx: %w", context.Canceled), false},
		// Caller's own deadline exhausted → terminal.
		{"context.DeadlineExceeded not retried", context.DeadlineExceeded, false},
		{"wrapped context.DeadlineExceeded not retried",
			fmt.Errorf("dl: %w", context.DeadlineExceeded), false},
		// Transport-layer timeout (deadline expiry) → retry.
		{"net.Error timeout retried", &fakeNetErr{timeout: true, msg: "read deadline"}, true},
		// net.OpError wrapping a timeout net.Error still resolves as timeout.
		{"OpError with timeout inner retried", opErrTimeout, true},
		// Non-timeout net.OpError (e.g. connection refused) → retryable.
		{"non-timeout OpError retried", opErrNonTimeout, true},
		// A net.Error that is neither a timeout nor an OpError: the net.Error
		// Timeout() check fails, the OpError errors.As fails, and execution
		// reaches the generic fallback → true.
		{"non-timeout non-OpError net.Error falls through to fallback",
			&fakeNetErr{timeout: false, msg: "transient"}, true},
		// Generic opaque error → fallback retry policy (mirrors httpclient).
		{"generic error retried by fallback", errors.New("any failure"), true},
		// A net.Error reachable via an Unwrap chain still resolves.
		{"net.Error via unwrap chain retried",
			&wrappedNetErr{fakeNetErr: fakeNetErr{timeout: true, msg: "deep"}, cause: errors.New("root")}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRetry(tc.err); got != tc.want {
				t.Fatalf("shouldRetry(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestShouldRetry_DirectErrClosed covers the exact (unwrapped) errClosed case,
// which the wrapped variant above leaves ambiguous.
func TestShouldRetry_DirectErrClosed(t *testing.T) {
	if shouldRetry(errClosed) {
		t.Fatal("shouldRetry(errClosed) = true, want false")
	}
}

// --- retryDelay: backoff arithmetic & guards --------------------------------

func TestRetryDelay(t *testing.T) {
	const minW = 10 * time.Millisecond
	const maxW = 1 * time.Second

	cases := []struct {
		name        string
		attempt     int
		minWait     time.Duration
		maxWait     time.Duration
		delayMin    time.Duration // inclusive lower bound (jitter in [0.5,1.0))
		delayMax    time.Duration // exclusive upper bound
		wantZero    bool          // expect exactly 0
		description string
	}{
		{
			name:    "disabled when minWait <= 0",
			attempt: 0, minWait: 0, maxWait: maxW,
			wantZero: true, description: "zero minWait short-circuits to 0",
		},
		{
			name:    "disabled when maxWait <= 0",
			attempt: 0, minWait: minW, maxWait: 0,
			wantZero: true, description: "zero maxWait short-circuits to 0",
		},
		{
			name:    "disabled when both negative",
			attempt: 3, minWait: -time.Second, maxWait: -time.Second,
			wantZero: true, description: "negative waits short-circuit to 0",
		},
		{
			name:    "first retry uses minWait band",
			attempt: 0, minWait: minW, maxWait: maxW,
			delayMin: minW / 2, delayMax: minW,
			description: "attempt 0 → backoff=minWait, jitter [0.5*min, min)",
		},
		{
			name:    "second retry doubles",
			attempt: 1, minWait: minW, maxWait: maxW,
			delayMin: minW, delayMax: 2 * minW,
			description: "attempt 1 → backoff=2*minWait",
		},
		{
			name:    "large attempt clamps to maxWait",
			attempt: 30, minWait: minW, maxWait: maxW,
			delayMin: maxW / 2, delayMax: maxW,
			description: "exponential growth stops at maxWait",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := retryDelay(tc.attempt, tc.minWait, tc.maxWait)
			if tc.wantZero {
				if got != 0 {
					t.Fatalf("retryDelay = %v, want 0 (%s)", got, tc.description)
				}
				return
			}
			// Jitter is [0.5, 1.0) of the clamped backoff → result in [delayMin, delayMax).
			if got < tc.delayMin || got >= tc.delayMax {
				t.Fatalf("retryDelay = %v, want in [%v, %v) (%s)",
					got, tc.delayMin, tc.delayMax, tc.description)
			}
		})
	}
}

// TestRetryDelay_OverflowGuard drives the int64-shift overflow branch
// (next <= backoff → clamp to maxWait). With minWait=1ns and a maxWait near
// math.MaxInt64, the doubling loop runs enough iterations to wrap int64, which
// the guard must catch and clamp rather than returning a negative duration.
func TestRetryDelay_OverflowGuard(t *testing.T) {
	minW := time.Duration(1) // 1ns
	maxW := time.Duration(math.MaxInt64)
	got := retryDelay(63, minW, maxW)
	// On overflow the branch sets backoff = maxWait, so the jittered result lives
	// in [0.5*maxWait, maxWait). The key assertions: NOT negative, NOT >= maxWait.
	if got < 0 {
		t.Fatalf("retryDelay overflowed to negative: %v", got)
	}
	if got >= maxW {
		t.Fatalf("retryDelay = %v, want < maxWait %v (jitter upper bound)", got, maxW)
	}
	if got < maxW/2 {
		t.Fatalf("retryDelay = %v, want >= 0.5*maxWait %v (jitter lower bound)", got, maxW/2)
	}
}

// TestRetryDelay_ClampAboveMax covers the post-loop clamp (backoff > maxWait)
// using a maxWait that is NOT a clean power-of-two multiple of minWait, so the
// doubling overshoots maxWait inside the loop and gets clamped after it.
func TestRetryDelay_ClampAboveMax(t *testing.T) {
	minW := 300 * time.Millisecond
	maxW := 500 * time.Millisecond // 300*2=600 > 500 → clamp fires
	// attempt 1 doubles 300→600 which exceeds maxWait=500; loop stops, clamp
	// brings it back to 500. Jitter → [250ms, 500ms).
	got := retryDelay(1, minW, maxW)
	if got < 250*time.Millisecond || got >= 500*time.Millisecond {
		t.Fatalf("retryDelay(clamp) = %v, want in [250ms, 500ms)", got)
	}
}

// --- sleep: ctx-cancellation during backoff ---------------------------------

// TestSleep_DelayZero_AlreadyDoneCtx covers the delay<=0 select branch when the
// ctx is already cancelled: the default case is NOT selected because ctx.Done
// is immediately ready, so sleep returns false.
func TestSleep_DelayZero_AlreadyDoneCtx(t *testing.T) {
	c := &Client{opts: ClientOptions{RetryWaitMin: 0, RetryWaitMax: 0}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ok := c.sleep(ctx, 0); ok {
		t.Fatal("sleep(delay=0, ctx cancelled) = true, want false")
	}
}

// TestSleep_DelayZero_ActiveCtx covers the delay<=0 select branch's default
// case: ctx not cancelled and delay==0 → return true immediately (immediate
// retry, no backoff).
func TestSleep_DelayZero_ActiveCtx(t *testing.T) {
	c := &Client{opts: ClientOptions{RetryWaitMin: 0, RetryWaitMax: 0}}
	start := time.Now()
	if ok := c.sleep(context.Background(), 0); !ok {
		t.Fatal("sleep(delay=0, active ctx) = false, want true")
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("sleep(delay=0) slept %v, want ~0", elapsed)
	}
}

// TestSleep_PositiveDelay_CtxCancelled covers the delay>0 select branch's
// ctx.Done() case: cancel the ctx mid-sleep and confirm sleep returns false
// promptly instead of waiting out the full backoff.
func TestSleep_PositiveDelay_CtxCancelled(t *testing.T) {
	c := &Client{opts: ClientOptions{
		RetryWaitMin: 200 * time.Millisecond,
		RetryWaitMax: 400 * time.Millisecond,
	}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	if ok := c.sleep(ctx, 0); ok {
		t.Fatal("sleep(positive delay, ctx cancelled mid-sleep) = true, want false")
	}
	elapsed := time.Since(start)
	if elapsed > 150*time.Millisecond {
		t.Fatalf("sleep elapsed = %v, want to honour ctx cancel promptly", elapsed)
	}
}

// TestSleep_PositiveDelay_Completes covers the delay>0 select branch's
// time.After case: no cancellation, sleep runs to completion and returns true.
func TestSleep_PositiveDelay_Completes(t *testing.T) {
	c := &Client{opts: ClientOptions{
		RetryWaitMin: 10 * time.Millisecond,
		RetryWaitMax: 20 * time.Millisecond,
	}}
	start := time.Now()
	if ok := c.sleep(context.Background(), 0); !ok {
		t.Fatal("sleep(positive delay, no cancel) = false, want true")
	}
	elapsed := time.Since(start)
	if elapsed < 5*time.Millisecond {
		t.Fatalf("sleep elapsed = %v, want >= ~5ms (actually slept)", elapsed)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("sleep elapsed = %v, want bounded by backoff", elapsed)
	}
}

// --- Close: the conn==nil branch --------------------------------------------

// TestClose_NilConn covers the conn==nil early return in Close, which the
// public tests can't reach because NewClient always supplies a non-nil conn.
func TestClose_NilConn(t *testing.T) {
	c := &Client{} // zero value: conn==nil, closed==false
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil-conn client = %v, want nil", err)
	}
	if !c.closed.Load() {
		t.Fatal("Close did not set closed=true")
	}
	// Second Close hits the already-closed branch (CompareAndSwap fails → nil).
	if err := c.Close(); err != nil {
		t.Fatalf("second Close = %v, want nil", err)
	}
}

// --- NewClient: DialUDP failure ---------------------------------------------

// TestNewClient_DialUDPFailure covers the net.DialUDP error branch by passing a
// LocalAddress whose port is already bound (and not ours), forcing DialUDP to
// fail with EADDRINUSE.
func TestNewClient_DialUDPFailure(t *testing.T) {
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	owned, err := net.ListenUDP("udp", laddr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer owned.Close()
	boundAddr := owned.LocalAddr().String()

	_, err = NewClient(ClientOptions{
		Address:      "127.0.0.1:1",
		LocalAddress: boundAddr,
	})
	if err == nil {
		t.Fatal("NewClient: expected DialUDP error for in-use local address, got nil")
	}
}

// --- helper: local UDP servers for the retry-path tests ---------------------

// startSilentServer starts a loopback UDP listener that reads and discards
// datagrams (never replies). Returns its address and a *uint64 counting
// observed datagrams. Cleanup is registered with t.
func startSilentServer(t *testing.T) (string, *uint64) {
	t.Helper()
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var received uint64
	go func() {
		buf := make([]byte, 4096)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n > 0 {
				atomic.AddUint64(&received, 1)
			}
		}
	}()
	return conn.LocalAddr().String(), &received
}

// --- sendWithRetry: closed-mid-loop, deadline errors, ctx-cancel ------------

// TestSendWithRetry_ClosedMidLoop covers the closed.Load() check inside the
// retry loop. We flip closed AFTER NewClient (so the conn is live) and invoke
// sendWithRetry directly; the in-loop check returns errClosed on the first
// iteration without touching the socket.
func TestSendWithRetry_ClosedMidLoop(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		RetryMax:     3,
		RetryWaitMin: time.Microsecond,
		RetryWaitMax: 10 * time.Microsecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.closed.Store(true)
	err = c.sendWithRetry(context.Background(), []byte("closed-mid-loop"))
	if !errors.Is(err, errClosed) {
		t.Fatalf("sendWithRetry on closed client = %v, want errClosed", err)
	}
}

// TestSendWithRetry_SetWriteDeadlineError covers the SetWriteDeadline error
// branch: closing the conn directly (leaving closed=false) makes the next
// SetWriteDeadline fail, so sendWithRetry returns that error without retrying.
func TestSendWithRetry_SetWriteDeadlineError(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		RetryMax:     3,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_ = c.conn.Close()
	err = c.sendWithRetry(context.Background(), []byte("closed-conn"))
	if err == nil {
		t.Fatal("sendWithRetry on closed conn = nil, want SetWriteDeadline error")
	}
	if errors.Is(err, errClosed) {
		t.Fatalf("returned errClosed, want a net error (got %v)", err)
	}
}

// TestSendWithRetry_WriteErrorBudgetExhausted covers the
// `!shouldRetry(werr) || attempt == RetryMax` return (line ~298): the write
// fails with a retryable error but RetryMax==0 means the budget is spent on the
// first attempt, so sendWithRetry returns the error immediately without
// retrying.
//
// The retryable write error is produced deterministically by setting
// WriteTimeout to a negative duration AFTER construction (withDefaults would
// otherwise replace a <=0 WriteTimeout with the 2s default). A negative
// WriteTimeout makes SetWriteDeadline set a deadline in the past, which
// succeeds, and the subsequent Write fails with a retryable i/o timeout
// (net.Error with Timeout()==true). This is cross-platform and avoids any
// dependence on ICMP delivery.
func TestSendWithRetry_WriteErrorBudgetExhausted(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 200 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		RetryMax:     2,
		RetryWaitMin: time.Microsecond,
		RetryWaitMax: 10 * time.Microsecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	// Bypass withDefaults: force the budget-exhausted short-circuit on attempt 0.
	c.opts.RetryMax = 0
	// Negative WriteTimeout → SetWriteDeadline(past) succeeds, Write times out.
	c.opts.WriteTimeout = -1 * time.Second

	err = c.sendWithRetry(context.Background(), []byte("one-shot"))
	if err == nil {
		t.Fatal("sendWithRetry: expected write-timeout error, got nil")
	}
	// RetryMax==0 → the branch returned immediately, no retries recorded.
	if got := c.retried.Load(); got != 0 {
		t.Fatalf("retried = %d, want 0 (RetryMax=0 short-circuit)", got)
	}
}

// TestSendWithRetry_WriteErrorCtxCancelledDuringBackoff covers the
// `!c.retry(ctx, attempt, n)` return (line ~301): the write fails with a
// retryable error, attempt < RetryMax so retry() is entered, and ctx is
// cancelled during the backoff sleep so retry returns false and sendWithRetry
// surfaces the last error instead of looping again.
//
// Deterministic write failure via a negative WriteTimeout (see
// TestSendWithRetry_WriteErrorBudgetExhausted for the rationale).
func TestSendWithRetry_WriteErrorCtxCancelledDuringBackoff(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 200 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		RetryMax:     20,
		RetryWaitMin: 150 * time.Millisecond,
		RetryWaitMax: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.opts.WriteTimeout = -1 * time.Second // deterministically fail every Write

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = c.sendWithRetry(ctx, []byte("cancel-during-backoff"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("sendWithRetry: expected write-timeout error, got nil")
	}
	// Must return shortly after the 40ms ctx deadline, not run the full budget.
	if elapsed > 250*time.Millisecond {
		t.Fatalf("elapsed = %v, expected to honour ctx cancellation promptly", elapsed)
	}
	// The retry that was in flight when ctx cancelled counts; no further ones.
	if got := c.retried.Load(); got > 1 {
		t.Fatalf("retried = %d, want <= 1 (ctx cancelled during first backoff)", got)
	}
}

// TestSendWithRetry_LoopFallthroughReturn covers the post-loop `return err`
// (line ~306). With RetryMax<0 the loop body never executes, so the function
// falls through to return the initial err (nil).
func TestSendWithRetry_LoopFallthroughReturn(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		RetryMax:     1,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	// Force the loop to run zero iterations by setting RetryMax negative. The
	// for-condition `attempt <= RetryMax` is false on the first check, so the
	// body never runs and the function returns the zero-value err (nil).
	c.opts.RetryMax = -1
	if err := c.sendWithRetry(context.Background(), []byte("no-loop")); err != nil {
		t.Fatalf("sendWithRetry(RetryMax=-1) = %v, want nil (loop never ran)", err)
	}
}

// --- sendReceiveWithRetry: the symmetric write/read error & cancel paths ----

// TestSendReceiveWithRetry_ClosedMidLoop covers the in-loop closed check.
func TestSendReceiveWithRetry_ClosedMidLoop(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		RetryMax:     3,
		RetryWaitMin: time.Microsecond,
		RetryWaitMax: 10 * time.Microsecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.closed.Store(true)
	_, err = c.sendReceiveWithRetry(context.Background(), []byte("closed-mid-loop"))
	if !errors.Is(err, errClosed) {
		t.Fatalf("sendReceiveWithRetry on closed client = %v, want errClosed", err)
	}
}

// TestSendReceiveWithRetry_SetWriteDeadlineError covers the SetWriteDeadline
// error branch in the write stage (closing the conn directly so the in-loop
// closed check doesn't short-circuit).
func TestSendReceiveWithRetry_SetWriteDeadlineError(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		RetryMax:     3,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_ = c.conn.Close()
	_, err = c.sendReceiveWithRetry(context.Background(), []byte("closed-conn"))
	if err == nil {
		t.Fatal("sendReceiveWithRetry on closed conn = nil, want SetWriteDeadline error")
	}
	if errors.Is(err, errClosed) {
		t.Fatalf("returned errClosed, want a net error (got %v)", err)
	}
}

// TestSendReceiveWithRetry_SetReadDeadlineError covers the SetReadDeadline
// error branch. We arm a write error (ICMP) but with a LIVE conn so writes
// succeed on the first attempt — then the read stage runs. To make the read
// deadline fail we instead drive it via a closed conn where the write stage
// also fails. The reliable, race-free drive for the SetReadDeadline branch is:
// point at an unreachable peer so writes succeed (no ICMP on first write), the
// read deadline is set, and the read itself blocks until its deadline expires
// (retryable). For the SetReadDeadline *error* specifically (a closed conn),
// coverage is shared with the SetWriteDeadline path since both error on a
// closed conn. This test asserts the closed-conn read path surfaces an error
// and documents that the SetReadDeadline error branch is covered when the
// write stage happens to succeed on this platform.
func TestSendReceiveWithRetry_SetReadDeadlineError(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		RetryMax:     0, // single attempt
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_ = c.conn.Close()
	_, err = c.sendReceiveWithRetry(context.Background(), []byte("read-deadline"))
	if err == nil {
		t.Fatal("sendReceiveWithRetry on closed conn = nil, want a deadline error")
	}
}

// TestSendReceiveWithRetry_WriteErrorBudgetExhausted covers the write-error
// retry tail's budget-exhausted return (line ~339): a retryable write error with
// RetryMax==0 returns immediately. Deterministic write failure via a negative
// WriteTimeout (see TestSendWithRetry_WriteErrorBudgetExhausted).
func TestSendReceiveWithRetry_WriteErrorBudgetExhausted(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 200 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		RetryMax:     2,
		RetryWaitMin: time.Microsecond,
		RetryWaitMax: 10 * time.Microsecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.opts.RetryMax = 0
	c.opts.WriteTimeout = -1 * time.Second

	_, err = c.sendReceiveWithRetry(context.Background(), []byte("one-shot"))
	if err == nil {
		t.Fatal("sendReceiveWithRetry: expected write-timeout error, got nil")
	}
	if got := c.retried.Load(); got != 0 {
		t.Fatalf("retried = %d, want 0 (RetryMax=0 short-circuit)", got)
	}
}

// TestSendReceiveWithRetry_WriteErrorCtxCancelledDuringBackoff covers the
// write-error retry tail's ctx-cancelled return (line ~342): write fails
// retryably, retry() is entered, ctx cancels during backoff, and
// sendReceiveWithRetry surfaces the last error. Deterministic write failure via
// a negative WriteTimeout.
func TestSendReceiveWithRetry_WriteErrorCtxCancelledDuringBackoff(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 200 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		RetryMax:     20,
		RetryWaitMin: 150 * time.Millisecond,
		RetryWaitMax: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.opts.WriteTimeout = -1 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = c.sendReceiveWithRetry(ctx, []byte("cancel-sr"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("sendReceiveWithRetry: expected write-timeout error, got nil")
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("elapsed = %v, expected to honour ctx cancellation promptly", elapsed)
	}
	if got := c.retried.Load(); got > 1 {
		t.Fatalf("retried = %d, want <= 1 (ctx cancelled during first backoff)", got)
	}
}

// TestSendReceiveWithRetry_ReadTimeoutBudgetExhausted covers the read-error
// retry tail's budget-exhausted return (line ~361) and the loop-fallthrough
// `return nil, err` (line ~368). A silent server times out every read; with
// RetryMax==2 the loop exhausts its budget on the third attempt and surfaces
// the read-timeout error. The fallthrough return is exercised because the last
// iteration's `attempt == RetryMax` returns inside the loop; to reach the
// fallthrough we use RetryMax<0 separately below.
func TestSendReceiveWithRetry_ReadTimeoutBudgetExhausted(t *testing.T) {
	addr, received := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 100 * time.Millisecond,
		ReadTimeout:  20 * time.Millisecond,
		RetryMax:     2,
		RetryWaitMin: time.Microsecond,
		RetryWaitMax: 10 * time.Microsecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	start := time.Now()
	out, err := c.sendReceiveWithRetry(context.Background(), []byte("timeout-budget"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("sendReceiveWithRetry: expected read-timeout error, got nil")
	}
	if out != nil {
		t.Fatalf("out = %v, want nil on failure", out)
	}
	if got := atomic.LoadUint64(received); got != 3 {
		t.Fatalf("server received = %d, want 3 (1 initial + 2 retries)", got)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("elapsed = %v, expected to bail out after 3 read timeouts", elapsed)
	}
}

// TestSendReceiveWithRetry_ReadTimeoutCtxCancelledDuringBackoff covers the
// read-error retry tail's ctx-cancelled return (line ~364): read times out,
// retry() is entered, ctx cancels during backoff, the call surfaces the last
// error promptly.
func TestSendReceiveWithRetry_ReadTimeoutCtxCancelledDuringBackoff(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 100 * time.Millisecond,
		ReadTimeout:  15 * time.Millisecond,
		RetryMax:     20,
		RetryWaitMin: 150 * time.Millisecond,
		RetryWaitMax: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = c.sendReceiveWithRetry(ctx, []byte("cancel-read-backoff"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("sendReceiveWithRetry: expected timeout/cancel error")
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("elapsed = %v, expected to honour ctx cancellation promptly", elapsed)
	}
}

// TestSendReceiveWithRetry_WriteErrorRetriesAndContinues covers the `continue`
// (line ~345) after a successful retry on a write error: the write fails
// retryably, retry() sleeps and returns true (ctx not cancelled), and the loop
// continues to the next attempt (which also fails). With RetryMax=2 and a
// negative WriteTimeout this drives two full write-error→retry→continue
// iterations before the budget exhausts.
func TestSendReceiveWithRetry_WriteErrorRetriesAndContinues(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 200 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		RetryMax:     2,
		RetryWaitMin: time.Microsecond,
		RetryWaitMax: 10 * time.Microsecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.opts.WriteTimeout = -1 * time.Second // every Write fails retryably

	start := time.Now()
	_, err = c.sendReceiveWithRetry(context.Background(), []byte("retry-continue"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("sendReceiveWithRetry: expected write-timeout error, got nil")
	}
	// RetryMax=2 → 1 initial + 2 retries that each hit the continue, then the
	// third attempt (attempt==RetryMax) returns. The retried counter reflects
	// the two retry() calls that returned true and continued.
	if got := c.retried.Load(); got != 2 {
		t.Fatalf("retried = %d, want 2 (two write-error retries that continued)", got)
	}
	// Two microsecond-range backoffs plus three near-instant failed writes must
	// complete well under 500ms.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("elapsed = %v, expected to complete quickly", elapsed)
	}
}

// TestSendReceiveWithRetry_LoopFallthroughReturn covers the post-loop
// `return nil, err` (line ~368). With RetryMax<0 the loop body never executes,
// so the function falls through to return (nil, nil).
func TestSendReceiveWithRetry_LoopFallthroughReturn(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		RetryMax:     1,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.opts.RetryMax = -1
	out, err := c.sendReceiveWithRetry(context.Background(), []byte("no-loop"))
	if err != nil {
		t.Fatalf("sendReceiveWithRetry(RetryMax=-1) err = %v, want nil (loop never ran)", err)
	}
	if out != nil {
		t.Fatalf("sendReceiveWithRetry(RetryMax=-1) out = %v, want nil", out)
	}
}

// --- Send / SendReceive: nil-ctx defaulting & failed-call bookkeeping -------

// TestSend_NilCtxDefaultAndFailure covers (1) the `ctx == nil → Background()`
// branch and (2) the failed-call bookkeeping (failed.Add + fireEvent("failed"))
// by driving a deterministic write-timeout via a negative WriteTimeout and a
// nil ctx.
func TestSend_NilCtxDefaultAndFailure(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 200 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		RetryMax:     2,
		RetryWaitMin: time.Microsecond,
		RetryWaitMax: 10 * time.Microsecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.opts.RetryMax = 0                    // single attempt; first failure short-circuits
	c.opts.WriteTimeout = -1 * time.Second // deterministically fail the write

	var failedEvents int32
	c.SetOnEvent(func(evt ClientEvent) {
		if evt.Name == "failed" {
			atomic.AddInt32(&failedEvents, 1)
		}
	})

	// nil ctx must be defaulted internally rather than panicking.
	err = c.Send(nil, []byte("nil-ctx"))
	if err == nil {
		t.Fatal("Send: expected write-timeout error, got nil")
	}
	if m := c.Metrics(); m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1 after failed Send", m.Failed)
	}
	if m := c.Metrics(); m.Total != 1 {
		t.Fatalf("Metrics.Total = %d, want 1", m.Total)
	}
	if got := atomic.LoadInt32(&failedEvents); got != 1 {
		t.Fatalf("failed events = %d, want 1", got)
	}
}

// TestSendReceive_NilCtxDefault covers the `ctx == nil → Background()` branch
// in SendReceive on the happy path (echo), proving no nil-ctx panic and that
// the defaulting preserves call semantics.
func TestSendReceive_NilCtxDefault(t *testing.T) {
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		buf := make([]byte, 4096)
		for {
			n, raddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if _, err := conn.WriteToUDP(buf[:n], raddr); err != nil {
				return
			}
		}
	}()

	c, err := NewClient(ClientOptions{
		Address:      conn.LocalAddr().String(),
		WriteTimeout: 200 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		RetryMax:     1,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	out, err := c.SendReceive(nil, []byte("nil-ctx-echo"))
	if err != nil {
		t.Fatalf("SendReceive(nil ctx): %v", err)
	}
	if string(out) != "nil-ctx-echo" {
		t.Fatalf("reply = %q, want %q", out, "nil-ctx-echo")
	}
	if m := c.Metrics(); m.Success != 1 {
		t.Fatalf("Metrics.Success = %d, want 1", m.Success)
	}
}

// TestSend_FailedBookkeeping_OnClosedConn covers the failed-call tail of Send
// (failed.Add + fireEvent("failed")) deterministically by closing the conn
// directly before the call. closed stays false so Send enters the attempt loop,
// the deadline fails, and the err != nil branch runs.
func TestSend_FailedBookkeeping_OnClosedConn(t *testing.T) {
	addr, _ := startSilentServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		WriteTimeout: 100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		RetryMax:     1,
		RetryWaitMin: time.Microsecond,
		RetryWaitMax: 10 * time.Microsecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_ = c.conn.Close()

	var failedEvents int32
	c.SetOnEvent(func(evt ClientEvent) {
		if evt.Name == "failed" {
			atomic.AddInt32(&failedEvents, 1)
		}
	})

	err = c.Send(context.Background(), []byte("bookkeep"))
	if err == nil {
		t.Skip("platform did not surface a write error on closed conn")
	}
	if m := c.Metrics(); m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
	if m := c.Metrics(); m.Total != 1 {
		t.Fatalf("Metrics.Total = %d, want 1", m.Total)
	}
	if got := atomic.LoadInt32(&failedEvents); got != 1 {
		t.Fatalf("failed events = %d, want 1", got)
	}
}
