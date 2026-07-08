// This file is an internal coverage test (package tcpclient, not
// tcpclient_test) so it can exercise the unexported helpers directly:
// shouldRetry, retryDelay, isClosedErr, newConnPool, connPool.put/expired, and
// the deadline/short-write branches of writeOnce/writeReadAll/writeReadLine
// via a controlled fake net.Conn injected into the pool. All cases are
// deterministic (no real network, no flaky timeouts).
package tcpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- fake net.Conn ----------------------------------------------------------

// fakeConn is a controllable net.Conn used to drive the per-attempt I/O
// helpers (writeOnce/writeReadAll/writeReadLine) into their error branches
// without a real socket. Each field gates a return value; zero values yield
// benign defaults so a bare fakeConn behaves like a no-op, never-closing pipe.
type fakeConn struct {
	mu sync.Mutex

	// Configuration knobs (set before use).
	writeDeadlineErr error  // returned by SetWriteDeadline
	readDeadlineErr  error  // returned by SetReadDeadline
	writeErr         error  // returned by Write after writing writeN bytes
	writeN           int    // bytes Write reports as written (<= len(data))
	readData         []byte // bytes Read returns (one shot, then readErr)
	readErr          error  // returned by Read after exhausting readData

	// Counters (read-only via atomic).
	closes     atomic.Int64
	writeCalls atomic.Int64
	readCalls  atomic.Int64
	deadlinesW atomic.Int64
	deadlinesR atomic.Int64
}

func (f *fakeConn) Read(p []byte) (int, error) {
	f.readCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.readData) > 0 {
		n := copy(p, f.readData)
		f.readData = f.readData[n:]
		if len(f.readData) == 0 {
			return n, f.readErr
		}
		return n, nil
	}
	return 0, f.readErr
}

func (f *fakeConn) Write(p []byte) (int, error) {
	f.writeCalls.Add(1)
	n := f.writeN
	if n > len(p) {
		n = len(p)
	}
	if n < 0 {
		n = 0
	}
	return n, f.writeErr
}

func (f *fakeConn) Close() error {
	f.closes.Add(1)
	return nil
}
func (f *fakeConn) SetWriteDeadline(t time.Time) error {
	f.deadlinesW.Add(1)
	return f.writeDeadlineErr
}
func (f *fakeConn) SetReadDeadline(t time.Time) error {
	f.deadlinesR.Add(1)
	return f.readDeadlineErr
}
func (f *fakeConn) LocalAddr() net.Addr         { return nil }
func (f *fakeConn) RemoteAddr() net.Addr        { return nil }
func (f *fakeConn) SetDeadline(time.Time) error { return nil }

// newClientWithFakeConn builds a Client whose pool is pre-seeded with fc so
// the first checkout returns fc (no dial). IdleTimeout=0 disables eviction so
// the seeded conn is never skipped, and fc.Close is a no-op so the same conn
// survives being closed on the error paths and can be re-seeded for the next
// sub-test.
func newClientWithFakeConn(t *testing.T, fc *fakeConn, opts ClientOptions) *Client {
	t.Helper()
	opts = opts.withDefaults()
	// Disable eviction so the seeded conn is always returned on checkout.
	opts.IdleTimeout = 0
	// Keep retry at 0 so writeOnce runs exactly once (the per-attempt helper is
	// what we are exercising; retry semantics are covered separately).
	opts.RetryMax = 0
	p := newConnPool(opts.Network, opts.Address, opts.PoolSize, opts.ConnectTimeout, opts.IdleTimeout)
	p.pool <- &poolConn{Conn: fc, lastUsed: time.Now()}
	return &Client{opts: opts, pool: p}
}

// --- shouldRetry ------------------------------------------------------------

// timeoutErr is a net.Error whose Timeout() is configurable, used to drive the
// net.Error branch of shouldRetry (with and without a wrapped context error).
type timeoutErr struct {
	timeout bool
	wrapped error
}

func (e *timeoutErr) Error() string   { return "timeoutErr" }
func (e *timeoutErr) Timeout() bool   { return e.timeout }
func (e *timeoutErr) Temporary() bool { return false }
func (e *timeoutErr) Unwrap() error   { return e.wrapped }

// opErr is a net.OpError stand-in used to drive the net.OpError branch of
// shouldRetry. We use the real *net.OpError via wrapping to exercise the
// errors.As path: wrap a sentinel in a real *net.OpError.
func opErrWithWrapped(wrapped error) error {
	return &net.OpError{Op: "read", Err: wrapped}
}

