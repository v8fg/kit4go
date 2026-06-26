package tcpclient

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// rngMu guards rngFloat64. A *rand.Rand is NOT safe for concurrent use, so the
// shared source is guarded by this mutex. The contention is negligible because
// retries are rare relative to the call volume, and the critical section is a
// single Float64. gosec's G404 rule is disabled for this package in
// .golangci.yml.
var rngMu sync.Mutex

// rng is the shared source of jitter for retryDelay. math/rand is good enough
// here — we want spread, not cryptographic unpredictability.
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// rngFloat64 returns a random float in [0.0, 1.0) under the rng mutex.
func rngFloat64() float64 {
	rngMu.Lock()
	defer rngMu.Unlock()
	return rng.Float64()
}

// shouldRetry reports whether a network error warrants a retry. Retryable:
// transport-layer timeouts (dial/read/write), connection refused/reset,
// unexpected EOF, and bare connection-closed errors. Not retryable: context
// cancellation or deadline exhaustion originating from the caller's ctx
// (retrying would just blow past the same deadline again).
func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	// Context errors belong to the caller; never retry them.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Transient read errors (peer closed mid-message) are retryable.
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	// Transport-layer timeouts (dial/read/write) are retryable on a fresh
	// attempt — distinct from context deadlines above. Guard against the rare
	// wrapper that surfaces a context error through net.Error.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		if errors.Is(netErr, context.Canceled) || errors.Is(netErr, context.DeadlineExceeded) {
			return false
		}
		return true
	}
	// Any net.OpError (connection refused / reset / broken pipe) is retryable.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr, context.Canceled) || errors.Is(opErr, context.DeadlineExceeded) {
			return false
		}
		return true
	}
	// Fallback: treat any remaining non-context, non-nil error as retryable.
	// This catches syscall-level connection refused / broken pipe errors that
	// do not wrap net.OpError on every platform.
	return true
}

// retryDelay computes the delay before the (attempt)-th retry using exponential
// backoff with full jitter. attempt is 0-indexed (0 = delay before the first
// retry). If minWait <= 0 or maxWait <= 0 the result is 0, so a caller who
// disables backoff gets immediate retries.
func retryDelay(attempt int, minWait, maxWait time.Duration) time.Duration {
	if minWait <= 0 || maxWait <= 0 {
		return 0
	}
	backoff := int64(minWait)
	for i := 0; i < attempt && backoff < int64(maxWait); i++ {
		next := backoff << 1
		if next <= backoff { // overflow guard
			backoff = int64(maxWait)
			break
		}
		backoff = next
	}
	if backoff > int64(maxWait) {
		backoff = int64(maxWait)
	}
	factor := 0.5 + rngFloat64()*0.5 // [0.5, 1.0)
	return time.Duration(float64(backoff) * factor)
}

// ClientMetrics is a point-in-time snapshot of the counters maintained by a
// [Client]. Values are gathered via atomic loads and may be slightly
// inconsistent with one another under concurrent load; that is acceptable for
// monitoring/observability use.
type ClientMetrics struct {
	// Total is the number of calls observed, regardless of outcome.
	Total uint64

	// Success is the number of calls that completed without error.
	Success uint64

	// Failed is the number of calls that ended in an error.
	Failed uint64

	// Retried is the total number of retry attempts made (not counting the
	// initial attempt). A call that required two retries contributes 2 here.
	Retried uint64

	// ActiveConn is the number of in-flight connections at snapshot time —
	// connections currently checked out of the pool and doing I/O. Read via
	// an atomic load, so zero-contention and safe to scrape on the hot path.
	ActiveConn int32

	// PoolSize is the current depth of the idle-connection pool (the number of
	// idle connections waiting to be reused). It is the channel depth at the
	// instant of the snapshot and may change as connections are checked out
	// or returned.
	PoolSize int
}

