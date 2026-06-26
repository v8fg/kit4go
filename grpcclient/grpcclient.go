package grpcclient

import (
	"context"
	"errors"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
)

// ClientMetrics is a point-in-time snapshot of the counters maintained by a
// [Middleware]. Values are gathered via atomic loads and may be slightly
// inconsistent with one another under concurrent load; that is acceptable for
// monitoring/observability use.
type ClientMetrics struct {
	// Total is the number of RPCs observed, regardless of outcome. Incremented
	// once per logical call (retries do not inflate this counter).
	Total uint64

	// Success is the number of calls whose final attempt returned a nil error
	// (gRPC status OK).
	Success uint64

	// Failed is the number of calls that did not end in OK: this covers both
	// non-OK final statuses and any error that prevented the RPC from running.
	Failed uint64

	// Retried is the total number of retry attempts made (not counting the
	// initial attempt). A call that required two retries contributes 2 here.
	// Only unary RPCs are retried, so stream calls never inflate this counter.
	Retried uint64
}

// ClientEvent is passed to the hook installed via [Middleware.SetOnEvent] for
// every notable outcome of an RPC lifecycle. It is the integration point for
// metrics push (Prometheus counters/histograms, tracing spans, log4go alerts),
// mirroring the hook pattern used by the httpclient and breaker packages.
//
// Name is one of:
//   - "request": an attempt was sent (one per send, including retries).
//   - "retry":   an attempt failed with a retryable code and will be retried
//     (fires before backoff).
//   - "success": the call completed with a nil error (status OK).
//   - "failed":  the call completed with a non-OK status, or could not be sent
//     (breaker open, context cancelled, etc.).
//
// Method is the full gRPC method name (e.g. "/pkg.Service/Method"). Code is the
// gRPC status code name of the relevant attempt (e.g. "Unavailable"); empty
// when no status was obtained. Attempt is the 0-indexed attempt number this
// event pertains to (0 = the initial send).
type ClientEvent struct {
	Name    string
	Method  string
	Code    string
	Attempt int
}

// Client holds the shared counters and event hook for a [Middleware]. It is a
// distinct type from Middleware so the metrics/hook surface is named clearly
// and can be reused independently of the interceptor wiring. All fields are
// accessed atomically; the zero value is a ready-to-use metrics sink.
type Client struct {
	opts ClientOptions

	// Counters are laid out as separate atomics rather than a single packed
	// struct so increments don't contend on the same cache line.
	total   atomic.Uint64
	success atomic.Uint64
	failed  atomic.Uint64
	retried atomic.Uint64

	// onEvent, when non-nil, is invoked for every notable RPC outcome (request,
	// retry, success, failed). Set via SetOnEvent and read with an atomic load,
	// so the default (nil) is zero-overhead on the hot path.
	onEvent atomic.Pointer[func(ClientEvent)]
}

// SetOnEvent installs a hook invoked for every notable RPC lifecycle event. fn
// receives a [ClientEvent] describing the attempt and its outcome. Pass nil to
// disable a previously-installed hook.
//
// The hook is intended for metrics/tracing and must be cheap and non-blocking:
// it fires synchronously on the goroutine issuing the RPC. Install it once at
// construction time (before traffic) for the cleanest ordering; SetOnEvent is
// nevertheless safe to call concurrently with in-flight RPCs.
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
func (c *Client) fireEvent(name, method, codeName string, attempt int) {
	if p := c.onEvent.Load(); p != nil {
		(*p)(ClientEvent{
			Name:    name,
			Method:  method,
			Code:    codeName,
			Attempt: attempt,
		})
	}
}

// Metrics returns a point-in-time snapshot of the middleware's counters.
func (c *Client) Metrics() ClientMetrics {
	return ClientMetrics{
		Total:   c.total.Load(),
		Success: c.success.Load(),
		Failed:  c.failed.Load(),
		Retried: c.retried.Load(),
	}
}