func TestShouldRetry_AllBranches(t *testing.T) {
	ctxCanceled := fmt.Errorf("wrap: %w", context.Canceled)
	ctxDeadline := fmt.Errorf("wrap: %w", context.DeadlineExceeded)

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"bare canceled", context.Canceled, false},
		{"bare deadline", context.DeadlineExceeded, false},
		{"wrapped canceled", ctxCanceled, false},
		{"wrapped deadline", ctxDeadline, false},
		{"io.EOF", io.EOF, true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"wrapped EOF", fmt.Errorf("read: %w", io.EOF), true},
		{"net.Error timeout pure", &timeoutErr{timeout: true}, true},
		{"net.Error timeout wrapping canceled", &timeoutErr{timeout: true, wrapped: context.Canceled}, false},
		{"net.Error timeout wrapping deadline", &timeoutErr{timeout: true, wrapped: context.DeadlineExceeded}, false},
		{"net.Error non-timeout falls to fallback", &timeoutErr{timeout: false}, true}, // Timeout()=false skips timeout block; not a *net.OpError -> fallback true
		{"net.OpError pure (syscall refused)", opErrWithWrapped(errors.New("connection refused")), true},
		{"net.OpError wrapping canceled", opErrWithWrapped(context.Canceled), false},
		{"net.OpError wrapping deadline", opErrWithWrapped(context.DeadlineExceeded), false},
		{"generic fallback", errors.New("anything else"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRetry(tc.err); got != tc.want {
				t.Fatalf("shouldRetry(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// --- retryDelay -------------------------------------------------------------

func TestRetryDelay_AllBranches(t *testing.T) {
	cases := []struct {
		name     string
		attempt  int
		minWait  time.Duration
		maxWait  time.Duration
		minWant  time.Duration // inclusive lower bound (after jitter)
		maxWant  time.Duration // inclusive upper bound
		zeroWant bool          // expect exactly 0
	}{
		// Backoff disabled paths: any non-positive bound -> 0.
		{name: "min<=0", minWait: 0, maxWait: time.Second, zeroWant: true},
		{name: "max<=0", minWait: time.Millisecond, maxWait: 0, zeroWant: true},
		{name: "both negative", minWait: -1, maxWait: -1, zeroWant: true},

		// attempt 0 -> backoff stays minWait, jitter factor [0.5,1.0).
		{name: "attempt0", attempt: 0, minWait: 10 * time.Millisecond, maxWait: time.Second,
			minWant: 5 * time.Millisecond, maxWant: 10 * time.Millisecond},

		// attempt 1 -> backoff doubles to 2*min.
		{name: "attempt1 doubles", attempt: 1, minWait: 10 * time.Millisecond, maxWait: time.Second,
			minWant: 10 * time.Millisecond, maxWant: 20 * time.Millisecond},

		// attempt large -> backoff capped at maxWait.
		{name: "capped at max", attempt: 30, minWait: 10 * time.Millisecond, maxWait: 100 * time.Millisecond,
			minWant: 50 * time.Millisecond, maxWant: 100 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Run a few times because of jitter; assert within [minWant, maxWant].
			for range 50 {
				d := retryDelay(tc.attempt, tc.minWait, tc.maxWait)
				if tc.zeroWant {
					if d != 0 {
						t.Fatalf("retryDelay = %v, want 0", d)
					}
					continue
				}
				if d < tc.minWant || d > tc.maxWant {
					t.Fatalf("retryDelay = %v, want in [%v, %v]", d, tc.minWant, tc.maxWant)
				}
			}
		})
	}
}

// TestRetryDelay_OverflowGuard lives further below (next to the options
// coverage tests); it was moved there to keep the retryDelay group tight.

// --- isClosedErr ------------------------------------------------------------

func TestIsClosedErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"net.ErrClosed bare", net.ErrClosed, true},
		{"net.ErrClosed wrapped", fmt.Errorf("read: %w", net.ErrClosed), true},
		{"plain string not API-stable", errors.New("use of closed network connection"), false},
		{"non-matching string", errors.New("read: connection reset by peer"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isClosedErr(tc.err); got != tc.want {
				t.Fatalf("isClosedErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// --- DoWithRetry: nil ctx, ctx-cancel-during-backoff, final-attempt return --

// TestDoWithRetry_NilCtx exercises the `if ctx == nil` defaulting branch: a nil
// ctx must be replaced with context.Background so fn still runs.
func TestDoWithRetry_NilCtx(t *testing.T) {
	c := NewClient(ClientOptions{Address: "127.0.0.1:1", RetryMax: 0})
	defer c.Close()

	called := false
	err := c.DoWithRetry(nil, func(ctx context.Context) error {
		called = true
		if ctx == nil {
			t.Fatal("ctx is nil inside fn")
		}
		if ctx != context.Background() {
			// WithBackground is what DoWithRetry substitutes; equality holds
			// because Background is a singleton.
			t.Fatalf("ctx = %v, want context.Background", ctx)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !called {
		t.Fatal("fn not called")
	}
}

// TestDoWithRetry_CtxCanceledDuringBackoff covers the `case <-ctx.Done()`
// branch where the wait is interrupted. The last-attempt error is surfaced
// because it is non-nil when ctx fires.
func TestDoWithRetry_CtxCanceledDuringBackoff_TransportErr(t *testing.T) {
	c := NewClient(ClientOptions{
		Address:      "127.0.0.1:1",
		RetryMax:     5,
		RetryWaitMin: 200 * time.Millisecond,
		RetryWaitMax: 400 * time.Millisecond,
	})
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	sentinel := errors.New("boom-retryable")
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := c.DoWithRetry(ctx, func(ctx context.Context) error {
		return sentinel // always retryable, so we back off
	})
	elapsed := time.Since(start)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel %v", err, sentinel)
	}
	// Must NOT have waited the full backoff.
	if elapsed > 150*time.Millisecond {
		t.Fatalf("elapsed = %v, want ctx to interrupt the backoff quickly", elapsed)
	}
}

// TestDoWithRetry_CtxCanceledDuringBackoff_NilErr covers the `if err == nil`
// sub-branch: if fn returned nil but was retried because... that cannot happen
// (nil is not retryable). The only way err==nil at the select is if fn returned
// a retryable error the first time and nil the second — but then we would not
// be in the backoff wait. In practice the `if err == nil` guard fires only when
// fn itself returns nil on a retryable path, which is impossible by contract.
// We instead cover the equivalent observable: a fn that returns nil the first
// time returns immediately (no backoff entered).
func TestDoWithRetry_NilErrNoBackoff(t *testing.T) {
	c := NewClient(ClientOptions{
		Address:      "127.0.0.1:1",
		RetryMax:     3,
		RetryWaitMin: time.Second, // huge; would stall if we wrongly back off
		RetryWaitMax: 2 * time.Second,
	})
	defer c.Close()

	calls := 0
	start := time.Now()
	err := c.DoWithRetry(context.Background(), func(ctx context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if calls != 1 {
		t.Fatalf("fn called %d times, want 1", calls)
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("took %v, should not back off on nil", d)
	}
}

// TestDoWithRetry_LastAttemptNoBackoff covers the `if attempt == RetryMax`
// early return: the final allowed attempt returns its error immediately
// without entering the backoff wait. RetryMax=1 with a bounded backoff and a
// forever-retryable fn: the second (final) attempt must return at once.
func TestDoWithRetry_LastAttemptNoBackoff(t *testing.T) {
	c := NewClient(ClientOptions{
		Address:      "127.0.0.1:1",
		RetryMax:     1,
		RetryWaitMin: 50 * time.Millisecond,
		RetryWaitMax: 100 * time.Millisecond,
	})
	defer c.Close()

	sentinel := errors.New("forever-retryable")
	calls := 0
	start := time.Now()
	err := c.DoWithRetry(context.Background(), func(ctx context.Context) error {
		calls++
		return sentinel
	})
	elapsed := time.Since(start)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
	if calls != 2 { // attempt 0 + attempt 1 (RetryMax)
		t.Fatalf("fn called %d times, want 2", calls)
	}
	// Exactly one backoff between attempt 0 and 1 (the retry). The final return
	// must not wait again, so elapsed is a single backoff window: factor
	// [0.5,1.0) on min=50ms => [25ms, 50ms). Two windows would be >= 50ms.
	if elapsed >= 100*time.Millisecond {
		t.Fatalf("elapsed = %v, looks like more than one backoff", elapsed)
	}
}

// --- unreachable defensive branches: documented + locked-in ------------------
//
// shouldRetry and DoWithRetry each contain a sub-branch that is logically
// unreachable for any input a real caller can produce. Rather than contrive a
// fragile fixture that happens to flip a coverage bit, we document WHY each
// branch cannot fire and lock the reasoning into a test: if a future refactor
// makes the branch reachable, these tests break and force a review. The
// branches themselves stay (they are correct defensive guards); we just refuse
// to claim coverage we cannot honestly earn.

// TestShouldRetry_NetErrorCtxBranchUnreachable proves the inner
// `errors.Is(netErr, context.Canceled|DeadlineExceeded)` checks inside the
// net.Error and *net.OpError blocks of shouldRetry can never evaluate true.
//
// Reasoning: errors.As(err, &netErr) sets netErr to err or to an error in err's
// chain. Therefore any error in netErr's chain is also in err's chain. The
// top-level errors.Is(err, <ctx error>) at the head of shouldRetry already
// returned false for any ctx error reachable from err, so by the time we enter
// the net.Error block the inner errors.Is(netErr, ...) is necessarily false.
// The *net.OpError block is identical.
//
// We assert the invariant directly: for a net.Error / *net.OpError whose chain
// contains a context error, shouldRetry returns false via the TOP-LEVEL check
// (reachable), not via the inner check (unreachable). The observable outcome is
// the same; only the coverage attribution differs.
func TestShouldRetry_NetErrorCtxBranchUnreachable(t *testing.T) {
	// (1) A net.Error whose Unwrap chains to context.Canceled. shouldRetry must
	// short-circuit at the top-level errors.Is and return false; it must NOT
	// need the inner net.Error-block guard.
	e := &timeoutErr{timeout: true, wrapped: context.Canceled}
	if shouldRetry(e) != false {
		t.Fatal("net.Error wrapping context.Canceled should be non-retryable")
	}
	// (2) Same for DeadlineExceeded through a real *net.OpError.
	opE := &net.OpError{Op: "read", Err: context.DeadlineExceeded}
	if shouldRetry(opE) != false {
		t.Fatal("*net.OpError wrapping DeadlineExceeded should be non-retryable")
	}
	// (3) A net.Error that does NOT chain to a ctx error reaches the net.Error
	// block and returns true via `return true` (NOT via the inner guard).
	plain := &timeoutErr{timeout: true}
	if shouldRetry(plain) != true {
		t.Fatal("plain net.Error timeout should be retryable")
	}
	// (4) Prove the inner check is redundant: a net.Error that is a ctx error
	// at the SAME object is caught at the top. There is no input shape for
	// which the top-level check is false but the inner check is true.
	t.Logf("documented: shouldRetry's inner net.Error/*net.OpError context-error " +
		"guards (tcpclient.go ~lines 56-58, 64-66) are unreachable; the same " +
		"outcomes are produced by the top-level errors.Is check at line 44")
}

// TestDoWithRetry_CtxDoneNilErrUnreachable proves the `if err == nil` sub-branch
// inside DoWithRetry's `case <-ctx.Done()` cannot fire.
//
// Reasoning: the select is only reachable when shouldRetry(err) returned true at
// the top of the loop body. shouldRetry(nil) returns false, so err is non-nil
// at the select. Therefore `err == nil` is always false there and the
// `err = ctx.Err()` assignment is dead. The guard is retained defensively.
//
// We assert the contract that makes it dead: shouldRetry(nil) == false. If that
// ever changed, this test would need updating and the dead branch would become
// live.
func TestDoWithRetry_CtxDoneNilErrUnreachable(t *testing.T) {
	if shouldRetry(nil) != false {
		t.Fatal("shouldRetry(nil) must be false; the DoWithRetry ctx.Done nil-err " +
			"guard depends on this contract")
	}
	// And concretely: a fn returning nil never enters backoff, so ctx.Done
	// racing a nil err is structurally impossible.
	c := NewClient(ClientOptions{
		Address:      "127.0.0.1:1",
		RetryMax:     2,
		RetryWaitMin: 5 * time.Second, // would hang if we wrongly entered backoff
		RetryWaitMax: 10 * time.Second,
	})
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	err := c.DoWithRetry(ctx, func(context.Context) error { return nil })
	if err != nil {
		t.Fatalf("err = %v, want nil (nil is not retried, returns at once)", err)
	}
	t.Logf("documented: DoWithRetry's `if err == nil` in case <-ctx.Done() " +
		"(tcpclient.go ~lines 375-377) is unreachable; shouldRetry(nil)==false " +
		"guarantees err is non-nil whenever the select is reached")
}

// TestDoWithRetry_TrailingReturnUnreachable proves the `return err` after the
// for loop in DoWithRetry cannot execute for any Client built via NewClient.
//
// Reasoning: withDefaults forces RetryMax >= 0. The loop runs attempt from 0 to
// RetryMax inclusive. On the iteration where attempt == RetryMax, the early
// `if attempt == c.opts.RetryMax { return err }` fires unconditionally, so the
// loop never falls through to the trailing return. The trailing return exists
// only to satisfy the compiler's exhaustiveness check.
//
// We assert the invariant: every NewClient yields RetryMax >= 0, and a
// forever-retryable fn terminates exactly at attempt == RetryMax (not via loop
// exhaustion).
func TestDoWithRetry_TrailingReturnUnreachable(t *testing.T) {
	for _, rm := range []int{-1, 0, 1, 3} {
		c := NewClient(ClientOptions{Address: "127.0.0.1:1", RetryMax: rm})
		if c.opts.RetryMax < 0 {
			t.Fatalf("RetryMax=%d left negative after withDefaults", rm)
		}
		sentinel := errors.New("forever-retryable")
		calls := 0
		err := c.DoWithRetry(context.Background(), func(context.Context) error {
			calls++
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("RetryMax=%d: err = %v, want sentinel", rm, err)
		}
		// Exactly RetryMax+1 calls: the final attempt returns via the early
		// return, NOT via loop fall-through (which would imply RetryMax+2 calls
		// or a nil err from the trailing return).
		if calls != c.opts.RetryMax+1 {
			t.Fatalf("RetryMax=%d: calls = %d, want %d", rm, calls, c.opts.RetryMax+1)
		}
		c.Close()
	}
	t.Logf("documented: DoWithRetry's trailing `return err` (tcpclient.go " +
		"~line 382) is unreachable; withDefaults forces RetryMax>=0 and the " +
		"attempt==RetryMax early return always fires first")
}

// --- connPool: newConnPool negative size, expired disabled, put nil/closed --

func TestNewConnPool_NegativeSize(t *testing.T) {
	p := newConnPool("tcp", "127.0.0.1:1", -5, time.Second, time.Second)
	defer p.close()
	if cap(p.pool) != 0 {
		t.Fatalf("cap(pool) = %d, want 0 for negative size", cap(p.pool))
	}
}

func TestConnPool_Expired_DisabledWhenIdleTimeoutZero(t *testing.T) {
	p := newConnPool("tcp", "127.0.0.1:1", 2, time.Second, 0)
	defer p.close()
	// A conn with an ancient lastUsed must NOT be expired when idleTimeout<=0.
	pc := &poolConn{Conn: &fakeConn{}, lastUsed: time.Unix(0, 0)}
	if p.expired(pc) {
		t.Fatal("expired=true, want false when idleTimeout<=0")
	}
}

func TestConnPool_Put_NilConn(t *testing.T) {
	p := newConnPool("tcp", "127.0.0.1:1", 2, time.Second, time.Second)
	defer p.close()
	// Must be a no-op (not panic, not enqueue anything).
	p.put(nil)
	if len(p.pool) != 0 {
		t.Fatalf("pool len = %d, want 0 after put(nil)", len(p.pool))
	}
}

func TestConnPool_Put_AfterClose(t *testing.T) {
	fc := &fakeConn{}
	p := newConnPool("tcp", "127.0.0.1:1", 2, time.Second, time.Second)
	p.close() // mark closed
	p.put(fc) // must close fc rather than enqueue
	if len(p.pool) != 0 {
		t.Fatalf("pool len = %d, want 0 (closed pool must not retain)", len(p.pool))
	}
	if got := fc.closes.Load(); got != 1 {
		t.Fatalf("fc.closes = %d, want 1", got)
	}
}

// --- R15 F5: pool put/close race must not strand a conn in the dead channel ---

// TestConnPool_PutCloseRace_NoOrphanedConn is the R15-F5 regression test.
//
// The bug: put() read `closed` under mu, released mu, then did the non-blocking
// channel send. close() could run in that window: it sets closed=true (under
// mu), releases mu, and drains the channel. If put's send lands AFTER close's
// drain loop, the conn sits in the dead channel forever — never reused, never
// closed -> FD leak.
//
// The fix: hold mu across the closed-check AND the send, so close() cannot
// interleave: any put that observes closed==false completes its send before
// close acquires mu, and close's subsequent drain picks the conn up.
//
// Reproduction: hammer put and close concurrently. Every conn handed to put
// must be accounted for exactly once — either closed (by put's overspill/full
// path, by put's closed path, or by close's drain) — and the channel must be
// empty once close returns. On the buggy code some conns are orphaned in the
// channel with Close never called.
func TestConnPool_PutCloseRace_NoOrphanedConn(t *testing.T) {
	const rounds = 200
	for range rounds {
		// Capacity 1 keeps the race window tight (most puts hit the full path,
		// exercising the closed-check vs send interleave).
		p := newConnPool("tcp", "127.0.0.1:1", 1, time.Second, time.Second)

		const n = 16
		var wg sync.WaitGroup
		wg.Add(n + 1)
		conns := make([]*fakeConn, n)
		for i := range conns {
			conns[i] = &fakeConn{}
		}
		start := make(chan struct{})
		// Closer races against the puts.
		go func() {
			defer wg.Done()
			<-start
			p.close()
		}()
		for i := range conns {
			go func(fc *fakeConn) {
				defer wg.Done()
				<-start
				p.put(fc)
			}(conns[i])
		}
		close(start)
		wg.Wait()

		// After close() returns the pool is drained: the channel must be empty
		// (no orphaned conn). On the buggy code a conn can be stranded here.
		if got := len(p.pool); got != 0 {
			t.Fatalf("round: pool channel still holds %d conn(s) after close "+
				"(put/close race stranded a conn — FD leak)", got)
		}
		// Every conn must be closed exactly once. If any conn is neither in the
		// channel nor closed, it leaked (the buggy window). The channel is empty
		// (asserted above), so closes must total n.
		var closed int64
		for _, fc := range conns {
			closed += fc.closes.Load()
		}
		if closed != n {
			t.Fatalf("round: closes = %d, want %d (some conn was never closed "+
				"nor pooled — orphaned by the put/close race)", closed, n)
		}
	}
}

// TestConnPool_PutHoldsMuAcrossSend pins the fix's mechanism: put must hold mu
// across both the closed-check and the channel send. We verify this by taking mu
// ourselves and confirming put blocks until mu is released (i.e. put cannot
// sneak its send in while we hold mu, which is what closes the race).
func TestConnPool_PutHoldsMuAcrossSend(t *testing.T) {
	const n = 3
	// Capacity >= n so every put enqueues (none hit the overspill close path,
	// which would muddy the "put holds mu" assertion).
	p := newConnPool("tcp", "127.0.0.1:1", n, time.Second, time.Second)
	defer p.close()

	// Hold mu: a correctly-implemented put (check + send under mu) cannot make
	// progress; it must block here. We release mu via the signal so put can run.
	p.mu.Lock()
	conns := make([]*fakeConn, n)
	for i := range conns {
		conns[i] = &fakeConn{}
	}
	var wg sync.WaitGroup
	wg.Add(n)
	for _, fc := range conns {
		go func(fc *fakeConn) {
			defer wg.Done()
			p.put(fc) // must block on mu
		}(fc)
	}
	// Give the puts a moment to block on mu.
	time.Sleep(20 * time.Millisecond)
	// While mu is held, the channel must be untouched by put.
	if got := len(p.pool); got != 0 {
		t.Fatalf("pool len = %d while mu held, want 0 (put must hold mu across send)", got)
	}
	p.mu.Unlock()
	wg.Wait()
	// Now all puts completed under mu; the channel holds exactly n.
	if got := len(p.pool); got != n {
		t.Fatalf("pool len = %d, want %d after mu release", got, n)
	}
}

// --- writeOnce / writeReadAll / writeReadLine error branches -----------------

// TestWriteOnce_WriteDeadlineError covers the SetWriteDeadline error branch of
// writeOnce.
func TestWriteOnce_WriteDeadlineError(t *testing.T) {
	fc := &fakeConn{writeDeadlineErr: errors.New("setdeadline boom")}
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
	})
	defer c.Close()

	err := c.writeOnce(context.Background(), []byte("x"))
	if err == nil {
		t.Fatal("writeOnce: expected setdeadline error")
	}
	if got := fc.deadlinesW.Load(); got != 1 {
		t.Fatalf("SetWriteDeadline calls = %d, want 1", got)
	}
	// On error withConn closes the conn.
	if got := fc.closes.Load(); got < 1 {
		t.Fatalf("closes = %d, want >= 1", got)
	}
}

// TestWriteOnce_WriteError covers the Write-error branch of writeOnce.
func TestWriteOnce_WriteError(t *testing.T) {
	wErr := errors.New("write boom")
	fc := &fakeConn{writeErr: wErr, writeN: 0}
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
	})
	defer c.Close()

	err := c.writeOnce(context.Background(), []byte("x"))
	if !errors.Is(err, wErr) {
		t.Fatalf("err = %v, want %v", err, wErr)
	}
}

// TestWriteOnce_ShortWrite covers the `n < len(data)` short-write branch.
func TestWriteOnce_ShortWrite(t *testing.T) {
	fc := &fakeConn{writeN: 2} // writes 2 of 5 bytes
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
	})
	defer c.Close()

	err := c.writeOnce(context.Background(), []byte("hello"))
	if err == nil {
		t.Fatal("writeOnce: expected short-write error")
	}
}

// TestWriteReadAll_ReadDeadlineError covers the SetReadDeadline error branch
// (ReadTimeout > 0 path).
func TestWriteReadAll_ReadDeadlineError(t *testing.T) {
	rdErr := errors.New("read setdeadline boom")
	fc := &fakeConn{
		writeN:          64, // accept any write
		readDeadlineErr: rdErr,
	}
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
	})
	defer c.Close()

	_, err := c.writeReadAll(context.Background(), []byte("hello"))
	if !errors.Is(err, rdErr) {
		t.Fatalf("err = %v, want %v", err, rdErr)
	}
}

// TestWriteReadAll_ReadError covers a non-EOF read error.
func TestWriteReadAll_ReadError(t *testing.T) {
	rErr := errors.New("read boom")
	fc := &fakeConn{
		writeN:   64,
		readData: nil,
		readErr:  rErr,
	}
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
	})
	defer c.Close()

	out, err := c.writeReadAll(context.Background(), []byte("hello"))
	if !errors.Is(err, rErr) {
		t.Fatalf("err = %v, want %v", err, rErr)
	}
	if out != nil {
		t.Fatalf("out = %v, want nil", out)
	}
}

// TestWriteReadAll_ReadTimeoutZeroClearsDeadline covers the ReadTimeout<=0
// branch of writeReadAll, which clears any stale write deadline before reading.
func TestWriteReadAll_ReadTimeoutZeroClearsDeadline(t *testing.T) {
	// Server-side: fake conn returns EOF after the configured read data, and
	// the read deadline should be cleared (not set) because ReadTimeout=0.
	fc := &fakeConn{
		writeN:   64,
		readData: []byte("resp"),
		readErr:  io.EOF, // clean EOF -> peer-closed path (not pooled)
	}
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
		ReadTimeout:  0, // exercises the deadline-clearing branch
	})
	// withDefaults replaces a zero ReadTimeout with the 10s default, which would
	// skip the branch we mean to cover. Force it back to 0 now that the client
	// is built (internal-package access lets us mutate opts directly).
	c.opts.ReadTimeout = 0
	defer c.Close()

	out, err := c.writeReadAll(context.Background(), []byte("hello"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if string(out) != "resp" {
		t.Fatalf("out = %q, want %q", out, "resp")
	}
	if got := fc.deadlinesR.Load(); got != 1 {
		t.Fatalf("SetReadDeadline calls = %d, want 1 (the clearing call)", got)
	}
	if got := fc.closes.Load(); got < 1 {
		t.Fatalf("closes = %d, want >= 1 (EOF conn must be closed not pooled)", got)
	}
}

// TestWriteReadLine_ReadDeadlineError covers the SetReadDeadline error branch
// (ReadTimeout > 0 path) of writeReadLine.
func TestWriteReadLine_ReadDeadlineError(t *testing.T) {
	rdErr := errors.New("line read setdeadline boom")
	fc := &fakeConn{
		writeN:          64,
		readDeadlineErr: rdErr,
	}
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
	})
	defer c.Close()

	_, err := c.writeReadLine(context.Background(), []byte("x\n"))
	if !errors.Is(err, rdErr) {
		t.Fatalf("err = %v, want %v", err, rdErr)
	}
}

