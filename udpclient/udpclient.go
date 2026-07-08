package udpclient

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"sync/atomic"
	"time"
)

// retryDelay uses math/rand/v2's top-level Float64 for jitter — concurrent-safe
// and auto-seeded. A shared *rand.Rand (math/rand) is NOT safe for concurrent
// use and would race on the retry hot path.

// errClosed is returned (wrapped) by Send/SendReceive after [Client.Close] has
// been called. It is not retryable.
var errClosed = errors.New("udpclient: connection closed")

// ClientMetrics is a point-in-time snapshot of the counters maintained by a
// [Client]. Values are gathered via atomic loads and may be slightly
// inconsistent with one another under concurrent load; that is acceptable for
// monitoring/observability use.
type ClientMetrics struct {
	// Total is the number of calls observed, regardless of outcome.
	Total uint64

	// Success is the number of calls that completed without error.
	Success uint64

	// Failed is the number of calls that ended in an error (after all retries).
	Failed uint64

	// Retried is the total number of retry attempts made (not counting the
	// initial attempt). A call that required two retries contributes 2 here.
	Retried uint64

	// ActiveSends is the number of in-flight datagram calls currently doing
	// socket I/O at snapshot time. UDP is connectionless so there is no
	// pool/active-connection analog; this counter tracks the depth of
	// concurrent writers/readers. Read via atomic load, zero-contention.
	ActiveSends int32
}

// ClientEvent is passed to the hook installed via [Client.SetOnEvent] for every
// notable outcome of a call lifecycle. It is the integration point for metrics
// push (Prometheus counters/histograms, tracing spans, etc.), mirroring the
// hook pattern used by the httpclient and breaker packages.
//
// Name is one of:
//   - "send":    a datagram attempt was written (one per attempt, including
//     retries). For SendReceive this fires at the write stage.
//   - "receive": a reply datagram was read (SendReceive only).
//   - "retry":   an attempt failed and will be retried (fires before backoff).
//   - "success": the call completed without error.
//   - "failed":  the call completed with an error after all retries, or could
//     not be sent at all.
//
// Addr is the remote peer the client is connected to. Bytes is the number of
// bytes written (for "send") or read (for "receive"); 0 otherwise. Attempt is
// the 0-indexed attempt number this event pertains to (0 = the initial send).
type ClientEvent struct {
	Name string
	Addr string
	// Bytes is populated for "send" and "receive" events; 0 otherwise.
	Bytes int
	// Attempt is the 0-indexed attempt number (0 = initial send).
	Attempt int
}

// Client is a production-grade UDP client wrapping a connected [net.UDPConn]
// with per-call read/write timeouts, retry with backoff and optional
// circuit-breaker integration. The zero value is not usable; construct one with
// [NewClient].
//
// All methods are safe for concurrent use by multiple goroutines. The
// underlying socket and its deadlines are shared across goroutines, so under
// concurrency SendReceive in particular contends on the read path; for
// high-throughput request/reply fan-out prefer one client per goroutine.
type Client struct {
	conn *net.UDPConn
	opts ClientOptions

	// Counters are laid out as separate atomics rather than a single packed
	// struct so increments don't contend on the same cache line.
	total   atomic.Uint64
	success atomic.Uint64
	failed  atomic.Uint64
	retried atomic.Uint64

	// activeSends is the number of in-flight datagram calls (Send or
	// SendReceive) currently doing socket I/O. UDP is connectionless so there
	// is no pool/active-connection analog; instead this counter tracks the
	// depth of concurrent writers/readers. Read via atomic load for a
	// zero-contention snapshot.
	activeSends atomic.Int32

	// closed is set by Close so subsequent calls fail fast without touching the
	// (possibly nil) conn. Read atomically; written once under no lock since
	// Close is expected to be called once at teardown.
	closed atomic.Bool

	// onEvent, when non-nil, is invoked for every notable call outcome (send,
	// receive, retry, success, failed). Set via SetOnEvent and read with an
	// atomic load, so the default (nil) is zero-overhead on the hot path.
	onEvent atomic.Pointer[func(ClientEvent)]
}

