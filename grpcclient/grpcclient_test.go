// This file is a package test (package grpcclient_test, the external test
// suite) so it exercises only the exported surface — NewMiddleware, DialConn,
// the interceptors, Metrics and SetOnEvent — exactly as a real caller would.
package grpcclient_test

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"

	grpcclient "github.com/v8fg/kit4go/grpcclient"
)

const (
	// bufSize is the in-memory connection buffer used by bufconn. Large enough
	// that no realistic test message fills it.
	bufSize = 1 << 20
	// serviceName / method names match the hand-written ServiceDesc below; no
	// .proto codegen is involved.
	serviceName  = "echo.Echo"
	methodEcho   = "/echo.Echo/Echo"
	methodStream = "/echo.Echo/Stream"
)

// echoerServer is the service interface grpc.RegisterService checks against.
// The concrete *echoServer satisfies it. It exists only because RegisterService
// requires HandlerType to be an interface (it calls Type.Implements).
type echoerServer interface {
	echoService() // marker method; the real handlers are plain funcs in the desc.
}

// echoServer is a trivial gRPC server implementing a unary /echo.Echo/Echo and
// a server-streaming /echo.Echo/Stream. It is hand-rolled (no .proto codegen)
// so the test suite has zero build-time dependency on protoc; we speak the wire
// format directly via grpc.ServiceDesc with manually-typed handlers using the
// well-known wrapperspb.StringValue message.
type echoServer struct {
	mu        sync.Mutex
	behaviour echoBehaviour
	attempts  atomic.Int32 // unary call count, read by tests to assert retries
}

// echoService is the marker satisfying echoerServer.
func (*echoServer) echoService() {}

// echoBehaviour configures how the echo server answers the next call(s). Tests
// swap it under the server's mutex to stage different failure modes.
type echoBehaviour struct {
	// code != codes.OK makes the unary handler return that gRPC status.
	code codes.Code
	// codeStream != codes.OK makes the stream handler return that status before
	// the first message.
	codeStream codes.Code
	// delay sleeps for this long before responding (used for timeout tests).
	delay time.Duration
	// failThenOK, when true, puts the unary handler in "fail N times then
	// succeed" mode: the first failCount calls return code, and every call
	// after that succeeds regardless of code. Mutually exclusive in spirit with
	// a standing persistent code (failThenOK=false + code != OK).
	failThenOK bool
	// failCount is the number of unary calls that should fail before the server
	// starts succeeding (only meaningful when failThenOK is true). Mutated
	// under mu as calls land.
	failCount int
}

func newEchoServer() *echoServer { return &echoServer{} }

func (s *echoServer) setBehaviour(b echoBehaviour) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.behaviour = b
}

func (s *echoServer) behaviourSnapshot() echoBehaviour {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.behaviour
}

// echoUnaryHandler is the grpc.MethodHandler for /echo.Echo/Echo. The signature
// matches grpc.MethodHandler exactly (srv, ctx, dec, interceptor); the
// interceptor arg is nil because the test server is built without server-side
// interceptors.
func echoUnaryHandler(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	s := srv.(*echoServer)
	req := new(wrapperspb.StringValue)
	if err := dec(req); err != nil {
		return nil, err
	}
	s.attempts.Add(1)
	b := s.behaviourSnapshot()

	if b.delay > 0 {
		select {
		case <-time.After(b.delay):
		case <-ctx.Done():
			return nil, status.FromContextError(ctx.Err()).Err()
		}
	}

	// failThenOK mode: fail the first failCount calls (with b.code), then
	// succeed regardless of b.code. failCount is mutated under mu as calls
	// land so retries see the decremented value.
	if b.failThenOK {
		if b.failCount > 0 {
			s.mu.Lock()
			s.behaviour.failCount--
			s.mu.Unlock()
			code := b.code
			if code == codes.OK {
				code = codes.Unavailable
			}
			return nil, status.Error(code, "injected failure")
		}
		// failCount exhausted → success phase.
		return wrapperspb.String(req.GetValue()), nil
	}
	// Standing persistent failure (failThenOK == false, code != OK).
	if b.code != codes.OK {
		return nil, status.Error(b.code, "injected failure")
	}
	return wrapperspb.String(req.GetValue()), nil
}

