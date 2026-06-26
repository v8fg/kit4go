// This file is the external benchmark suite (package grpcclient_test), so it
// exercises only the exported surface exactly as a real caller would. It
// reuses the bufconn-backed echo server and dial helpers defined in
// grpcclient_test.go (the same test binary), so every benchmark runs entirely
// in memory — no real socket, no port allocation, no flakiness from a real
// network. The Middleware metrics/hook path and the per-RPC retry/timeout
// bookkeeping are all on the measured path.
package grpcclient_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/wrapperspb"

	grpcclient "github.com/v8fg/kit4go/grpcclient"
)

// benchMiddleware returns a Middleware tuned for benchmarking: no retry on the
// happy path (RetryMax=0), tiny backoffs in case a failure path is exercised,
// and a generous per-RPC timeout so the echo call never approaches it. The
// bufconn echo server always succeeds, so the success path is what we measure.
func benchMiddleware() *grpcclient.Middleware {
	return grpcclient.NewMiddleware(grpcclient.ClientOptions{
		RetryMax:       0,
		RetryWaitMin:   time.Microsecond,
		RetryWaitMax:   10 * time.Microsecond,
		RequestTimeout: 10 * time.Second,
	})
}

// --- Benchmarks --------------------------------------------------------------

// BenchmarkUnary measures a single unary /echo.Echo/Echo round-trip over
// bufconn: build request, invoke (which runs the unary interceptor: timeout
// set, invoker call, metrics update, event hook dispatch), decode reply. The
// bufconn transport has no real I/O, so this isolates the client-side
// per-call overhead (interceptor + gRPC framing).
func BenchmarkUnary(b *testing.B) {
	srv := newEchoServer()
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := benchMiddleware()
	conn := dialBufconn(b, dialer, mw)
	defer conn.Close()

	ctx := context.Background()
	in := wrapperspb.String("hello")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := echoUnary(ctx, conn, in)
		if err != nil {
			b.Fatalf("Echo: %v", err)
		}
		if out.GetValue() != "hello" {
			b.Fatalf("got %q, want hello", out.GetValue())
		}
	}
	b.StopTimer()
	m := mw.Metrics()
	if m.Total != uint64(b.N) || m.Success != uint64(b.N) || m.Failed != 0 {
		b.Fatalf("metrics = %+v, want total=success=N failed=0", m)
	}
}

// BenchmarkUnary_Parallel exercises the unary path under RunParallel. The
// bufconn transport serialises internally (it is an in-memory pipe), so this
// benchmark surfaces contention in the Middleware's metric atomics and the
// interceptor's per-call allocations rather than transport parallelism —
// useful for detecting a regression in the lock-free hot path.
func BenchmarkUnary_Parallel(b *testing.B) {
	srv := newEchoServer()
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := benchMiddleware()
	conn := dialBufconn(b, dialer, mw)
	defer conn.Close()

	ctx := context.Background()
	in := wrapperspb.String("p")
	b.ReportAllocs()
	b.ResetTimer()
	var failures atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := echoUnary(ctx, conn, in); err != nil {
				failures.Add(1)
			}
		}
	})
	b.StopTimer()
	if failures.Load() > 0 {
		b.Fatalf("parallel unary reported %d errors", failures.Load())
	}
	m := mw.Metrics()
	if m.Success+m.Failed != m.Total {
		b.Fatalf("success+failed=%d != total=%d", m.Success+m.Failed, m.Total)
	}
}

// BenchmarkMiddleware_Metrics measures the cost of taking a metrics snapshot
// — the five atomic loads (total/success/failed/retried/active). This is the
// cost paid by every Prometheus/observability scrape of the middleware, so it
// must stay negligible relative to a real RPC.
func BenchmarkMiddleware_Metrics(b *testing.B) {
	srv := newEchoServer()
	dialer, shutdown := startTestServer(srv)
	defer shutdown()

	mw := benchMiddleware()
	conn := dialBufconn(b, dialer, mw)
	defer conn.Close()

	// Move some counters off zero so the load path isn't optimised away.
	ctx := context.Background()
	for i := 0; i < 16; i++ {
		_, _ = echoUnary(ctx, conn, wrapperspb.String("x"))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mw.Metrics()
	}
}
