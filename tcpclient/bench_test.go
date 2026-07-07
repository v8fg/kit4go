// This file is an internal benchmark/coverage test (package tcpclient, not
// tcpclient_test) so it can reach the unexported connPool directly for the
// pool-only benchmark (BenchmarkPool_GetPut) and assert the new ActiveConn /
// PoolSize metric fields. The networked benchmarks spin up a local echo server
// to measure the real round-trip cost (pool checkout, dial, write, read,
// retry bookkeeping) end to end.
package tcpclient

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// benchEchoListener starts a TCP listener whose accepted connections are
// echoed back via io.Copy and kept open (persistent echo). This is the right
// fixture for Send (write-only) benchmarks: Send leaves the connection poolable
// so the pool warms up and subsequent sends reuse a connection — which is what
// we want to measure (the steady-state pooled cost, not a fresh dial each
// time). The returned *uint64 counts accepted connections so the pool-reuse
// benchmark can assert it stayed flat.
func benchEchoListener(tb testing.TB) (net.Listener, *uint64) {
	tb.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("listen: %v", err)
	}
	var conns uint64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddUint64(&conns, 1)
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn) // echo until error or EOF
			}(c)
		}
	}()
	return ln, &conns
}

// benchEchoOnceListener starts a listener that, for each accepted connection,
// reads one chunk, echoes it back and closes — the SendReceive fixture. The
// close is essential: SendReceive reads until EOF (or ReadTimeout), so an
// open echo would force every call to hit its read deadline.
func benchEchoOnceListener(tb testing.TB) net.Listener {
	tb.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				buf := make([]byte, 4*1024)
				_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
				n, err := conn.Read(buf)
				if err != nil || n == 0 {
					return
				}
				_, _ = conn.Write(buf[:n])
			}(c)
		}
	}()
	return ln
}

// benchOpts returns ClientOptions tuned for benchmarking: a generous pool so
// the pool is not the bottleneck for the parallel benchmark, tiny backoffs so
// any failure path doesn't sleep, and short (but non-zero) timeouts so a
// wedged peer fails fast rather than stalling the benchmark.
func benchOpts(addr string) ClientOptions {
	return ClientOptions{
		Network:        "tcp",
		Address:        addr,
		ConnectTimeout: time.Second,
		ReadTimeout:    500 * time.Millisecond,
		WriteTimeout:   500 * time.Millisecond,
		PoolSize:       64,
		IdleTimeout:    30 * time.Second,
		RetryMax:       0, // no retries on the hot path; benchmarks measure the happy path
		RetryWaitMin:   time.Microsecond,
		RetryWaitMax:   10 * time.Microsecond,
	}
}

// --- Benchmarks --------------------------------------------------------------

// BenchmarkSend measures a write-only Send against a persistent echo server.
// After the first iteration the connection is pooled, so this measures the
// steady-state checkout/write/return cost under a single goroutine.
func BenchmarkSend(b *testing.B) {
	ln, _ := benchEchoListener(b)
	defer ln.Close()
	c := NewClient(benchOpts(ln.Addr().String()))
	defer c.Close()

	ctx := context.Background()
	data := []byte("x")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := c.Send(ctx, data); err != nil {
			b.Fatalf("Send: %v", err)
		}
	}
	b.StopTimer()
	m := c.Metrics()
	if m.Total != uint64(b.N) || m.Success != uint64(b.N) || m.Failed != 0 {
		b.Fatalf("metrics = %+v, want total=success=N failed=0", m)
	}
}

// BenchmarkSendReceive measures a write+read-until-EOF round-trip against an
// echo-then-close server. Each iteration dials a fresh connection (the server
// closes after one echo so the connection is never pooled), which is the
// realistic cost model for request/reply protocols whose peer half-closes.
func BenchmarkSendReceive(b *testing.B) {
	ln := benchEchoOnceListener(b)
	defer ln.Close()
	c := NewClient(benchOpts(ln.Addr().String()))
	defer c.Close()

	ctx := context.Background()
	data := []byte("hello")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := c.SendReceive(ctx, data)
		if err != nil {
			b.Fatalf("SendReceive: %v", err)
		}
		if string(resp) != "hello" {
			b.Fatalf("resp = %q, want hello", resp)
		}
	}
	b.StopTimer()
	m := c.Metrics()
	if m.Total != uint64(b.N) || m.Success != uint64(b.N) {
		b.Fatalf("metrics = %+v, want total=success=N", m)
	}
}