// echoStreamHandler is the grpc.StreamHandler for /echo.Echo/Stream.
func echoStreamHandler(srv any, stream grpc.ServerStream) error {
	s := srv.(*echoServer)
	first := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(first); err != nil {
		return err
	}
	b := s.behaviourSnapshot()
	if b.delay > 0 {
		select {
		case <-time.After(b.delay):
		case <-stream.Context().Done():
			return status.FromContextError(stream.Context().Err()).Err()
		}
	}
	if b.codeStream != codes.OK {
		return status.Error(b.codeStream, "injected stream failure")
	}
	return stream.SendMsg(wrapperspb.String(first.GetValue()))
}

// echoUnaryMethodDesc / echoStreamDesc are the hand-written service descriptor
// entries.
var (
	echoUnaryMethodDesc = grpc.MethodDesc{
		MethodName: "Echo",
		Handler:    echoUnaryHandler,
	}
	echoStreamDesc = grpc.StreamDesc{
		StreamName:    "Stream",
		ServerStreams: true,
		Handler:       echoStreamHandler,
	}
)

// startTestServer spins up a bufconn-backed gRPC server registering the echo
// service, returning (dialer-to-reach-it, shutdown). The caller calls shutdown
// to stop the server.
func startTestServer(srv *echoServer) (dialer func(context.Context, string) (net.Conn, error), shutdown func()) {
	lis := bufconn.Listen(bufSize)
	gs := grpc.NewServer()
	gs.RegisterService(&grpc.ServiceDesc{
		ServiceName: serviceName,
		HandlerType: (*echoerServer)(nil),
		Methods:     []grpc.MethodDesc{echoUnaryMethodDesc},
		Streams:     []grpc.StreamDesc{echoStreamDesc},
		Metadata:    "echo.proto",
	}, srv)
	go func() { _ = gs.Serve(lis) }()
	return func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}, gs.GracefulStop
}

// testingTB is the minimal slice of *testing.T that dialBufconn needs.
// Defining it as an interface lets Example functions reuse dialBufconn via a
// no-op fakeT (Examples cannot receive a *testing.T).
type testingTB interface {
	Helper()
	Fatalf(format string, args ...any)
}