// ClientEvent is passed to the hook installed via [Client.SetOnEvent] for every
// notable outcome of a call lifecycle. It is the integration point for metrics
// push (Prometheus counters/histograms, tracing spans, etc.), mirroring the
// hook pattern used by the httpclient and breaker packages.
//
// Name is one of:
//   - "connect": a connection was obtained (pooled or freshly dialled).
//   - "send":    a write attempt was made (one per attempt, including retries).
//   - "receive": a read attempt was made.
//   - "retry":   an attempt failed and will be retried (fires before backoff).
//   - "success": the call completed without error.
//   - "failed":  the call completed with an error, or could not be sent.
//
// Attempt is the 0-indexed attempt number (0 = the initial send). Bytes is the
// number of bytes written ("send") or read ("receive") on this attempt, or 0
// for the other event kinds.
type ClientEvent struct {
	Name    string
	Addr    string
	Bytes   int
	Attempt int
}

// Client is a production-grade TCP/Unix-socket client with connection pooling,
// per-operation timeouts, retry and optional circuit-breaker integration. The
// zero value is not usable; construct one with [NewClient].
//
// All methods are safe for concurrent use by multiple goroutines.
type Client struct {
	opts ClientOptions
	pool *connPool

	// Counters are separate atomics (not a packed struct) so increments don't
	// contend on the same cache line.
	total   atomic.Uint64
	success atomic.Uint64
	failed  atomic.Uint64
	retried atomic.Uint64

	// activeConn is the number of connections currently checked out of the
	// pool and doing I/O. Incremented in withConn before the per-attempt I/O
	// runs and decremented via defer afterwards, so a snapshot (atomic load)
	// reports the in-flight depth at zero contention.
	activeConn atomic.Int32

	// onEvent, when non-nil, is invoked for every notable call-lifecycle event.
	// Set via SetOnEvent and read with an atomic load, so the default (nil) is
	// zero-overhead on the hot path.
	onEvent atomic.Pointer[func(ClientEvent)]

	// closeOnce makes Close idempotent.
	closeOnce sync.Once
}

// SetOnEvent installs a hook invoked for every notable call-lifecycle event.
// fn receives a [ClientEvent] describing the attempt and its outcome. Pass nil
// to disable a previously-installed hook.
//
// The hook fires synchronously on the goroutine issuing the call, so it must
// be cheap and non-blocking. SetOnEvent is safe to call concurrently with
// in-flight requests; install once at construction time for the cleanest
// ordering.
func (c *Client) SetOnEvent(fn func(ClientEvent)) {
	if fn == nil {
		c.onEvent.Store(nil)
		return
	}
	f := fn // copy to heap
	c.onEvent.Store(&f)
}

// fireEvent is the single chokepoint for hook dispatch. When onEvent is nil
// (the default) the call collapses to a single nil compare.
func (c *Client) fireEvent(name string, bytes, attempt int) {
	if p := c.onEvent.Load(); p != nil {
		(*p)(ClientEvent{
			Name:    name,
			Addr:    c.opts.Address,
			Bytes:   bytes,
			Attempt: attempt,
		})
	}
}

// observe records the call's end-to-end latency on the configured observer. It
// assumes c.opts.Latency is non-nil — callers guard the start/defer behind a
// nil check so the disabled path is a single branch with no time.Now and no
// defer.
func (c *Client) observe(start time.Time) {
	c.opts.Latency.Observe(time.Since(start))
}
// package defaults and wiring up a connection pool sized by PoolSize. The
// returned client is safe for concurrent use and ready to serve traffic.
func NewClient(opts ClientOptions) *Client {
	opts = opts.withDefaults()
	return &Client{
		opts: opts,
		pool: newConnPool(opts.Network, opts.Address, opts.PoolSize, opts.ConnectTimeout, opts.IdleTimeout),
	}
}

// Send writes data to the connection and returns without reading a response.
// It applies WriteTimeout, retry on transient network errors (up to
// [ClientOptions.RetryMax]), and circuit-breaker integration when a Breaker is
// configured. A pooled connection is used when available and returned to the
// pool afterwards. Metrics (total/success/failed/retried) are updated.
func (c *Client) Send(ctx context.Context, data []byte) error {
	c.total.Add(1)
	if c.opts.Latency != nil {
		start := time.Now()
		defer c.observe(start)
	}

	doFn := func(ctx context.Context) error {
		return c.DoWithRetry(ctx, func(ctx context.Context) error {
			return c.writeOnce(ctx, data)
		})
	}

	var err error
	if c.opts.Breaker != nil {
		err = c.opts.Breaker.Execute(ctx, doFn)
	} else {
		err = doFn(ctx)
	}

	if err != nil {
		c.failed.Add(1)
		c.fireEvent("failed", 0, 0)
		return err
	}
	c.success.Add(1)
	c.fireEvent("success", len(data), 0)
	return nil
}