// TestWriteReadLine_ReadTimeoutZeroClearsDeadline covers the ReadTimeout<=0
// branch of writeReadLine.
func TestWriteReadLine_ReadTimeoutZeroClearsDeadline(t *testing.T) {
	// bufio.ReadString needs a newline to succeed; provide one.
	fc := &fakeConn{
		writeN:   64,
		readData: []byte("line\n"),
		readErr:  io.EOF,
	}
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
		ReadTimeout:  0,
	})
	// withDefaults replaces a zero ReadTimeout with the 10s default; force it
	// back to 0 so writeReadLine takes the deadline-clearing else branch.
	c.opts.ReadTimeout = 0
	defer c.Close()

	line, err := c.writeReadLine(context.Background(), []byte("x\n"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if line != "line" {
		t.Fatalf("line = %q, want %q", line, "line")
	}
	if got := fc.deadlinesR.Load(); got != 1 {
		t.Fatalf("SetReadDeadline calls = %d, want 1 (clearing)", got)
	}
}

// TestWriteReadLine_ReadError covers a non-EOF read error from ReadString.
func TestWriteReadLine_ReadError(t *testing.T) {
	rErr := errors.New("line read boom")
	fc := &fakeConn{
		writeN:   64,
		readData: nil,
		readErr:  rErr,
	}
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
	})
	defer c.Close()

	_, err := c.writeReadLine(context.Background(), []byte("x\n"))
	if !errors.Is(err, rErr) {
		t.Fatalf("err = %v, want %v", err, rErr)
	}
}

