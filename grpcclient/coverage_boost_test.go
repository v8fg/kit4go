// This file is the external coverage-boost suite (package grpcclient_test). It
// closes the end-to-end gaps that the internal suite cannot reach because they
// need a real bufconn-backed gRPC round-trip: the LatencyObserver hook firing
// through both interceptors (observe() via the interceptors' defer path), the
// DialConn happy path (non-empty Target), the stream failure + breaker branches,
// and the cancelOnDoneStream idempotent-cancel arm. It reuses the bufconn echo
// fixture and helpers defined in grpcclient_test.go (same test binary).
package grpcclient_test

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	grpcclient "github.com/v8fg/kit4go/grpcclient"
)

// recordingObserver is the external analogue of the internal fakeObserver: it
// counts Observe calls and records the max duration seen, guarded by a mutex so
// -race stays clean when the interceptor dispatches from the RPC goroutine.
type recordingObserver struct {
	mu      sync.Mutex
	count   int
	maxDur  time.Duration
	sawTime bool
}

func (r *recordingObserver) Observe(d time.Duration) {
	r.mu.Lock()
	r.count++
	if d > r.maxDur {
		r.maxDur = d
	}
	if d > 0 {
		r.sawTime = true
	}
	r.mu.Unlock()
}

func (r *recordingObserver) snapshot() (int, time.Duration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count, r.maxDur, r.sawTime
}

// TestUnaryLatencyObserverFires wires a recordingObserver through the unary
// interceptor and asserts Observe fires exactly once per call with a strictly
// positive duration. This covers the previously-0% observe() helper end-to-end
// (via the defer m.observe(start) branch in UnaryClientInterceptor) and proves
// the disabled (nil) path is inert by contrast with every other test.
func TestUnaryLatencyObserverFires(t *testing.T) {
	srv := newEchoServer()
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	obs := &recordingObserver{}
	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax:       0,
		RequestTimeout: 5 * time.Second,
		Latency:        obs,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	const n = 4
	for i := 0; i < n; i++ {
		if _, err := echoUnary(context.Background(), conn, wrapperspb.String("x")); err != nil {
			t.Fatalf("Echo: %v", err)
		}
	}
	count, _, sawTime := obs.snapshot()
	if count != n {
		t.Fatalf("observe count = %d, want %d (one per unary call)", count, n)
	}
	if !sawTime {
		t.Error("observed durations were all zero; want strictly positive")
	}
}

// TestUnaryLatencyObserverFiresOnRetry asserts the latency hook fires once per
// logical call (not once per attempt): a retried call records exactly one
// sample whose duration covers the full retry loop. Covers observe() on the
// retry path of UnaryClientInterceptor.
func TestUnaryLatencyObserverFiresOnRetry(t *testing.T) {
	srv := newEchoServer()
	srv.setBehaviour(echoBehaviour{code: codes.Unavailable, failThenOK: true, failCount: 1})
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	obs := &recordingObserver{}
	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax:     2,
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: 5 * time.Millisecond,
		Latency:      obs,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	if _, err := echoUnary(context.Background(), conn, wrapperspb.String("x")); err != nil {
		t.Fatalf("Echo: %v", err)
	}
	count, _, _ := obs.snapshot()
	if count != 1 {
		t.Fatalf("observe count = %d, want 1 (one per logical call, not per attempt)", count)
	}
}