// SendReceive writes data then reads the full response until the ReadTimeout
// deadline elapses or the peer closes the connection. It applies WriteTimeout
// on the write and ReadTimeout on the read, retries transient network errors
// up to RetryMax, and funnels through the Breaker when configured. The reply
// bytes are returned; a nil slice with a nil error means the peer sent nothing
// before closing.
func (c *Client) SendReceive(ctx context.Context, data []byte) ([]byte, error) {
	c.total.Add(1)
	if c.opts.Latency != nil {
		start := time.Now()
		defer c.observe(start)
	}

	var resp []byte
	doFn := func(ctx context.Context) error {
		return c.DoWithRetry(ctx, func(ctx context.Context) error {
			var rErr error
			resp, rErr = c.writeReadAll(ctx, data)
			return rErr
		})
	}

	var err error
	if c.opts.Breaker != nil {
		err = c.opts.Breaker.Execute(ctx, doFn)
	} else {
		err = doFn(ctx)
	}

	if err != nil {
		c.failed.Add(1)
		c.fireEvent("failed", 0, 0)
		return nil, err
	}
	c.success.Add(1)
	c.fireEvent("success", len(resp), 0)
	return resp, nil
}

// SendReceiveLine writes data then reads a single line (terminated by '\n',
// newline excluded from the result). It is a thin convenience over
// SendReceive for line-oriented protocols (RESP-like, debug shells, etc.). If
// the peer closes without emitting a newline, the partial bytes accumulated so
// far are returned together with io.EOF.
func (c *Client) SendReceiveLine(ctx context.Context, data []byte) (string, error) {
	c.total.Add(1)
	if c.opts.Latency != nil {
		start := time.Now()
		defer c.observe(start)
	}

	var line string
	doFn := func(ctx context.Context) error {
		return c.DoWithRetry(ctx, func(ctx context.Context) error {
			var lErr error
			line, lErr = c.writeReadLine(ctx, data)
			return lErr
		})
	}

	var err error
	if c.opts.Breaker != nil {
		err = c.opts.Breaker.Execute(ctx, doFn)
	} else {
		err = doFn(ctx)
	}

	if err != nil {
		c.failed.Add(1)
		c.fireEvent("failed", 0, 0)
		return line, err
	}
	c.success.Add(1)
	c.fireEvent("success", len(line), 0)
	return line, nil
}