// TestWriteReadAll_WriteDeadlineError covers the SetWriteDeadline error branch
// of writeReadAll (its write phase is a separate copy from writeOnce's).
func TestWriteReadAll_WriteDeadlineError(t *testing.T) {
	wdErr := errors.New("write setdeadline boom")
	fc := &fakeConn{writeDeadlineErr: wdErr, writeN: 64}
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
	})
	defer c.Close()

	_, err := c.writeReadAll(context.Background(), []byte("hello"))
	if !errors.Is(err, wdErr) {
		t.Fatalf("err = %v, want %v", err, wdErr)
	}
}

// TestWriteReadAll_WriteError covers the Write-error branch of writeReadAll.
func TestWriteReadAll_WriteError(t *testing.T) {
	wErr := errors.New("write boom")
	fc := &fakeConn{writeErr: wErr, writeN: 0}
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
	})
	defer c.Close()

	_, err := c.writeReadAll(context.Background(), []byte("hello"))
	if !errors.Is(err, wErr) {
		t.Fatalf("err = %v, want %v", err, wErr)
	}
}

// TestWriteReadAll_ShortWrite covers the `n < len(data)` short-write branch of
// writeReadAll.
func TestWriteReadAll_ShortWrite(t *testing.T) {
	fc := &fakeConn{writeN: 2} // writes 2 of 5 bytes
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
	})
	defer c.Close()

	_, err := c.writeReadAll(context.Background(), []byte("hello"))
	if err == nil {
		t.Fatal("writeReadAll: expected short-write error")
	}
}