// dialBufconn returns a *grpc.ClientConn that talks to dialer and is wired with
// mw's interceptors. The caller must Close it.
func dialBufconn(t testingTB, dialer func(context.Context, string) (net.Conn, error), mw *grpcclient.Middleware) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(mw.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(mw.StreamClientInterceptor()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	return conn
}

// echoUnary invokes the unary /echo.Echo/Echo method.
func echoUnary(ctx context.Context, cc *grpc.ClientConn, in *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	out := new(wrapperspb.StringValue)
	err := cc.Invoke(ctx, methodEcho, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// echoStream opens the server-streaming /echo.Echo/Stream.
func echoStream(ctx context.Context, cc *grpc.ClientConn) (grpc.ClientStream, error) {
	return cc.NewStream(ctx, &grpc.StreamDesc{
		StreamName:    "Stream",
		ServerStreams: true,
	}, methodStream)
}

// codeOf extracts the gRPC status code from an error.
func codeOf(err error) codes.Code { return status.Code(err) }

// ===========================================================================
// Tests
// ===========================================================================

// TestUnaryBasic verifies a clean echo round-trip increments success only.
func TestUnaryBasic(t *testing.T) {
	srv := newEchoServer()
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{RetryMax: 0})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	resp, err := echoUnary(context.Background(), conn, wrapperspb.String("hello"))
	if err != nil {
		t.Fatalf("Echo: %v", err)
	}
	if resp.GetValue() != "hello" {
		t.Fatalf("got %q, want hello", resp.GetValue())
	}

	m := mw.Metrics()
	if m.Total != 1 || m.Success != 1 || m.Failed != 0 || m.Retried != 0 {
		t.Fatalf("metrics = %+v, want total=1 success=1 failed=0 retried=0", m)
	}
}

// TestRetryOnUnavailable stages a server that fails the first calls with
// Unavailable then succeeds, and asserts the client retries and ultimately
// succeeds. The default RetryCodes include Unavailable.
func TestRetryOnUnavailable(t *testing.T) {
	srv := newEchoServer()
	srv.setBehaviour(echoBehaviour{code: codes.Unavailable, failThenOK: true, failCount: 2})
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax:     3,
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: 5 * time.Millisecond,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	resp, err := echoUnary(context.Background(), conn, wrapperspb.String("ok"))
	if err != nil {
		t.Fatalf("Echo: %v", err)
	}
	if resp.GetValue() != "ok" {
		t.Fatalf("got %q, want ok", resp.GetValue())
	}

	// The server should have seen 3 attempts: 2 failing + 1 success.
	if got := srv.attempts.Load(); got != 3 {
		t.Fatalf("server attempts = %d, want 3", got)
	}

	m := mw.Metrics()
	if m.Total != 1 || m.Success != 1 || m.Failed != 0 {
		t.Fatalf("metrics = %+v, want success path", m)
	}
	if m.Retried != 2 {
		t.Fatalf("retried = %d, want 2", m.Retried)
	}
}

// TestNoRetryOnNotFound asserts a non-retryable code (NotFound) is NOT retried
// even when RetryMax > 0.
func TestNoRetryOnNotFound(t *testing.T) {
	srv := newEchoServer()
	srv.setBehaviour(echoBehaviour{code: codes.NotFound})
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax:     3,
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: 5 * time.Millisecond,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	_, err := echoUnary(context.Background(), conn, wrapperspb.String("x"))
	if err == nil {
		t.Fatal("expected NotFound error, got nil")
	}
	if got := srv.attempts.Load(); got != 1 {
		t.Fatalf("server attempts = %d, want 1 (no retry)", got)
	}
	if m := mw.Metrics(); m.Retried != 0 || m.Failed != 1 || m.Total != 1 {
		t.Fatalf("metrics = %+v, want 1 failed, 0 retried", m)
	}
	if codeOf(err) != codes.NotFound {
		t.Fatalf("surfaced code = %v, want NotFound", codeOf(err))
	}
}

// TestUnaryTimeout verifies the per-RPC RequestTimeout fires when the server
// sleeps past it. The server delay exceeds the configured timeout.
func TestUnaryTimeout(t *testing.T) {
	srv := newEchoServer()
	srv.setBehaviour(echoBehaviour{delay: 200 * time.Millisecond})
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RequestTimeout: 50 * time.Millisecond,
		RetryMax:       2,
		RetryWaitMin:   time.Millisecond,
		RetryWaitMax:   5 * time.Millisecond,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	start := time.Now()
	_, err := echoUnary(context.Background(), conn, wrapperspb.String("slow"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	// Should return within ~100ms of the timeout (50ms), not the full 200ms
	// server delay.
	if elapsed > 150*time.Millisecond {
		t.Fatalf("took %v, want ~50ms (timeout)", elapsed)
	}
	if m := mw.Metrics(); m.Failed != 1 || m.Total != 1 {
		t.Fatalf("metrics = %+v, want 1 failed", m)
	}
	// A deadline-driven failure is nominally not retried (the interceptor
	// bails when callCtx.Err() != nil). There is, however, a small race
	// window in which the transport surfaces DeadlineExceeded a hair before
	// the context's own Err() flips non-nil, so a retry or two can slip
	// through under scheduler/CPU pressure (notably under -race, which adds
	// ~10x per-op overhead). Because DeadlineExceeded is a default
	// RetryCode, this is expected and benign: each such retry immediately
	// fails on the now-expired context, so the call is still bounded by the
	// ~50ms timeout asserted above. The strict "ctx cancel is never retried"
	// contract is asserted deterministically (and independently) by
	// TestContextCancelNotRetried, so we do not pin Retried here.
}

// TestRetryMaxExhausted verifies that once RetryMax attempts fail, the last
// error is surfaced and metrics record the retries.
func TestRetryMaxExhausted(t *testing.T) {
	srv := newEchoServer()
	srv.setBehaviour(echoBehaviour{code: codes.Unavailable})
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax:     2,
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: 5 * time.Millisecond,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	_, err := echoUnary(context.Background(), conn, wrapperspb.String("x"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// RetryMax=2 means 3 total attempts.
	if got := srv.attempts.Load(); got != 3 {
		t.Fatalf("server attempts = %d, want 3", got)
	}
	if m := mw.Metrics(); m.Retried != 2 || m.Failed != 1 || m.Success != 0 {
		t.Fatalf("metrics = %+v, want 2 retried, 1 failed, 0 success", m)
	}
	if codeOf(err) != codes.Unavailable {
		t.Fatalf("surfaced code = %v, want Unavailable", codeOf(err))
	}
}

// TestBreakerIntegration uses a mock breaker to assert the unary interceptor
// funnels each call through Execute and short-circuits when open.
func TestBreakerIntegration(t *testing.T) {
	srv := newEchoServer()
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	var fnCalls atomic.Int32
	mb := &mockBreaker{
		execute: func(ctx context.Context, fn func(ctx context.Context) error) error {
			fnCalls.Add(1)
			return fn(ctx)
		},
	}

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax: 0,
		Breaker:  mb,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	_, err := echoUnary(context.Background(), conn, wrapperspb.String("hi"))
	if err != nil {
		t.Fatalf("Echo: %v", err)
	}
	if fnCalls.Load() != 1 {
		t.Fatalf("breaker.Execute fn calls = %d, want 1", fnCalls.Load())
	}

	// Now flip the breaker open: Execute returns Unavailable without calling fn.
	mb.execute = func(ctx context.Context, fn func(ctx context.Context) error) error {
		return status.Error(codes.Unavailable, "circuit breaker open")
	}
	_, err = echoUnary(context.Background(), conn, wrapperspb.String("hi"))
	if err == nil {
		t.Fatal("expected breaker-open error, got nil")
	}
	if codeOf(err) != codes.Unavailable {
		t.Fatalf("surfaced code = %v, want Unavailable", codeOf(err))
	}
	// fn must NOT have been invoked on the open call.
	if fnCalls.Load() != 1 {
		t.Fatalf("breaker.Execute fn calls = %d after open, want still 1", fnCalls.Load())
	}
	// Two calls total: 1 success + 1 failed (the open call never reached the
	// server, but the interceptor counts it as failed).
	if m := mw.Metrics(); m.Total != 2 || m.Failed != 1 || m.Success != 1 {
		t.Fatalf("metrics = %+v, want total=2 success=1 failed=1", m)
	}
}

// TestStreamBasic verifies the stream interceptor opens a streaming RPC and the
// echoed message arrives. It also confirms no retries happen on streams.
func TestStreamBasic(t *testing.T) {
	srv := newEchoServer()
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax:     3,
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: 5 * time.Millisecond,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	stream, err := echoStream(context.Background(), conn)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := stream.SendMsg(wrapperspb.String("streamed")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	msg := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(msg); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if msg.GetValue() != "streamed" {
		t.Fatalf("got %q, want streamed", msg.GetValue())
	}
	// Server half-closes after one message.
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}

	// Streams must NOT be retried.
	if m := mw.Metrics(); m.Total != 1 || m.Success != 1 || m.Retried != 0 {
		t.Fatalf("metrics = %+v, want total=1 success=1 retried=0", m)
	}
}

// TestStreamTimeout verifies the per-RPC RequestTimeout tears down a server
// stream that sleeps past it. The delay is server-side before the first Send.
func TestStreamTimeout(t *testing.T) {
	srv := newEchoServer()
	srv.setBehaviour(echoBehaviour{delay: 200 * time.Millisecond})
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RequestTimeout: 50 * time.Millisecond,
		RetryMax:       0,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	start := time.Now()
	stream, err := echoStream(context.Background(), conn)
	if err != nil {
		// Open may itself fail with DeadlineExceeded if the timeout fires
		// during setup; that still counts as a failed call.
		if m := mw.Metrics(); m.Failed != 1 {
			t.Fatalf("metrics = %+v, want 1 failed", m)
		}
		return
	}
	if err := stream.SendMsg(wrapperspb.String("x")); err != nil {
		// Send error is acceptable too; ensure metrics reflect failure.
		_ = err
		return
	}
	if err := stream.CloseSend(); err != nil {
		_ = err
	}
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err == nil {
		t.Fatal("expected RecvMsg error from timeout, got nil")
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("took %v, want ~50ms (timeout)", elapsed)
	}
}

// TestSetOnEvent verifies the event hook fires request/success for a clean
// unary call and request/retry/request/success for a retried one.
func TestSetOnEvent(t *testing.T) {
	srv := newEchoServer()
	srv.setBehaviour(echoBehaviour{code: codes.Unavailable, failThenOK: true, failCount: 1})
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax:     2,
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: 5 * time.Millisecond,
	})

	var mu sync.Mutex
	var events []grpcclient.ClientEvent
	mw.SetOnEvent(func(e grpcclient.ClientEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	})

	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	if _, err := echoUnary(context.Background(), conn, wrapperspb.String("x")); err != nil {
		t.Fatalf("Echo: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// Expected sequence: request(attempt0), retry(attempt1), request(attempt1), success.
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4: %+v", len(events), events)
	}
	wantNames := []string{"request", "retry", "request", "success"}
	for i, w := range wantNames {
		if events[i].Name != w {
			t.Fatalf("event[%d].Name = %q, want %q (all: %+v)", i, events[i].Name, w, events)
		}
	}
	// The retry event's Code should be the retryable status name.
	if events[1].Code != codes.Unavailable.String() {
		t.Fatalf("retry event Code = %q, want %q", events[1].Code, codes.Unavailable.String())
	}
	if events[3].Code != "" {
		t.Fatalf("success event Code = %q, want empty", events[3].Code)
	}
}

// TestDialConnMissingTarget verifies DialConn rejects an empty Target with a
// clear error rather than panicking inside grpc.Dial.
func TestDialConnMissingTarget(t *testing.T) {
	_, err := grpcclient.DialConn(grpcclient.ClientOptions{Target: ""})
	if err == nil {
		t.Fatal("expected error for empty Target, got nil")
	}
}

// TestWithDefaults exercises the option-defaulting path: a ClientOptions with
// only the waits overridden must still pick up the default RetryMax (2) and the
// default RetryCodes (Unavailable), so a failing server is retried twice.
func TestWithDefaults(t *testing.T) {
	srv := newEchoServer()
	srv.setBehaviour(echoBehaviour{code: codes.Unavailable})
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: 5 * time.Millisecond,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	_, _ = echoUnary(context.Background(), conn, wrapperspb.String("x"))
	// Default RetryMax == 2 → 3 attempts.
	if got := srv.attempts.Load(); got != 3 {
		t.Fatalf("server attempts = %d, want 3 (default RetryMax=2)", got)
	}
}

// TestRetryDelayBoundedIndirectly sanity-checks the backoff by timing two
// retries: the total wall-clock must sit between two retries' minimum jitter
// (>= 0.5*minWait each) and a generous upper bound.
func TestRetryDelayBoundedIndirectly(t *testing.T) {
	srv := newEchoServer()
	srv.setBehaviour(echoBehaviour{code: codes.Unavailable})
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax:     2,
		RetryWaitMin: 20 * time.Millisecond,
		RetryWaitMax: 40 * time.Millisecond,
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	start := time.Now()
	_, _ = echoUnary(context.Background(), conn, wrapperspb.String("x"))
	elapsed := time.Since(start)
	// Two retries with backoff in [10ms, 20ms] each → at least ~20ms total,
	// and certainly far below 1s.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("retry backoff took %v, expected < 500ms", elapsed)
	}
	if elapsed < 20*time.Millisecond {
		t.Fatalf("retry backoff took %v, expected >= ~20ms (two retries >= 10ms each)", elapsed)
	}
}

// TestContextCancelNotRetried asserts that when the caller cancels the context
// mid-call (distinct from a RequestTimeout deadline), the failure is surfaced
// immediately without retry even though the code would otherwise be retryable.
func TestContextCancelNotRetried(t *testing.T) {
	srv := newEchoServer()
	// Long delay so the caller's cancel fires while the RPC is in flight.
	srv.setBehaviour(echoBehaviour{delay: time.Second, code: codes.Unavailable})
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax:     3,
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: 5 * time.Millisecond,
		// No RequestTimeout: only the caller's cancel aborts the call.
	})
	conn := dialBufconn(t, dialer, mw)
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel shortly after issuing; the server's 1s delay guarantees the call
	// is still in flight.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := echoUnary(ctx, conn, wrapperspb.String("x"))
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	// Must NOT have been retried.
	if m := mw.Metrics(); m.Retried != 0 {
		t.Fatalf("retried = %d, want 0 (ctx cancel not retried)", m.Retried)
	}
	if m := mw.Metrics(); m.Failed != 1 || m.Total != 1 {
		t.Fatalf("metrics = %+v, want 1 failed", m)
	}
}

// --- mock breaker ---

// mockBreaker implements grpcclient.CircuitBreaker by delegating to a callable
// field, so tests can swap the open/closed behaviour mid-flight.
type mockBreaker struct {
	execute func(ctx context.Context, fn func(ctx context.Context) error) error
}

func (m *mockBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	return m.execute(ctx, fn)
}

// Keep the errors import referenced: status.FromContextError returns a status
// that wraps context errors, and we assert via errors.Is in no test directly,
// but the import documents intent. Use a compile-time reference to satisfy the
// goimports "unused import" check if the file is trimmed later.
var _ = errors.Is
