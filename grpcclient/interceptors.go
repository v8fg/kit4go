package grpcclient

import (
	"context"
	"math/rand"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// rng is the shared source of jitter for retryDelay. math/rand is good enough
// here — we want spread, not cryptographic unpredictability — and gosec's G404
// rule is disabled for this package in .golangci.yml.
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// UnaryClientInterceptor returns a [grpc.UnaryClientInterceptor] that applies,
// in order:
//
//  1. Per-RPC timeout: RequestTimeout is applied via context.WithTimeout when
//     the caller's ctx has no deadline.
//  2. Retry: if the RPC returns a status code in RetryCodes and the attempt is
//     below RetryMax, sleep with exponential backoff + jitter and retry.
//  3. Circuit breaker: if Breaker is configured, the whole retry loop is run
//     inside Breaker.Execute so an open breaker short-circuits the call.
//  4. Metrics + event hooks: total/success/failed/retried counters and the
//     SetOnEvent callback fire at the relevant points.
//
// Retries are NOT attempted when the caller's context is cancelled or hits its
// deadline: a DeadlineExceeded caused by the per-RPC timeout is retried only if
// the underlying transport returned it (via a gRPC Unavailable/DeadlineExceeded
// status), not when ctx.Err() fires.
func (m *Middleware) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		m.metrics.total.Add(1)
		if m.opts.Latency != nil {
			start := time.Now()
			defer m.observe(start)
		}
		m.metrics.active.Add(1)
		defer m.metrics.active.Add(-1)

		// Per-RPC timeout (only when the caller's ctx lacks a deadline). The
		// cancel is owned by this interceptor and torn down after the loop.
		rpcCtx, cancel := m.withTimeout(ctx)
		defer cancel()

		// The retry loop lives inside a closure so the breaker (when present)
		// wraps the whole loop as one logical call.
		run := func(callCtx context.Context) error {
			var err error
			for attempt := 0; attempt <= m.opts.RetryMax; attempt++ {
				err = invoker(callCtx, method, req, reply, cc, opts...)

				codeName := codeNameOf(err)
				m.metrics.fireEvent("request", method, codeName, attempt)

				// No error → success, stop.
				if err == nil {
					return nil
				}

				// Context cancellation/deadline from the caller or our timeout
				// is never retried: the deadline is exhausted, so another
				// attempt would blow straight past it again. Surface the error
				// as-is.
				if callCtx.Err() != nil {
					return err
				}

				// Non-retryable status → surface as-is.
				st, ok := status.FromError(err)
				if !ok || !m.opts.retryable(st.Code()) {
					return err
				}

				// Retryable status, but no attempts left: surface the last
				// error rather than retrying forever.
				if attempt == m.opts.RetryMax {
					return err
				}

				// More attempts remain: count the retry, fire the event, then
				// back off honouring ctx cancellation.
				m.metrics.retried.Add(1)
				m.metrics.fireEvent("retry", method, codeName, attempt+1)

				delay := retryDelay(attempt, m.opts.RetryWaitMin, m.opts.RetryWaitMax)
				if delay > 0 {
					select {
					case <-callCtx.Done():
						return callCtx.Err()
					case <-time.After(delay):
					}
				}
			}
			return err
		}

		var err error
		if m.opts.Breaker != nil {
			err = m.opts.Breaker.Execute(rpcCtx, run)
		} else {
			err = run(rpcCtx)
		}

		if err != nil {
			m.metrics.failed.Add(1)
			m.metrics.fireEvent("failed", method, codeNameOf(err), 0)
			return err
		}
		m.metrics.success.Add(1)
		m.metrics.fireEvent("success", method, "", 0)
		return nil
	}
}

// StreamClientInterceptor returns a [grpc.StreamClientInterceptor] that applies
// the per-RPC timeout and (if configured) circuit-breaker integration. It does
// NOT retry: retrying a stream is semantically unsafe (the server may have
// already started emitting messages), so a caller whose stream errored must
// reconnect explicitly.
func (m *Middleware) StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		m.metrics.total.Add(1)
		if m.opts.Latency != nil {
			start := time.Now()
			defer m.observe(start)
		}
		m.metrics.active.Add(1)
		defer m.metrics.active.Add(-1)

		rpcCtx, cancel := m.withTimeout(ctx)

		// The stream creation (and subsequent Recv/Send) happens outside the
		// interceptor's stack frame, so we cannot simply defer cancel. Instead
		// we wrap the returned stream so cancel fires when the caller finishes
		// the stream (Recv returns io.EOF or an error). gRPC guarantees the
		// context is no longer needed after the stream ends.
		streamer2 := streamer
		open := func(callCtx context.Context) (grpc.ClientStream, error) {
			s, e := streamer2(callCtx, desc, cc, method, opts...)
			if e != nil {
				return nil, e
			}
			return &cancelOnDoneStream{ClientStream: s, cancel: cancel}, nil
		}

		var (
			stream grpc.ClientStream
			err    error
		)
		if m.opts.Breaker != nil {
			// The breaker only sees stream *open* failures — once the stream is
			// open, per-message failures are the caller's problem. This matches
			// how a breaker treats a long-lived connection: the dial is the
			// unit of failure, not each read.
			err = m.opts.Breaker.Execute(rpcCtx, func(callCtx context.Context) error {
				stream, err = open(callCtx)
				return err
			})
		} else {
			stream, err = open(rpcCtx)
		}

		if err != nil {
			cancel()
			m.metrics.failed.Add(1)
			m.metrics.fireEvent("failed", method, codeNameOf(err), 0)
			return nil, err
		}
		m.metrics.success.Add(1)
		m.metrics.fireEvent("success", method, "", 0)
		return stream, nil
	}
}

// cancelOnDoneStream wraps a [grpc.ClientStream] so the per-RPC cancel func
// (from context.WithTimeout) is invoked exactly once when the stream
// terminates. gRPC surfaces stream termination as Recv returning io.EOF (clean
// half-close from the server) or a non-EOF error; we hook both.
type cancelOnDoneStream struct {
	grpc.ClientStream
	cancel context.CancelFunc
	done   bool
}

// RecvMsg forwards to the underlying stream and, on the terminal io.EOF /
// error, releases the per-RPC timeout context. We hook RecvMsg rather than
// Header/SendMsg because RecvMsg is the only method that observes the server's
// half-close (io.EOF); SendMsg-only clients are rare and the context is bounded
// by RequestTimeout regardless.
func (s *cancelOnDoneStream) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if s.done {
		return err
	}
	if err != nil {
		s.done = true
		s.cancel()
	}
	return err
}

// retryDelay computes the delay before the (attempt+1)-th retry using
// exponential backoff with full jitter:
//
//	base   = min(maxWait, minWait * 2^attempt)
//	delay  = base * (0.5 + random(0, 0.5))   // jitter band [0.5*base, base)
//
// attempt is 0-indexed: the delay before the first retry (after the initial
// call fails) is computed with attempt=0. The jitter decorrelates retries
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
	factor := 0.5 + rng.Float64()*0.5
	return time.Duration(float64(backoff) * factor)
}

// codeNameOf renders err's gRPC status code name, or "" when err is nil, or
// "Unknown" for a non-gRPC error (status.FromError returns codes.Unknown for a
// non-status error). The result feeds the Code field of [ClientEvent].
func codeNameOf(err error) string {
	if err == nil {
		return ""
	}
	st, ok := status.FromError(err)
	if !ok {
		return codes.Unknown.String()
	}
	return st.Code().String()
}