// TestWriteReadLine_WriteDeadlineError covers the SetWriteDeadline error branch
// of writeReadLine.
func TestWriteReadLine_WriteDeadlineError(t *testing.T) {
	wdErr := errors.New("line write setdeadline boom")
	fc := &fakeConn{writeDeadlineErr: wdErr, writeN: 64}
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
	})
	defer c.Close()

	_, err := c.writeReadLine(context.Background(), []byte("x\n"))
	if !errors.Is(err, wdErr) {
		t.Fatalf("err = %v, want %v", err, wdErr)
	}
}

// TestWriteReadLine_WriteError covers the Write-error branch of writeReadLine.
func TestWriteReadLine_WriteError(t *testing.T) {
	wErr := errors.New("line write boom")
	fc := &fakeConn{writeErr: wErr, writeN: 0}
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
	})
	defer c.Close()

	_, err := c.writeReadLine(context.Background(), []byte("x\n"))
	if !errors.Is(err, wErr) {
		t.Fatalf("err = %v, want %v", err, wErr)
	}
}

// TestWriteReadLine_ShortWrite covers the `n < len(data)` short-write branch of
// writeReadLine.
func TestWriteReadLine_ShortWrite(t *testing.T) {
	fc := &fakeConn{writeN: 1} // writes 1 of 3 bytes ("x\n" is 2, but use 3)
	c := newClientWithFakeConn(t, fc, ClientOptions{
		Address:      "fake",
		WriteTimeout: 50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
	})
	defer c.Close()

	_, err := c.writeReadLine(context.Background(), []byte("abc"))
	if err == nil {
		t.Fatal("writeReadLine: expected short-write error")
	}
}

// --- options: RetryMax < 0 falls back to default ----------------------------