// DoWithRetry runs fn up to RetryMax+1 times, retrying whenever fn returns a
// transient network error (see [shouldRetry]). Between attempts it sleeps for a
// jittered exponential backoff bounded by [RetryWaitMin, RetryWaitMax], honouring
// ctx cancellation during the wait. Context cancellations are never retried.
//
// fn is the caller's per-attempt operation; it receives ctx and must use it for
// any blocking work. fn is responsible for obtaining (and returning or
// closing) its own connection, typically via the unexported withConn helper —
// this keeps each retry on a fresh connection, which is what makes retry useful
// for a reset connection.
func (c *Client) DoWithRetry(ctx context.Context, fn func(ctx context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
	for attempt := 0; attempt <= c.opts.RetryMax; attempt++ {
		err = fn(ctx)
		if !shouldRetry(err) {
			return err
		}
		// Last allowed attempt: stop without backing off.
		if attempt == c.opts.RetryMax {
			return err
		}
		c.retried.Add(1)
		c.fireEvent("retry", 0, attempt+1)

		delay := retryDelay(attempt, c.opts.RetryWaitMin, c.opts.RetryWaitMax)
		select {
		case <-ctx.Done():
			// Cancelled while waiting: surface the last transport error if one
			// is present, otherwise the ctx error.
			if err == nil {
				err = ctx.Err()
			}
			return err
		case <-time.After(delay):
		}
	}
	return err
}

// withConn obtains a pooled connection, runs fn with it, and returns it to the
// pool when fn signals it is still healthy. fn receives the connection and a
// *bool poolable flag (initialised to true) which it sets to false when it
// detects the peer has half-closed the connection (e.g. io.ReadAll returned
// because of EOF) — such a connection must not be reused, so withConn closes
// it instead of returning it to the pool.
//
// On any error from fn the connection is closed (it may be in a half-broken
// state) and the error is returned, translated to the ctx error when the
// failure was caused by ctx cancellation tearing the connection down (so
// shouldRetry treats it as non-retryable).
func (c *Client) withConn(ctx context.Context, fn func(conn net.Conn, poolable *bool) error) error {
	conn, err := c.pool.get(ctx, c.opts.ConnectTimeout)
	if err != nil {
		return err
	}
	c.activeConn.Add(1)
	defer c.activeConn.Add(-1)
	c.fireEvent("connect", 0, 0)

	// Raw net.Conn reads/writes are not woken by context cancellation — only
	// by deadlines or Close. To honour ctx for blocking reads (when the caller
	// sets ReadTimeout=0 and relies on ctx, or to cut a hung peer early), we
	// race fn against ctx.Done(): if the context fires first we Close the
	// connection, which unblocks any in-flight Read/Write with a "use of
	// closed network connection" error. done is closed when fn returns so the
	// watcher knows to stop.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	poolable := true
	fnErr := fn(conn, &poolable)
	close(done)

	if fnErr != nil {
		// On error the connection may be in a half-broken state; close it
		// rather than returning a bad conn to the pool. Close is idempotent so
		// the watcher's potential Close above is harmless.
		_ = conn.Close()
		// If fn failed solely because we tore the connection down on ctx
		// cancellation, surface the ctx error so shouldRetry treats it as
		// non-retryable.
		if ctx.Err() != nil && (errors.Is(fnErr, net.ErrClosed) || isClosedErr(fnErr)) {
			return ctx.Err()
		}
		return fnErr
	}
	if !poolable {
		// fn signalled the peer half-closed (e.g. clean EOF after a response):
		// close rather than poison the pool with a one-shot connection.
		_ = conn.Close()
		return nil
	}
	c.pool.put(conn)
	return nil
}

// isClosedErr reports whether err is (or wraps) a "use of closed network
// connection" error, which is how an in-flight Read/Write surfaces a
// connection torn down by the ctx watcher. errors.Is(net.ErrClosed) covers the
// Go-tracked path; the string fallback covers the syscall variant.
func isClosedErr(err error) bool {
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	return err != nil && err.Error() == "use of closed network connection"
}

// writeOnce performs a single write of data on a pooled connection with the
// configured WriteTimeout. It is the per-attempt body of Send. A write-only
// call leaves the connection poolable (the peer has not signalled close).
func (c *Client) writeOnce(ctx context.Context, data []byte) error {
	return c.withConn(ctx, func(conn net.Conn, poolable *bool) error {
		if c.opts.WriteTimeout > 0 {
			if err := conn.SetWriteDeadline(time.Now().Add(c.opts.WriteTimeout)); err != nil {
				return err
			}
		}
		n, err := conn.Write(data)
		c.fireEvent("send", n, 0)
		if err != nil {
			return err
		}
		if n < len(data) {
			return fmt.Errorf("tcpclient: short write: %d/%d bytes", n, len(data))
		}
		return nil
	})
}

// readAllUntilEOF reads from conn into a buffer one chunk at a time until the
// peer closes (io.EOF) or an error occurs. It returns the accumulated bytes and
// a flag indicating whether the read terminated in a clean EOF (peer closed),
// in which case the caller must not return the connection to the pool.
//
// We do NOT use io.ReadAll because it swallows the EOF that lets us distinguish
// "peer closed" (don't pool) from "read stopped for another reason".
func readAllUntilEOF(conn net.Conn) ([]byte, bool, error) {
	var out []byte
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err != nil {
			if err == io.EOF {
				return out, true, nil
			}
			return out, false, err
		}
	}
}