// SetOnEvent installs a hook invoked for every notable call lifecycle event. fn
// receives a [ClientEvent] describing the attempt and its outcome. Pass nil to
// disable a previously-installed hook.
//
// The hook is intended for metrics/tracing and must be cheap and non-blocking:
// it fires synchronously on the goroutine issuing the call. Install it once at
// construction time (before traffic) for the cleanest ordering; SetOnEvent is
// nevertheless safe to call concurrently with in-flight calls.
func (c *Client) SetOnEvent(fn func(evt ClientEvent)) {
	if fn == nil {
		c.onEvent.Store(nil)
		return
	}
	f := fn // copy to heap
	c.onEvent.Store(&f)
}

// fireEvent is the single chokepoint for hook dispatch. When onEvent is nil
// (the default) the call collapses to a single nil compare, so the no-hook hot
// path is unaffected.
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

// NewClient constructs a [Client] from opts, filling zero fields with the
// package defaults. It resolves the remote (and optional local) address, binds
// the local socket if requested, and "connects" the UDP socket to the remote
// peer so reads only accept datagrams from that peer.
//
// opts.Address is required; an empty Address yields an error. The returned
// client is safe for concurrent use and ready to serve traffic. Close it with
// [Client.Close] to release the underlying file descriptor.
func NewClient(opts ClientOptions) (*Client, error) {
	opts = opts.withDefaults()
	if opts.Address == "" {
		return nil, errors.New("udpclient: Address is required")
	}

	raddr, err := net.ResolveUDPAddr("udp", opts.Address)
	if err != nil {
		return nil, fmt.Errorf("udpclient: resolve remote address %q: %w", opts.Address, err)
	}

	var laddr *net.UDPAddr
	if opts.LocalAddress != "" {
		laddr, err = net.ResolveUDPAddr("udp", opts.LocalAddress)
		if err != nil {
			return nil, fmt.Errorf("udpclient: resolve local address %q: %w", opts.LocalAddress, err)
		}
	}

	// DialUDP gives a "connected" UDP socket: writes go to raddr and reads only
	// accept datagrams whose source matches raddr. This is far simpler to drive
	// than an unconnected ListenUDP socket (no per-call ReadFromUDP bookkeeping)
	// and is the right primitive for a single-peer client.
	conn, err := net.DialUDP("udp", laddr, raddr)
	if err != nil {
		return nil, fmt.Errorf("udpclient: dial udp %q: %w", opts.Address, err)
	}

	return &Client{
		conn: conn,
		opts: opts,
	}, nil
}