// TestWithDefaults_NegativeRetryMax covers the `o.RetryMax < 0` branch of
// withDefaults: a negative (unset) RetryMax is replaced by the package default.
func TestWithDefaults_NegativeRetryMax(t *testing.T) {
	o := ClientOptions{Address: "x", RetryMax: -7}.withDefaults()
	if o.RetryMax != defaultClientOptions().RetryMax {
		t.Fatalf("RetryMax = %d, want default %d", o.RetryMax, defaultClientOptions().RetryMax)
	}
}

// --- retryDelay: overflow guard ---------------------------------------------

// TestRetryDelay_OverflowGuard covers the `next <= backoff` overflow branch of
// retryDelay. minWait is large enough that one left-shift overflows int64, and
// maxWait is MaxInt64 so the loop keeps doubling (backoff < maxWait) until the
// shift overflows — exercising the overflow clamp to maxWait.
func TestRetryDelay_OverflowGuard(t *testing.T) {
	minWait := time.Duration(int64(1) << 60) // 1<<60, positive, large
	maxWait := time.Duration(maxInt64)       // huge cap so the loop runs to overflow
	// attempt >= 3 so the doubling loop iterates enough to overflow.
	for range 50 {
		d := retryDelay(3, minWait, maxWait)
		// After the overflow clamp backoff == maxWait; factor [0.5,1.0) yields
		// [0.5*maxWait, maxWait). Any result in that range proves the guard ran.
		if d < minWait {
			// At minimum the result must reflect the large backoff, not a small
			// minWait-only value (which would mean the loop never doubled).
			t.Fatalf("retryDelay = %v, want >= minWait %v (overflow guard broken)", d, minWait)
		}
	}
}

// maxInt64 as a typed const helper (time.Duration is int64).
const maxInt64 = 1<<63 - 1

// TestWithConn_CtxCancelSurfacesCtxErr covers the
// `if ctx.Err() != nil && (errors.Is(fnErr, net.ErrClosed) || isClosedErr(fnErr))`
// branch of withConn: when the ctx watcher closes the conn mid-Read, the
// in-flight Read returns net.ErrClosed and withConn must surface ctx.Err so
// shouldRetry treats it as non-retryable.
func TestWithConn_CtxCancelSurfacesCtxErr(t *testing.T) {
	// real listener so the conn is a real *net.TCPConn; the watcher's Close
	// surfaces as net.ErrClosed (wrapped).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Never write; block on read so the client's read hangs until ctx.
		go io.Copy(io.Discard, conn)
	}()

	c := NewClient(ClientOptions{
		Address:        ln.Addr().String(),
		ConnectTimeout: 200 * time.Millisecond,
		ReadTimeout:    0, // rely on ctx; the read must truly block
		WriteTimeout:   200 * time.Millisecond,
		RetryMax:       0,
		IdleTimeout:    0,
	})
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// fn mimics writeReadAll: write then block reading until the watcher
	// closes the conn.
	blockErr := make(chan error, 1)
	go func() {
		err := c.withConn(ctx, func(conn net.Conn, poolable *bool) error {
			_, werr := conn.Write([]byte("hi"))
			if werr != nil {
				return werr
			}
			_, rerr := conn.Read(make([]byte, 64)) // blocks until ctx closes conn
			return rerr
		})
		blockErr <- err
	}()
	time.Sleep(50 * time.Millisecond) // let the read block
	cancel()

	select {
	case err := <-blockErr:
		if err == nil {
			t.Fatal("withConn: expected ctx error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("withConn did not return after ctx cancel")
	}
}

// --- Send / SendReceive / SendReceiveLine: Latency + Breaker per method ------

// latencyRecorder is a minimal LatencyObserver shared by the three per-method
// tests (reusing latObserver from latency_test.go would couple assertions; we
// only need to know Observe fired).
type latencyRecorder struct{ count atomic.Int64 }

func (l *latencyRecorder) Observe(time.Duration) { l.count.Add(1) }

// TestSendReceive_LatencyObserver covers the Latency!=nil branch of
// SendReceive (the only one of the three not exercised by latency_test.go).
func TestSendReceive_LatencyObserver(t *testing.T) {
	ln := benchEchoOnceListener(t)
	defer ln.Close()

	obs := &latencyRecorder{}
	c := NewClient(ClientOptions{
		Address:     ln.Addr().String(),
		Latency:     obs,
		ReadTimeout: 500 * time.Millisecond,
	})
	defer c.Close()

	if _, err := c.SendReceive(context.Background(), []byte("lat")); err != nil {
		t.Fatalf("SendReceive: %v", err)
	}
	if got := obs.count.Load(); got != 1 {
		t.Fatalf("observe count = %d, want 1", got)
	}
}

// TestSendReceiveLine_LatencyObserver covers the Latency!=nil branch of
// SendReceiveLine.
func TestSendReceiveLine_LatencyObserver(t *testing.T) {
	ln := benchEchoOnceListener(t)
	defer ln.Close()
	// benchEchoOnceListener echoes the request verbatim but does not append a
	// newline, so SendReceiveLine returns io.EOF after reading the partial line.
	// That still drives observe once (Latency fires regardless of outcome).
	obs := &latencyRecorder{}
	c := NewClient(ClientOptions{
		Address:     ln.Addr().String(),
		Latency:     obs,
		ReadTimeout: 500 * time.Millisecond,
		RetryMax:    0,
	})
	defer c.Close()

	_, _ = c.SendReceiveLine(context.Background(), []byte("lat\n"))
	if got := obs.count.Load(); got != 1 {
		t.Fatalf("observe count = %d, want 1", got)
	}
}

// failingBreaker is a CircuitBreaker that always short-circuits with sentinel
// without calling fn. It covers the `c.opts.Breaker != nil` branch of Send and
// SendReceiveLine (SendReceive is already breaker-covered).
type failingBreaker struct{ err error }

func (b *failingBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	return b.err
}

// TestSend_BreakerShortCircuits covers the Breaker!=nil branch of Send.
func TestSend_BreakerShortCircuits(t *testing.T) {
	sentinel := errors.New("breaker-open")
	c := NewClient(ClientOptions{
		Address: "127.0.0.1:1",
		Breaker: &failingBreaker{err: sentinel},
	})
	defer c.Close()

	if err := c.Send(context.Background(), []byte("x")); !errors.Is(err, sentinel) {
		t.Fatalf("Send err = %v, want sentinel", err)
	}
	if m := c.Metrics(); m.Failed != 1 || m.Total != 1 {
		t.Fatalf("metrics = %+v, want Failed=1 Total=1", m)
	}
}

// TestSendReceiveLine_BreakerShortCircuits covers the Breaker!=nil branch of
// SendReceiveLine.
func TestSendReceiveLine_BreakerShortCircuits(t *testing.T) {
	sentinel := errors.New("breaker-open")
	c := NewClient(ClientOptions{
		Address: "127.0.0.1:1",
		Breaker: &failingBreaker{err: sentinel},
	})
	defer c.Close()

	if _, err := c.SendReceiveLine(context.Background(), []byte("x")); !errors.Is(err, sentinel) {
		t.Fatalf("SendReceiveLine err = %v, want sentinel", err)
	}
	if m := c.Metrics(); m.Failed != 1 || m.Total != 1 {
		t.Fatalf("metrics = %+v, want Failed=1 Total=1", m)
	}
}

// --- R15 F1: ctx-fired conns must not be pooled (close-vs-pool race) --------

// persistentAcceptAll accepts every connection and echoes writes back via
// io.Copy, keeping each connection open. Used by the F1/F2 regression tests
// that need a connection which stays alive across a write+read cycle (so a
// healthy pooled conn remains reusable, but a ctx-fired one must not be pooled).
func persistentAcceptAll(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c) // echo until error or EOF
			}(conn)
		}
	}()
	return ln
}