// writeReadAll writes data then reads the full reply, applying WriteTimeout on
// the write and ReadTimeout on the read. The connection is returned to the pool
// only when the read did NOT end in a clean EOF; a peer-closed connection is
// closed instead so the pool is never poisoned with a one-shot socket.
func (c *Client) writeReadAll(ctx context.Context, data []byte) ([]byte, error) {
	var out []byte
	err := c.withConn(ctx, func(conn net.Conn, poolable *bool) error {
		if c.opts.WriteTimeout > 0 {
			if err := conn.SetWriteDeadline(time.Now().Add(c.opts.WriteTimeout)); err != nil {
				return err
			}
		}
		n, err := conn.Write(data)
		c.fireEvent("send", n, 0)
		if err != nil {
			return err
		}
		if n < len(data) {
			return fmt.Errorf("tcpclient: short write: %d/%d bytes", n, len(data))
		}
		// Reset deadlines for the read phase. A zero ReadTimeout leaves the
		// connection with no read deadline (caller-imposed ctx only).
		if c.opts.ReadTimeout > 0 {
			if err := conn.SetReadDeadline(time.Now().Add(c.opts.ReadTimeout)); err != nil {
				return err
			}
		} else {
			// Clear any stale write deadline so a subsequent pooled read isn't
			// prematurely timed out by it.
			_ = conn.SetReadDeadline(time.Time{})
		}
		buf, eof, rErr := readAllUntilEOF(conn)
		c.fireEvent("receive", len(buf), 0)
		if rErr != nil {
			return rErr
		}
		out = buf
		// A clean EOF means the peer closed: do not pool this connection.
		*poolable = !eof
		return nil
	})
	return out, err
}

// writeReadLine writes data then reads up to the first '\n' using a bufio
// reader. The trailing newline is stripped from the returned string. If the
// peer closes before a newline, the partial bytes are returned with io.EOF and
// the connection is closed (not pooled), since the peer has half-closed.
func (c *Client) writeReadLine(ctx context.Context, data []byte) (string, error) {
	var line string
	err := c.withConn(ctx, func(conn net.Conn, poolable *bool) error {
		if c.opts.WriteTimeout > 0 {
			if err := conn.SetWriteDeadline(time.Now().Add(c.opts.WriteTimeout)); err != nil {
				return err
			}
		}
		n, err := conn.Write(data)
		c.fireEvent("send", n, 0)
		if err != nil {
			return err
		}
		if n < len(data) {
			return fmt.Errorf("tcpclient: short write: %d/%d bytes", n, len(data))
		}
		if c.opts.ReadTimeout > 0 {
			if err := conn.SetReadDeadline(time.Now().Add(c.opts.ReadTimeout)); err != nil {
				return err
			}
		} else {
			_ = conn.SetReadDeadline(time.Time{})
		}
		// bufio.Reader is allocated per call: keeping it would require pooling
		// the buffered-but-unread bytes across calls, which is unsafe for a
		// pooled raw connection. One allocation per SendReceiveLine is fine.
		br := bufio.NewReader(conn)
		s, rErr := br.ReadString('\n')
		c.fireEvent("receive", len(s), 0)
		line = trimNewline(s)
		if rErr != nil {
			// EOF (with or without partial data) means the peer closed: don't
			// pool the connection. Other read errors also close via the error
			// path in withConn.
			if rErr == io.EOF || errors.Is(rErr, io.EOF) {
				*poolable = false
			}
			return rErr
		}
		return nil
	})
	return line, err
}

// trimNewline strips a single trailing '\n' (and an optional '\r' before it)
// from s. Used by writeReadLine to return the line content without the
// delimiter.
func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// Metrics returns a point-in-time snapshot of the client's counters.
func (c *Client) Metrics() ClientMetrics {
	return ClientMetrics{
		Total:      c.total.Load(),
		Success:    c.success.Load(),
		Failed:     c.failed.Load(),
		Retried:    c.retried.Load(),
		ActiveConn: c.activeConn.Load(),
		PoolSize:   len(c.pool.pool),
	}
}

// Close drains the connection pool and marks the client closed. It is
// idempotent. In-flight calls are unaffected; connections held by callers are
// closed by whoever holds them (the pool only owns idle connections). After
// Close, new calls will still dial fresh connections (rather than fail), but
// callers should treat a closed client as retired.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.pool.close()
	})
	return nil
}