// Send writes a single datagram to the connected peer. It does not wait for a
// reply — fire-and-forget telemetry/statsd-style traffic. WriteTimeout bounds
// the write; on a transient write error (or a write timeout) the call is
// retried up to RetryMax times with exponential backoff. If a [CircuitBreaker]
// is configured the whole attempt loop is wrapped in Breaker.Execute.
//
// data is written verbatim. A nil or empty data is a valid (if useless) write
// of a zero-length datagram; the kernel will deliver it.
func (c *Client) Send(ctx context.Context, data []byte) error {
	if c.closed.Load() {
		return errClosed
	}
	c.total.Add(1)
	c.activeSends.Add(1)
	defer c.activeSends.Add(-1)

	if ctx == nil {
		ctx = context.Background()
	}

	doFn := func(ctx context.Context) error {
		return c.sendWithRetry(ctx, data)
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

// SendReceive writes a single datagram and then reads exactly one reply
// datagram from the connected peer, returning the reply bytes. WriteTimeout
// bounds the write; ReadTimeout bounds the read. On any transient error (write
// or read, including a read timeout) the whole write+read exchange is retried
// up to RetryMax times with exponential backoff — a fresh write gives the peer
// another chance to reply. If a [CircuitBreaker] is configured the whole
// attempt loop is wrapped in Breaker.Execute.
//
// The reply is truncated to [ClientOptions.BufferSize] by the kernel; datagrams
// larger than that are silently dropped past the buffer. The returned slice is
// owned by the caller; it is a fresh allocation (not aliased to any internal
// buffer) so it is safe to retain and mutate.
//
// Concurrency: SendReceive shares one connected socket across goroutines, so
// concurrent SendReceive calls can interleave reads and a reply may be
// attributed to the wrong request (UDP has no request-id correlation at the
// socket). For request/response under concurrency, give each goroutine its own
// Client or serialize SendReceive. Send (fire-and-forget) is safe to share.
func (c *Client) SendReceive(ctx context.Context, data []byte) ([]byte, error) {
	if c.closed.Load() {
		return nil, errClosed
	}
	c.total.Add(1)
	c.activeSends.Add(1)
	defer c.activeSends.Add(-1)

	if ctx == nil {
		ctx = context.Background()
	}

	var reply []byte
	doFn := func(ctx context.Context) error {
		var err error
		reply, err = c.sendReceiveWithRetry(ctx, data)
		return err
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
	c.fireEvent("success", len(reply), 0)
	return reply, nil
}

// sendWithRetry writes data up to RetryMax+1 times, applying WriteTimeout per
// attempt and exponential backoff between retries. Context cancellation is
// honoured during backoff. errClosed is never retried.
func (c *Client) sendWithRetry(ctx context.Context, data []byte) error {
	var err error
	for attempt := 0; attempt <= c.opts.RetryMax; attempt++ {
		if c.closed.Load() {
			return errClosed
		}

		if werr := c.conn.SetWriteDeadline(time.Now().Add(c.opts.WriteTimeout)); werr != nil {
			// A failed SetWriteDeadline usually means the conn is closed; not
			// worth retrying.
			return werr
		}
		n, werr := c.conn.Write(data)
		// Fire a "send" event for every attempt, successful or not, so observers
		// can attribute every datagram write. Bytes is the count actually
		// written (0 on error).
		c.fireEvent("send", n, attempt)

		err = werr
		if werr == nil {
			return nil
		}
		if !shouldRetry(werr) || attempt == c.opts.RetryMax {
			return err
		}
		if !c.retry(ctx, attempt, n) {
			// ctx cancelled during backoff: surface the last error.
			return err
		}
	}
	return err
}

// sendReceiveWithRetry performs the write+read exchange up to RetryMax+1 times.
// ReadTimeout bounds the read; a read timeout is retryable (the peer may simply
// have dropped the datagram). Context cancellation is honoured during backoff.
// errClosed is never retried.
//
// On a retryable write or read error the loop records the failure, fires a
// "retry" event, sleeps for the backoff and starts a fresh attempt; a fresh
// write gives the peer another chance to reply. On a non-retryable error, or
// once the retry budget is spent, the last error is surfaced.
func (c *Client) sendReceiveWithRetry(ctx context.Context, data []byte) ([]byte, error) {
	buf := make([]byte, c.opts.BufferSize)
	var (
		err  error // last error observed (write or read)
		last int   // byte count associated with err, for the retry event
	)
	for attempt := 0; attempt <= c.opts.RetryMax; attempt++ {
		if c.closed.Load() {
			return nil, errClosed
		}

		// Write the request datagram.
		if werr := c.conn.SetWriteDeadline(time.Now().Add(c.opts.WriteTimeout)); werr != nil {
			// A failed SetWriteDeadline usually means the conn is closed; not
			// worth retrying.
			return nil, werr
		}
		wn, werr := c.conn.Write(data)
		c.fireEvent("send", wn, attempt)
		if werr != nil {
			err, last = werr, wn
			if !shouldRetry(werr) || attempt == c.opts.RetryMax {
				return nil, err
			}
			if !c.retry(ctx, attempt, last) {
				return nil, err
			}
			continue
		}

		// Read exactly one reply datagram. The bare conn.Read is a blocking
		// syscall that ignores ctx: a ctx cancel mid-read would otherwise block
		// until ReadTimeout fires. readCtx races the read against ctx.Done() and,
		// on ctx cancel, resets the read deadline to now to force the in-flight
		// Read to return promptly, then surfaces ctx.Err().
		rn, rerr := c.readCtx(ctx, buf)
		c.fireEvent("receive", rn, attempt)
		if rerr == nil {
			// Hand the caller a right-sized slice that does not alias buf.
			out := make([]byte, rn)
			copy(out, buf[:rn])
			return out, nil
		}
		// If the read failed solely because ctx was cancelled (we poked the
		// deadline to force it), surface ctx.Err() so shouldRetry treats it as
		// terminal rather than retrying a torn-down call.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		err, last = rerr, rn
		if !shouldRetry(rerr) || attempt == c.opts.RetryMax {
			return nil, err
		}
		if !c.retry(ctx, attempt, last) {
			return nil, err
		}
	}
	return nil, err
}

// readCtx sets the per-attempt read deadline (ReadTimeout) and performs a single
// Read on the shared connected socket, honouring ctx.
//
// A raw conn.Read is a blocking syscall that nothing but a deadline or socket
// close can interrupt. The retry/backoff sleep already honours ctx, but a Read
// blocked waiting for a datagram from a silent peer would otherwise ignore ctx
// and block the full ReadTimeout — so cancelling ctx mid-read returned ~5s late
// (the default) instead of promptly. Unlike tcpclient (which has a per-call
// pooled conn it can Close to unblock the Read), udpclient owns one shared
// connected socket that cannot be closed per call. The minimal correct fix is
// to race the Read against ctx.Done(): when ctx fires first we reset the read
// deadline to now, which forces the in-flight Read to return immediately with a
// deadline-exceeded error, then the caller surfaces ctx.Err(). The watcher is
// torn down (done closed) as soon as Read returns so a slow ctx cancel arriving
// after a successful read cannot corrupt a subsequent attempt's deadline.
//
// If ReadTimeout <= 0 the deadline is cleared (an unbounded read); ctx cancel
// still unblocks it via the deadline-reset path.
func (c *Client) readCtx(ctx context.Context, buf []byte) (int, error) {
	deadline := time.Now().Add(c.opts.ReadTimeout)
	if c.opts.ReadTimeout <= 0 {
		deadline = time.Time{} // no deadline
	}
	if err := c.conn.SetReadDeadline(deadline); err != nil {
		return 0, err
	}

	done := make(chan struct{})
	c.ctxWatcher(ctx, done)

	n, err := c.conn.Read(buf)
	close(done)
	return n, err
}

// ctxWatcher runs a goroutine that, on ctx.Done(), resets the client's shared
// read deadline to now so any in-flight Read on the socket returns promptly. It
// stops once done is closed (the Read returned). done MUST be closed exactly
// once by readCtx after the Read completes.
//
// Resetting the deadline (rather than closing the shared socket) is the right
// primitive for a connected UDP socket shared across goroutines: a Close would
// kill every other in-flight caller. The deadline-reset only nudges Reads that
// are actually blocked; a later attempt re-establishes its own deadline, so the
// transient "now" deadline leaves no lasting damage.
func (c *Client) ctxWatcher(ctx context.Context, done <-chan struct{}) {
	go func() {
		select {
		case <-ctx.Done():
			// Force the blocked Read to return. The caller will see a deadline
			// error and, because ctx.Err() != nil, surface ctx.Err() instead.
			_ = c.conn.SetReadDeadline(time.Now())
		case <-done:
			// Read already returned; nothing to interrupt.
		}
	}()
}

// retry is the shared retry tail for sendWithRetry and sendReceiveWithRetry: it
// bumps the retried counter, fires a "retry" event for attempt+1, then sleeps
// for the backoff computed from attempt. It returns false if ctx was cancelled
// during the backoff sleep (in which case the caller surfaces its last error
// rather than looping again), and true otherwise.
func (c *Client) retry(ctx context.Context, attempt, bytes int) bool {
	c.retried.Add(1)
	c.fireEvent("retry", bytes, attempt+1)
	return c.sleep(ctx, attempt)
}

// sleep pauses for the retry backoff computed by retryDelay, honouring ctx
// cancellation. It returns false if ctx was cancelled (in which case the caller
// should surface the last error rather than retry), and true otherwise.
func (c *Client) sleep(ctx context.Context, attempt int) bool {
	delay := retryDelay(attempt, c.opts.RetryWaitMin, c.opts.RetryWaitMax)
	if delay <= 0 {
		select {
		default:
			return true
		case <-ctx.Done():
			return false
		}
	}
	select {
	case <-time.After(delay):
		return true
	case <-ctx.Done():
		return false
	}
}

// shouldRetry reports whether err warrants another attempt. UDP being
// connectionless, the retryable set is intentionally broad: read/write timeouts
// (the kernel reported a deadline expiry), any net.OpError (transient syscall
// failure), and unexpected EOF are all retried. Context cancellation and the
// client's own errClosed are never retried — the former means the caller tore
// down the call and the latter means the socket is gone for good.
func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errClosed) {
		return false
	}
	// Context errors are terminal: Canceled means the caller tore down the
	// call; DeadlineExceeded means the caller's own ctx deadline is exhausted
	// and a retry would just blow past it again. (The retryable "timeout" case
	// is a transport-layer net.Error timeout from SetReadDeadline/SetWriteDeadline,
	// handled below — that is per-attempt, not per-call.)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// A transport-layer timeout (read/write deadline expiry) is the canonical
	// UDP retry signal: the datagram was lost or the peer didn't answer.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	// Any remaining net.OpError (e.g. connection refused on a connected UDP
	// socket sending to a port with no listener on some platforms) is retryable.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	// Fallback: treat unknown non-context errors as retryable. This mirrors the
	// httpclient policy and keeps a transient syscall hiccup from failing a call
	// that a fresh attempt would have survived.
	return true
}

// retryDelay computes the delay before the (attempt+1)-th retry using
// exponential backoff with full jitter:
//
//	base   = min(maxWait, minWait * 2^attempt)
//	delay  = base * (0.5 + random(0, 0.5))   // jitter band [0.5*base, base)
//
// attempt is 0-indexed: the delay before the first retry (after the initial
// attempt fails) is computed with attempt=0. The jitter decorrelates retries
// across instances so a thundering herd does not form against a recovering
// downstream.
//
// If minWait <= 0 or maxWait <= 0 the result is 0, so a caller who disables
// backoff by zeroing both waits gets immediate retries.
func retryDelay(attempt int, minWait, maxWait time.Duration) time.Duration {
	if minWait <= 0 || maxWait <= 0 {
		return 0
	}
	// Cap the exponential at maxWait to avoid unbounded growth on a large
	// RetryMax. minWait * 2^attempt, clamped.
	backoff := int64(minWait)
	// Guard against overflow on very large attempt counts: stop doubling once we
	// exceed maxWait.
	for i := 0; i < attempt && backoff < int64(maxWait); i++ {
		next := backoff << 1
		// If shifting overflowed (went negative or wrapped), clamp to maxWait.
		if next <= backoff {
			backoff = int64(maxWait)
			break
		}
		backoff = next
	}
	if backoff > int64(maxWait) {
		backoff = int64(maxWait)
	}
	// Jitter: multiply by a factor in [0.5, 1.0).
	// rng.Float64()*0.5 gives [0.0, 0.5); add 0.5 for [0.5, 1.0).
	factor := 0.5 + rand.Float64()*0.5
	return time.Duration(float64(backoff) * factor)
}

// Metrics returns a point-in-time snapshot of the client's counters.
func (c *Client) Metrics() ClientMetrics {
	return ClientMetrics{
		Total:       c.total.Load(),
		Success:     c.success.Load(),
		Failed:      c.failed.Load(),
		Retried:     c.retried.Load(),
		ActiveSends: c.activeSends.Load(),
	}
}

// Close releases the underlying UDP socket. After Close, Send and SendReceive
// return errClosed without touching the socket. It is safe to call Close more
// than once; subsequent calls are no-ops.
func (c *Client) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil // already closed
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