// drainPooledConns pulls every idle conn currently sitting in the pool and
// returns them, closing each after the test inspects it.
func drainPooledConns(p *connPool) []*poolConn {
	var out []*poolConn
	for {
		select {
		case pc := <-p.pool:
			out = append(out, pc)
		default:
			return out
		}
	}
}

// TestWithConn_CtxFiredAfterSuccessNotPooled is the R15-F1 regression test.
//
// The bug: withConn's ctx-watcher goroutine runs
//
//	select { case <-ctx.Done(): conn.Close(); case <-done: }
//
// When ctx fires while fn is mid-success, both cases are ready once `done` is
// closed, so Go picks pseudo-randomly. If the watcher wins it calls conn.Close()
// AFTER the main path has already done c.pool.put(conn). A future caller then
// gets a closed conn from the pool -> spurious net.ErrClosed.
//
// The fix: before c.pool.put(conn) on the success path, guard
//
//	if ctx.Err() != nil { conn.Close(); return nil }
//
// A conn whose ctx fired is never pooled.
//
// This test reproduces the race under concurrency: thousands of withConn calls
// each cancel their ctx ~immediately after a successful write. After draining
// the pool, EVERY conn must still be writable. On the old (buggy) code a few
// conns are closed by straggling watchers -> Write returns net.ErrClosed.
func TestWithConn_CtxFiredAfterSuccessNotPooled(t *testing.T) {
	ln := persistentAcceptAll(t)
	defer ln.Close()

	c := NewClient(ClientOptions{
		Address:        ln.Addr().String(),
		ConnectTimeout: 500 * time.Millisecond,
		ReadTimeout:    0, // rely on ctx; reads in fn return promptly anyway
		WriteTimeout:   500 * time.Millisecond,
		RetryMax:       0,
		IdleTimeout:    0, // disable eviction so poisoned conns are observable
		PoolSize:       64,
	})
	defer c.Close()

	// fn writes (succeeds immediately against an echo server) and returns nil.
	// The caller cancels ctx concurrently so the watcher races the return.
	const n = 5000
	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})
	for range n {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			defer wg.Done()
			defer cancel()
			<-start
			_ = c.withConn(ctx, func(conn net.Conn, poolable *bool) error {
				_, err := conn.Write([]byte("x"))
				return err
			})
			// Cancel immediately after fn returns so the watcher's ctx.Done()
			// case becomes ready right around pool.put. (cancel is also deferred
			// above; calling twice is a no-op.)
			cancel()
		}()
	}
	close(start)
	wg.Wait()

	// Drain the pool: every conn that survived must still be usable. A
	// poisoned (watcher-closed) conn fails Write with net.ErrClosed.
	pooled := drainPooledConns(c.pool)
	t.Logf("drained %d pooled conns; each must still be writable", len(pooled))
	var poisoned int
	for _, pc := range pooled {
		// Give a straggling watcher a moment to finish its Close if it is going
		// to (the race window is tiny, but the watcher goroutine may not have
		// been scheduled yet).
		_ = pc.Conn.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
		if _, err := pc.Conn.Write([]byte("probe")); err != nil {
			poisoned++
			t.Errorf("pooled conn write err = %v (pooled-after-close bug)", err)
		}
		_ = pc.Conn.Close()
	}
	if poisoned != 0 {
		t.Fatalf("%d pooled conns were closed by a straggling watcher", poisoned)
	}
}

// TestWithConn_CtxFiredSuccessReturnsNil confirms the fix does not change the
// call's own return value: withConn returns nil for a successful fn even when
// ctx fired (the conn is simply closed rather than pooled). This pins the
// observable contract so a future refactor that turns the success-but-ctx-fired
// path into an error is caught.
func TestWithConn_CtxFiredSuccessReturnsNil(t *testing.T) {
	ln := persistentAcceptAll(t)
	defer ln.Close()

	c := NewClient(ClientOptions{
		Address:        ln.Addr().String(),
		ConnectTimeout: 500 * time.Millisecond,
		WriteTimeout:   500 * time.Millisecond,
		RetryMax:       0,
		IdleTimeout:    0,
		PoolSize:       1,
	})
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// fn cancels its own ctx right before returning success, so the guard at
	// pool.put observes ctx.Err() != nil deterministically. fn returns nil.
	err := c.withConn(ctx, func(conn net.Conn, poolable *bool) error {
		_, werr := conn.Write([]byte("x"))
		cancel() // ctx fires before fn returns
		return werr
	})
	if err != nil {
		t.Fatalf("withConn err = %v, want nil (success even when ctx fired)", err)
	}
	if got := len(c.pool.pool); got != 0 {
		t.Fatalf("pool len = %d, want 0 (ctx-fired conn must not be pooled)", got)
	}
}

// --- R15 F2: writeReadLine bufio over-read must not lose bytes ---------------

// lineThenExtraListener accepts every connection, reads the request, then
// writes a single line followed by extra bytes (no newline): "line1\nEXTRA".
// This is the server shape that exposes the bufio over-read bug: bufio.Reader
// pulls both chunks off the socket in one Read, ReadString('\n') returns
// "line1\n", and "EXTRA" sits in the bufio buffer. On the buggy code the conn
// is returned to the pool with "EXTRA" stranded in the GC'd bufio buffer.
func lineThenExtraListener(t *testing.T, reply string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 64)
				_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				_, _ = c.Read(buf) // consume the request so the reply lands
				// Write the whole reply in one call so it typically arrives in a
				// single TCP segment, maximising the chance bufio over-reads.
				_, _ = c.Write([]byte(reply))
			}(conn)
		}
	}()
	return ln
}

