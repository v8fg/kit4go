// This file is an internal benchmark/coverage test (package udpclient, not
// udpclient_test) so it can assert the new ActiveSends metric field and reach
// the unexported retryDelay helper if a backoff benchmark is added later. The
// networked benchmarks spin up a local UDP echo server to measure the real
// round-trip cost (socket write, kernel datagram handling, read) end to end.
package udpclient

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// benchEchoServer starts a UDP listener on an ephemeral port that reads one
// datagram at a time and writes it straight back to the sender. It returns the
// server's "host:port" address and a *uint64 counting datagrams received. The
// goroutine exits when the conn returned by newBenchUDPServer is closed by the
// caller's defer.
//
// This is the fixture for both Send (fire-and-forget) and SendReceive
// benchmarks: Send just writes and ignores any reply, while SendReceive
// writes then reads the echo back.
func benchEchoServer(tb testing.TB) (string, *uint64) {
	tb.Helper()
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("resolve: %v", err)
	}
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		tb.Fatalf("listen: %v", err)
	}
	addr := conn.LocalAddr().String()
	var received uint64
	go func() {
		buf := make([]byte, 4096)
		for {
			n, raddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return // conn closed by tb.Cleanup
			}
			if n > 0 {
				atomic.AddUint64(&received, 1)
			}
			if _, err := conn.WriteToUDP(buf[:n], raddr); err != nil {
				return
			}
		}
	}()
	tb.Cleanup(func() { _ = conn.Close() })
	return addr, &received
}

// benchOpts returns ClientOptions tuned for benchmarking: short timeouts so a
// wedged peer fails fast (rather than stalling the benchmark), tiny backoffs
// so the failure path doesn't sleep, and a generous read buffer.
func benchOpts(addr string) ClientOptions {
	return ClientOptions{
		Address:      addr,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,
		BufferSize:   4096,
		RetryMax:     0, // happy-path benchmarking; no retries on the hot path
		RetryWaitMin: time.Microsecond,
		RetryWaitMax: 10 * time.Microsecond,
	}
}

// --- Benchmarks --------------------------------------------------------------

// BenchmarkSend measures a fire-and-forget Send (write one datagram, no read).
// UDP sends to a reachable loopback peer are near-instant, so this primarily
// measures the per-call overhead: deadline set, write, counter increments,
// event-hook dispatch. The server echoes back but the client never reads it.
func BenchmarkSend(b *testing.B) {
	addr, received := benchEchoServer(b)
	c, err := NewClient(benchOpts(addr))
	if err != nil {
		b.Fatalf("NewClient: %v", err)
	}
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
	// Stop the timer before reading metrics so the atomic loads don't distort
	// the per-op average.
	b.StopTimer()
	m := c.Metrics()
	if m.Total != uint64(b.N) || m.Success != uint64(b.N) || m.Failed != 0 {
		b.Fatalf("metrics = %+v, want total=success=N failed=0", m)
	}
	// UDP Send is fire-and-forget: it returns once the kernel accepts the
	// datagram, but the echo server's ReadFromUDP is asynchronous, so the
	// server-side counter is a best-effort observer rather than a strict
	// invariant. We log it rather than asserting equality.
	b.Logf("server received %d datagrams (b.N=%d)", atomic.LoadUint64(received), b.N)
}

// BenchmarkSendReceive measures a write+read-one-datagram round-trip against
// the echo server. This is the realistic request/reply cost: deadline-set,
// write, deadline-set, read, copy into a right-sized reply slice.
func BenchmarkSendReceive(b *testing.B) {
	addr, _ := benchEchoServer(b)
	c, err := NewClient(benchOpts(addr))
	if err != nil {
		b.Fatalf("NewClient: %v", err)
	}
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

// BenchmarkSend_Parallel exercises Send under RunParallel. The underlying
// socket and its deadlines are shared across goroutines (documented on
// Client), so this benchmark surfaces any contention on the shared-conn write
// path or the metric atomics as a throughput drop.
func BenchmarkSend_Parallel(b *testing.B) {
	addr, _ := benchEchoServer(b)
	c, err := NewClient(benchOpts(addr))
	if err != nil {
		b.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	data := []byte("p")
	b.ReportAllocs()
	b.ResetTimer()
	var failures atomic.Int32
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := c.Send(ctx, data); err != nil {
				failures.Add(1)
			}
		}
	})
	b.StopTimer()
	if failures.Load() > 0 {
		b.Fatalf("parallel Send reported %d errors", failures.Load())
	}
	m := c.Metrics()
	if m.Success+m.Failed != m.Total {
		b.Fatalf("success+failed=%d != total=%d", m.Success+m.Failed, m.Total)
	}
}

// BenchmarkClient_Metrics measures the cost of taking a metrics snapshot — the
// five atomic loads (total/success/failed/retried/activeSends). This is the
// cost paid by every scrape of the client's observability surface, so it must
// stay negligible.
func BenchmarkClient_Metrics(b *testing.B) {
	addr, _ := benchEchoServer(b)
	c, err := NewClient(benchOpts(addr))
	if err != nil {
		b.Fatalf("NewClient: %v", err)
	}
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

// TestStress_HighRateSend drives 100,000 goroutines each issuing 10 Send calls
// (one million datagrams total) to a UDP echo server, then asserts the
// client's metrics account for every call. UDP is lossy by design, but on
// loopback with a responsive server the kernel does not drop under this load;
// any dropped datagram surfaces as a Send error (write timeout) and is counted
// in Failed. Skipped under -short so CI does not pay for it.
//
// Run manually with:
//
//	go test -run TestStress_HighRateSend -timeout 5m ./udpclient/
func TestStress_HighRateSend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping high-rate UDP stress test in -short mode")
	}
	const (
		goroutines = 100_000
		perGoro    = 10
		total      = goroutines * perGoro // 1,000,000
	)

	addr, received := benchEchoServer(t)
	c, err := NewClient(ClientOptions{
		Address:      addr,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		BufferSize:   4096,
		RetryMax:     2,
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var failures atomic.Int64
	start := make(chan struct{})
	for range goroutines {
		go func() {
			defer wg.Done()
			<-start
			for range perGoro {
				if err := c.Send(ctx, []byte("x")); err != nil {
					failures.Add(1)
				}
			}
		}()
	}
	close(start)
	wg.Wait()

	m := c.Metrics()
	if m.Total != total {
		t.Fatalf("Total = %d, want %d", m.Total, total)
	}
	if m.Success+m.Failed != total {
		t.Fatalf("success+failed = %d, want %d", m.Success+m.Failed, total)
	}
	if f := failures.Load(); f != int64(m.Failed) {
		t.Fatalf("reported failures %d != metric Failed %d", f, m.Failed)
	}
	// ActiveSends must have returned to zero once all sends complete.
	if m.ActiveSends != 0 {
		t.Fatalf("ActiveSends = %d, want 0 after completion", m.ActiveSends)
	}
	t.Logf("high-rate-send metrics: %+v, server received %d datagrams", m, atomic.LoadUint64(received))
}