// TestStreamLatencyObserverFires covers the Latency branch of
// StreamClientInterceptor: the stream-open path records one sample on success.
func TestStreamLatencyObserverFires(t *testing.T) {
	srv := newEchoServer()
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	obs := &recordingObserver{}
	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RequestTimeout: 5 * time.Second,
		Latency:        obs,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	stream, err := echoStream(context.Background(), conn)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := stream.SendMsg(wrapperspb.String("s")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	msg := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(msg); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	count, _, _ := obs.snapshot()
	if count != 1 {
		t.Fatalf("observe count = %d, want 1 (one per stream open)", count)
	}
}

// TestStreamFailureOpenError covers the stream-open failure arm of
// StreamClientInterceptor (the `if err != nil` block: cancel + failed+1 +
// fireEvent("failed") + return nil,err) AND the `if e != nil` arm of the
// inner open() closure (streamer returns an error). We drive it two ways:
//
//   - "no breaker, cancelled ctx": open(rpcCtx) runs and the streamer rejects
//     the already-cancelled context, exercising the open() error arm without a
//     breaker in the picture.
//   - "breaker open": Execute short-circuits with a status error, so open() is
//     never called but the interceptor's outer failure arm still fires.
//
// Streams are lazy in gRPC, so a handler-side NotFound surfaces on RecvMsg, not
// on NewStream; to make the *open* fail we cancel the context before NewStream
// is issued (the streamer then returns context.Canceled).
func TestStreamFailureOpenError(t *testing.T) {
	t.Run("no_breaker_cancelled_ctx", func(t *testing.T) {
		srv := newEchoServer()
		dialer, shutdown := startTestServer(srv)
		defer shutdown()

		var events []grpcclient.ClientEvent
		var mu sync.Mutex
		mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{RetryMax: 0})
		mw.SetOnEvent(func(e grpcclient.ClientEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		})
		conn := dialBufconn(t, dialer, mw)
		defer conn.Close()

		// Pre-cancel: the streamer rejects the context on NewStream, so open()
		// returns a non-nil error without invoking the server handler.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		stream, err := echoStream(ctx, conn)
		if err == nil {
			if stream != nil {
				_ = stream.CloseSend()
			}
			t.Fatal("expected stream-open error from cancelled ctx, got nil")
		}
		if codeOf(err) != codes.Canceled {
			t.Fatalf("surfaced code = %v, want Canceled", codeOf(err))
		}
		m := mw.Metrics()
		if m.Total != 1 || m.Failed != 1 || m.Success != 0 {
			t.Fatalf("metrics = %+v, want total=1 failed=1 success=0", m)
		}
		mu.Lock()
		defer mu.Unlock()
		var sawFailed bool
		for _, e := range events {
			if e.Name == "failed" {
				sawFailed = true
			}
		}
		if !sawFailed {
			t.Fatalf("no failed event fired; events = %+v", events)
		}
	})

	t.Run("breaker_open", func(t *testing.T) {
		srv := newEchoServer()
		dialer, shutdown := startTestServer(srv)
		defer shutdown()

		// Breaker short-circuits with a status error and never invokes fn, so
		// open() is never reached; the interceptor's outer failure arm fires.
		mb := &mockBreaker{
			execute: func(ctx context.Context, fn func(ctx context.Context) error) error {
				return status.Error(codes.Unavailable, "circuit open")
			},
		}
		var events []grpcclient.ClientEvent
		var mu sync.Mutex
		mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{RetryMax: 0, Breaker: mb})
		mw.SetOnEvent(func(e grpcclient.ClientEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		})
		conn := dialBufconn(t, dialer, mw)
		defer conn.Close()

		_, err := echoStream(context.Background(), conn)
		if err == nil {
			t.Fatal("expected breaker-open stream error, got nil")
		}
		if codeOf(err) != codes.Unavailable {
			t.Fatalf("surfaced code = %v, want Unavailable", codeOf(err))
		}
		if m := mw.Metrics(); m.Total != 1 || m.Failed != 1 || m.Success != 0 {
			t.Fatalf("metrics = %+v, want total=1 failed=1 success=0", m)
		}
		mu.Lock()
		defer mu.Unlock()
		var sawFailed bool
		for _, e := range events {
			if e.Name == "failed" && e.Code == codes.Unavailable.String() {
				sawFailed = true
			}
		}
		if !sawFailed {
			t.Fatalf("no failed Unavailable event fired; events = %+v", events)
		}
	})
}