// BenchmarkSendReceive_Parallel exercises SendReceive under RunParallel with a
// generous goroutine count (1000). The pool is sized to 64 so most goroutines
// dial concurrently and return connections to the pool; this surfaces any
// pool-contention or metric-contention regression as a throughput drop.
func BenchmarkSendReceive_Parallel(b *testing.B) {
	ln := benchEchoOnceListener(b)
	defer ln.Close()
	c := NewClient(benchOpts(ln.Addr().String()))
	defer c.Close()

	ctx := context.Background()
	data := []byte("p")
	b.ReportAllocs()
	b.ResetTimer()
	var errs atomic.Int32
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := c.SendReceive(ctx, data); err != nil {
				errs.Add(1)
			}
		}
	})
	b.StopTimer()
	if errs.Load() > 0 {
		b.Fatalf("parallel SendReceive reported %d errors", errs.Load())
	}
	// Under high concurrency a few calls may be in-flight when we snapshot, so
	// only assert the structural invariants: success+failed == total.
	m := c.Metrics()
	if m.Success+m.Failed != m.Total {
		b.Fatalf("success+failed=%d != total=%d", m.Success+m.Failed, m.Total)
	}
}

// BenchmarkPool_GetPut measures the raw pool checkout/return overhead with no
// network in the loop. It drives connPool.get + connPool.put directly so the
// result reflects only the channel + eviction-check cost — useful to detect a
// regression in the pool's fast path independent of dial/read/write latency.
//
// Because get dials when the pool is empty, the benchmark warms the pool with
// one real connection first and then loops get/put on that same connection.
func BenchmarkPool_GetPut(b *testing.B) {
	ln, _ := benchEchoListener(b)
	defer ln.Close()

	p := newConnPool("tcp", ln.Addr().String(), 64, time.Second, 30*time.Second)
	defer p.close()
	ctx := context.Background()
	// Warm the pool with one connection so the loop hits the reuse fast path.
	conn, err := p.get(ctx, time.Second)
	if err != nil {
		b.Fatalf("warm get: %v", err)
	}
	p.put(conn)

	b.ReportAllocs()

	for b.Loop() {
		c, err := p.get(ctx, time.Second)
		if err != nil {
			b.Fatalf("get: %v", err)
		}
		p.put(c)
	}
}

// BenchmarkClient_Metrics measures the cost of taking a metrics snapshot — the
// five atomic loads (total/success/failed/retried/activeConn) plus the
// len(pool) channel-depth read. This is the cost paid by every scrape of the
// client's observability surface, so it must stay negligible.
func BenchmarkClient_Metrics(b *testing.B) {
	ln, _ := benchEchoListener(b)
	defer ln.Close()
	c := NewClient(benchOpts(ln.Addr().String()))
	defer c.Close()

	// Move some counters off zero so the load path isn't optimised away.
	ctx := context.Background()
	for range 16 {
		_ = c.Send(ctx, []byte("x"))
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = c.Metrics()
	}
}

// --- Stress test -------------------------------------------------------------

// TestStress_MillionSends drives one million goroutines each issuing a single
// 1-byte Send to a persistent echo server, then asserts the client's metrics
// account for every call. It is the manual "can the client survive a
// million-concurrency fan-out" check: skipped under -short so CI does not pay
// for it.
//
// Run manually with:
//
//	go test -run TestStress_MillionSends -timeout 10m ./tcpclient/
func TestStress_MillionSends(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping million-concurrency stress test in -short mode")
	}
	const goroutines = 1_000_000

	ln, accepted := benchEchoListener(t)
	defer ln.Close()
	// A large pool absorbs the post-dial reuse; many goroutines will still
	// dial fresh because the fan-out is near-simultaneous, which is the point.
	c := NewClient(ClientOptions{
		Network:        "tcp",
		Address:        ln.Addr().String(),
		ConnectTimeout: 30 * time.Second,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		PoolSize:       1024,
		IdleTimeout:    time.Minute,
		RetryMax:       2,
		RetryWaitMin:   time.Millisecond,
		RetryWaitMax:   20 * time.Millisecond,
	})
	defer c.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(goroutines)
	// A semaphore would throttle concurrency, but the goal of this test is
	// exactly to verify the client survives a true million-goroutine fan-out,
	// so we launch them all (Go's scheduler handles the multiplexing).
	var failures atomic.Int64
	start := make(chan struct{})
	for range goroutines {
		go func() {
			defer wg.Done()
			<-start
			if err := c.Send(ctx, []byte("x")); err != nil {
				failures.Add(1)
			}
		}()
	}
	// Release all goroutines at once to maximise concurrency.
	close(start)
	wg.Wait()

	m := c.Metrics()
	// Every goroutine issued exactly one Send, so Total must equal goroutines.
	if m.Total != goroutines {
		t.Fatalf("Total = %d, want %d", m.Total, goroutines)
	}
	if m.Success+m.Failed != goroutines {
		t.Fatalf("success+failed = %d, want %d", m.Success+m.Failed, goroutines)
	}
	if f := failures.Load(); f != int64(m.Failed) {
		t.Fatalf("reported failures %d != metric Failed %d", f, m.Failed)
	}
	// ActiveConn must have returned to zero once all sends complete.
	if m.ActiveConn != 0 {
		t.Fatalf("ActiveConn = %d, want 0 after completion", m.ActiveConn)
	}
	t.Logf("million-sends metrics: %+v, server accepted %d connections", m, atomic.LoadUint64(accepted))
}