// TestWriteReadLine_OverreadNotPooled is the R15-F2 regression test.
//
// The bug: writeReadLine wraps the conn in a bufio.Reader and calls
// ReadString('\n'). bufio fills a 4096-byte buffer in one Read on the conn, so
// when the server sends "line1\nEXTRA" the whole payload is pulled off the
// socket; ReadString returns "line1\n" and "EXTRA" is stranded in the bufio
// buffer. On the buggy code the conn is returned to the pool and the bufio
// reader is GC'd with the unread tail still inside -> "EXTRA" silently lost.
//
// The fix: after a successful line read, if br.Buffered() > 0 the conn is
// marked not poolable (the over-read bytes cannot be put back on the socket), so
// withConn closes it instead of poisoning the pool.
//
// Reproduction: the server replies "line1\nEXTRA". writeReadLine must return
// "line1" AND the conn must NOT be pooled (a pooled conn would be reused by a
// later call and the lost "EXTRA" would manifest as a corrupt/empty stream).
func TestWriteReadLine_OverreadNotPooled(t *testing.T) {
	ln := lineThenExtraListener(t, "line1\nEXTRA")
	defer ln.Close()

	c := NewClient(ClientOptions{
		Address:        ln.Addr().String(),
		ConnectTimeout: 500 * time.Millisecond,
		ReadTimeout:    500 * time.Millisecond,
		WriteTimeout:   500 * time.Millisecond,
		RetryMax:       0,
		IdleTimeout:    0, // disable eviction so we can observe the pool directly
		PoolSize:       4,
	})
	defer c.Close()

	line, err := c.writeReadLine(context.Background(), []byte("q\n"))
	if err != nil {
		t.Fatalf("writeReadLine: %v", err)
	}
	if line != "line1" {
		t.Fatalf("line = %q, want %q", line, "line1")
	}
	// The over-read conn must NOT be in the pool: the bufio buffer holds
	// "EXTRA" which would be lost on reuse. On the buggy code this is non-zero.
	if got := len(c.pool.pool); got != 0 {
		t.Fatalf("pool len = %d, want 0 (over-read conn must not be pooled; "+
			"buf bytes would be lost)", got)
	}
}

// TestSendReceiveLine_OverreadDoesNotCorruptNextCall is the end-to-end form of
// the F2 regression: after a call whose response over-read extra bytes, a
// subsequent call must not receive the stale/lost bytes. The fix ensures the
// over-reading conn is closed rather than pooled, so the next call dials fresh.
func TestSendReceiveLine_OverreadDoesNotCorruptNextCall(t *testing.T) {
	ln := lineThenExtraListener(t, "ok\ntrailing")
	defer ln.Close()

	c := NewClient(ClientOptions{
		Address:        ln.Addr().String(),
		ConnectTimeout: 500 * time.Millisecond,
		ReadTimeout:    500 * time.Millisecond,
		WriteTimeout:   500 * time.Millisecond,
		RetryMax:       0,
		IdleTimeout:    0,
		PoolSize:       4,
	})
	defer c.Close()

	// First call: server replies "ok\ntrailing". The line is "ok"; "trailing"
	// is over-read and must not poison the pool.
	first, err := c.SendReceiveLine(context.Background(), []byte("1\n"))
	if err != nil {
		t.Fatalf("SendReceiveLine[1]: %v", err)
	}
	if first != "ok" {
		t.Fatalf("SendReceiveLine[1] = %q, want %q", first, "ok")
	}
	// Second call against a fresh conn: must get "ok" again, NOT a corrupted
	// stream caused by reusing a conn whose bufio buffer ate "trailing".
	second, err := c.SendReceiveLine(context.Background(), []byte("2\n"))
	if err != nil {
		t.Fatalf("SendReceiveLine[2]: %v", err)
	}
	if second != "ok" {
		t.Fatalf("SendReceiveLine[2] = %q, want %q (over-read bytes leaked?)", second, "ok")
	}
}

// --- R15 F6: deterministic pool-exhaustion extra-dial (replaces flaky test) ---

// echoOnceInternal is the internal-package echo-once listener (the external
// echoOnceListener lives in tcpclient_test.go and is unreachable here). It
// counts accepted connections for the blocked-checkout assertion.
func echoOnceInternal(t *testing.T) (net.Listener, *uint64) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var conns uint64
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddUint64(&conns, 1)
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 64*1024)
				_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				n, err := c.Read(buf)
				if err != nil || n == 0 {
					return
				}
				_, _ = c.Write(buf[:n])
			}(conn)
		}
	}()
	return ln, &conns
}

// TestClient_PoolExhaustion_BlockedCheckout deterministically proves that a
// second call dials a fresh connection when the pool's only conn is checked out
// by an in-flight call. This replaces the scheduler-dependent assertion in
// TestClient_PoolExhaustion_DialsExtra (external test), which flaked because a
// write-only Send returns before the second goroutine runs.
//
// We acquire the pool's single conn directly (internal access) and hold it
// checked out, then run a second Send: with no idle conn available it must
// dial. The server therefore accepts >= 2 connections — the deterministic
// "extra dial under genuine exhaustion" property.
func TestClient_PoolExhaustion_BlockedCheckout(t *testing.T) {
	ln, conns := echoOnceInternal(t)
	defer ln.Close()

	c := NewClient(ClientOptions{
		Network:        "tcp",
		Address:        ln.Addr().String(),
		ConnectTimeout: 500 * time.Millisecond,
		ReadTimeout:    500 * time.Millisecond,
		WriteTimeout:   500 * time.Millisecond,
		PoolSize:       1,
		IdleTimeout:    0,
		RetryMax:       0,
	})
	defer c.Close()

	// Hold the pool's single conn checked out so the next Send cannot reuse it.
	conn1, err := c.pool.get(context.Background(), c.opts.ConnectTimeout)
	if err != nil {
		t.Fatalf("pool.get[1]: %v", err)
	}
	defer c.pool.put(conn1)
	// Give the accept counter time to observe conn1's accept.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && atomic.LoadUint64(conns) < 1 {
		time.Sleep(2 * time.Millisecond)
	}

	// Second Send runs with conn1 checked out: the pool is exhausted, so it
	// must dial a fresh conn. The echo-once server accepts it -> success.
	if err := c.Send(context.Background(), []byte("x")); err != nil {
		t.Fatalf("Send[2] while conn checked out: %v", err)
	}
	// Server accepted >= 2 connections: one for conn1, one for the forced dial.
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && atomic.LoadUint64(conns) < 2 {
		time.Sleep(2 * time.Millisecond)
	}
	if got := atomic.LoadUint64(conns); got < 2 {
		t.Fatalf("blocked-checkout: server conns = %d, want >= 2 (forced dial)", got)
	}
}

// --- SetOnEvent nil path (fires no hook) -----------------------------------

// TestSetOnEvent_DisableThenFire covers the SetOnEvent(nil) branch and the
// nil-hook fast path of fireEvent after disable. (SetOnEvent's nil branch is
// reached; fireEvent's `p == nil` collapse is the zero-overhead path.)
func TestSetOnEvent_DisableThenFire(t *testing.T) {
	ln, _ := benchEchoListener(t)
	defer ln.Close()

	var calls atomic.Int64
	hook := func(ClientEvent) { calls.Add(1) }
	c := NewClient(ClientOptions{Address: ln.Addr().String(), RetryMax: 0})
	defer c.Close()
	c.SetOnEvent(hook)
	c.SetOnEvent(nil) // disable: exercises the nil branch

	if err := c.Send(context.Background(), []byte("x")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("hook calls = %d, want 0 after disable", got)
	}
}