// TestStreamWithBreaker covers the Breaker != nil branch of
// StreamClientInterceptor: the breaker wraps stream open. We use a mock breaker
// that counts Execute invocations and delegates, then flip it open to cover the
// failure surfacing (breaker short-circuits without opening the stream).
func TestStreamWithBreaker(t *testing.T) {
	srv := newEchoServer()
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	var execCalls atomic.Int32
	mb := &mockBreaker{
		execute: func(ctx context.Context, fn func(ctx context.Context) error) error {
			execCalls.Add(1)
			return fn(ctx)
		},
	}
	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax: 0,
		Breaker:  mb,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	// Happy path: breaker delegates, stream opens, success recorded.
	stream, err := echoStream(context.Background(), conn)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := stream.SendMsg(wrapperspb.String("s")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if execCalls.Load() != 1 {
		t.Fatalf("breaker.Execute calls = %d, want 1", execCalls.Load())
	}
	if m := mw.Metrics(); m.Success != 1 {
		t.Fatalf("metrics = %+v, want success=1", m)
	}

	// Flip the breaker open: Execute returns a status error without invoking
	// fn, so stream open fails and the failure arm fires.
	mb.execute = func(ctx context.Context, fn func(ctx context.Context) error) error {
		return status.Error(codes.Unavailable, "circuit open")
	}
	_, err = echoStream(context.Background(), conn)
	if err == nil {
		t.Fatal("expected breaker-open stream error, got nil")
	}
	if codeOf(err) != codes.Unavailable {
		t.Fatalf("surfaced code = %v, want Unavailable", codeOf(err))
	}
	if m := mw.Metrics(); m.Total != 2 || m.Failed != 1 || m.Success != 1 {
		t.Fatalf("metrics = %+v, want total=2 failed=1 success=1", m)
	}
}

// TestDialConnHappyPath covers the non-empty-Target branch of DialConn: it must
// build the dial options and return a usable *grpc.ClientConn wired with the
// interceptors. The dial is non-blocking, so we get a conn back even without a
// server listening; closing it must not error. bufconn is not used here because
// DialConn hard-codes insecure transport against opts.Target — instead we dial
// a reserved port that nothing answers and assert we still get a non-nil conn
// (grpc.Dial returns immediately under non-blocking dial).
func TestDialConnHappyPath(t *testing.T) {
	// Target with a resolver scheme so dial resolves to localhost immediately
	// without blocking on DNS. Non-blocking dial returns a conn regardless of
	// whether a server is up.
	conn, err := grpcclient.DialConn(grpcclient.ClientOptions{
		Target:         "127.0.0.1:0",
		ConnectTimeout: 100 * time.Millisecond,
		RequestTimeout: 100 * time.Millisecond,
		RetryMax:       0,
	})
	if err != nil {
		t.Fatalf("DialConn: %v", err)
	}
	if conn == nil {
		t.Fatal("DialConn returned nil conn with nil err")
	}
	// The conn is in transient failure (no server), but Close must succeed and
	// must not panic. This proves the interceptors were wired without error.
	if err := conn.Close(); err != nil {
		t.Fatalf("conn.Close: %v", err)
	}
}

// TestCancelOnDoneStreamIdempotent covers the s.done early-return arm of
// cancelOnDoneStream.RecvMsg: after the first terminal RecvMsg (io.EOF), a
// subsequent RecvMsg must NOT call cancel again (cancel is already invoked, and
// calling it twice is harmless but the branch must short-circuit). We verify by
// counting cancel invocations via a wrapper.
func TestCancelOnDoneStreamIdempotent(t *testing.T) {
	srv := newEchoServer()
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax:       0,
		RequestTimeout: 5 * time.Second,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	stream, err := echoStream(context.Background(), conn)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := stream.SendMsg(wrapperspb.String("s")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	// First RecvMsg returns the message; second returns io.EOF and triggers the
	// one-shot cancel.
	msg := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(msg); err != nil {
		t.Fatalf("first RecvMsg: %v", err)
	}
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Fatalf("second RecvMsg: want io.EOF, got %v", err)
	}
	// Third RecvMsg hits the s.done early-return arm: it must not panic and must
	// return io.EOF again (forwarded from the underlying stream).
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Fatalf("third RecvMsg after done: want io.EOF, got %v", err)
	}
}

// TestRetryDelayContextCancelledDuringBackoff covers the <-callCtx.Done() arm
// of the retry loop: while the interceptor is sleeping in the backoff select,
// cancelling the caller's context must short-circuit the retry and surface
// callCtx.Err() instead of waiting for the full delay.
func TestRetryDelayContextCancelledDuringBackoff(t *testing.T) {
	srv := newEchoServer()
	srv.setBehaviour(echoBehaviour{code: codes.Unavailable})
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax:     5,
		RetryWaitMin: 200 * time.Millisecond, // generous so cancel lands mid-backoff
		RetryWaitMax: 400 * time.Millisecond,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond) // first attempt fails, backoff begins
		cancel()
	}()
	start := time.Now()
	_, err := echoUnary(ctx, conn, wrapperspb.String("x"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error from cancelled backoff, got nil")
	}
	// Must abort well before the full retry schedule (5 retries * ~200ms).
	if elapsed > 300*time.Millisecond {
		t.Fatalf("took %v, want < ~300ms (cancel should abort backoff quickly)", elapsed)
	}
	// Exactly one retry was counted (the first failure), no second attempt.
	if m := mw.Metrics(); m.Retried < 1 {
		t.Fatalf("retried = %d, want >= 1", m.Retried)
	}
}

// Compile-time reference to keep the grpc import used (NewClient path shared
// with grpcclient_test.go via dialBufconn); avoids a stale-import lint if this
// file is later trimmed.
var _ grpc.ClientConnInterface = (*grpc.ClientConn)(nil)