// Middleware bundles a [Client] (metrics + event hook) with the frozen options
// used to build its interceptors. Construct one with [NewMiddleware] and pass
// [Middleware.UnaryClientInterceptor] / [Middleware.StreamClientInterceptor] to
// grpc.Dial. The interceptors share the same underlying Client, so metrics
// reflect every call regardless of which interceptor handled it.
type Middleware struct {
	opts    ClientOptions
	metrics *Client
}

// SetOnEvent installs a hook invoked for every notable RPC lifecycle event on
// the middleware's underlying Client. See [Client.SetOnEvent].
func (m *Middleware) SetOnEvent(fn func(evt ClientEvent)) { m.metrics.SetOnEvent(fn) }

// Metrics returns a point-in-time snapshot of the middleware's counters. See
// [Client.Metrics].
func (m *Middleware) Metrics() ClientMetrics { return m.metrics.Metrics() }

// NewMiddleware constructs a [Middleware] from opts, filling zero fields with
// the package defaults. The returned middleware owns its metrics counters and a
// fresh [Client]; pass its interceptors to grpc.Dial:
//
//	mw := grpcclient.NewMiddleware(opts)
//	conn, err := grpc.Dial(opts.Target,
//	    grpc.WithTransportCredentials(insecure.NewCredentials()),
//	    grpc.WithUnaryInterceptor(mw.UnaryClientInterceptor()),
//	    grpc.WithStreamInterceptor(mw.StreamClientInterceptor()),
//	)
func NewMiddleware(opts ClientOptions) *Middleware {
	opts = opts.withDefaults()
	return &Middleware{
		opts:    opts,
		metrics: &Client{opts: opts},
	}
}

// DialConn is a convenience that dials opts.Target with the middleware's
// interceptors already wired in, using insecure transport credentials and
// non-blocking dial with the configured ConnectTimeout. It returns a
// *grpc.ClientConn ready for stub construction; the caller must Close it.
//
// This helper uses insecure credentials (suited to in-cluster mTLS-stripped or
// local dev traffic). For TLS or custom dial options, construct the middleware
// with [NewMiddleware] and dial manually.
func DialConn(opts ClientOptions) (*grpc.ClientConn, error) {
	mw := NewMiddleware(opts)
	o := mw.opts
	if o.Target == "" {
		return nil, errEmptyTarget
	}
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(mw.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(mw.StreamClientInterceptor()),
		grpc.WithConnectParams(grpc.ConnectParams{
			// MinConnectTimeout bounds how long a single connection attempt
			// may take; ConnectTimeout on our options is the source of truth.
			MinConnectTimeout: o.ConnectTimeout,
			// Keep the default backoff strategy but bound it so a flapping
			// server doesn't hammer reconnects.
			Backoff: backoff.DefaultConfig,
		}),
	}
	// grpc.Dial is deprecated in favour of grpc.NewClient, but NewClient
	// changes the default resolver/blocking semantics. Dial with
	// WithBlock omitted (non-blocking) keeps the wrapper's behaviour stable and
	// simple; callers wanting blocking dial can build their own middleware.
	return grpc.Dial(o.Target, dialOpts...)
}

// withTimeout applies the per-RPC RequestTimeout to ctx only when the caller's
// ctx lacks a deadline; this lets callers impose tighter deadlines than the
// configured RequestTimeout without being overridden. Returns the (possibly
// wrapped) ctx and a cancel func the caller must defer.
func (m *Middleware) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDL := ctx.Deadline(); !hasDL && m.opts.RequestTimeout > 0 {
		return context.WithTimeout(ctx, m.opts.RequestTimeout)
	}
	return context.WithCancel(ctx)
}

// errEmptyTarget is returned by [DialConn] when ClientOptions.Target is empty.
var errEmptyTarget = errors.New("grpcclient: empty Target in ClientOptions")
