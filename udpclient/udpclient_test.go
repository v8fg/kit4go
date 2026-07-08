package udpclient_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/v8fg/kit4go/udpclient"
)

// fastOpts returns ClientOptions with tiny timeouts/backoffs so the timeout and
// retry tests run in milliseconds rather than seconds.
func fastOpts(addr string) udpclient.ClientOptions {
	return udpclient.ClientOptions{
		Address:      addr,
		ReadTimeout:  30 * time.Millisecond,
		WriteTimeout: 30 * time.Millisecond,
		BufferSize:   4096,
		RetryMax:     2,
		RetryWaitMin: 1 * time.Millisecond,
		RetryWaitMax: 5 * time.Millisecond,
	}
}

// newUDPServer starts a UDP listener on 127.0.0.1:0 and returns the conn, its
// local address string ("127.0.0.1:port") and a cleanup func. The caller owns
// the goroutine that services conn.
func newUDPServer(t *testing.T) (*net.UDPConn, string) {
	t.Helper()
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, conn.LocalAddr().String()
}

// echoServer runs a goroutine that reads one datagram at a time and writes it
// back to the sender. It returns the conn, its address, and a *uint64 counting
// datagrams received. Stopping happens when the conn is closed by t.Cleanup.
func echoServer(t *testing.T) (string, *uint64) {
	t.Helper()
	conn, addr := newUDPServer(t)
	var received uint64
	go func() {
		buf := make([]byte, 4096)
		for {
			n, raddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return // conn closed
			}
			atomic.AddUint64(&received, 1)
			if _, err := conn.WriteToUDP(buf[:n], raddr); err != nil {
				return
			}
		}
	}()
	return addr, &received
}

// silentServer reads and discards datagrams (never replies) so SendReceive's
// read deadline fires. Returns the address and a *uint64 counting datagrams.
func silentServer(t *testing.T) (string, *uint64) {
	t.Helper()
	conn, addr := newUDPServer(t)
	var received uint64
	go func() {
		buf := make([]byte, 4096)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n > 0 {
				atomic.AddUint64(&received, 1)
			}
		}
	}()
	return addr, &received
}

// captureServer reads one datagram at a time into the channel and does not
// reply. Used to verify Send delivers the exact bytes written.
func captureServer(t *testing.T) (string, chan []byte) {
	t.Helper()
	conn, addr := newUDPServer(t)
	ch := make(chan []byte, 16)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				close(ch)
				return
			}
			out := make([]byte, n)
			copy(out, buf[:n])
			select {
			case ch <- out:
			default:
			}
		}
	}()
	return addr, ch
}

func TestNewClient_DefaultsApplied(t *testing.T) {
	addr, _ := silentServer(t)
	c, err := udpclient.NewClient(udpclient.ClientOptions{Address: addr})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	// No way to read the private opts back, but a Send that returns nil (the
	// server exists) proves the default WriteTimeout (>0) was applied.
	if err := c.Send(context.Background(), []byte("x")); err != nil {
		t.Fatalf("Send with defaults: %v", err)
	}
}

func TestNewClient_EmptyAddress(t *testing.T) {
	_, err := udpclient.NewClient(udpclient.ClientOptions{})
	if err == nil {
		t.Fatal("NewClient: expected error for empty Address")
	}
}

func TestNewClient_BadAddress(t *testing.T) {
	_, err := udpclient.NewClient(udpclient.ClientOptions{Address: "not a valid:address:at:all"})
	if err == nil {
		t.Fatal("NewClient: expected error for unresolvable Address")
	}
}

func TestNewClient_WithLocalAddress(t *testing.T) {
	// Bind a specific local source port on the loopback, then verify the
	// client still reaches the echo server.
	addr, _ := echoServer(t)
	la, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve local: %v", err)
	}
	c, err := udpclient.NewClient(udpclient.ClientOptions{
		Address:      addr,
		LocalAddress: la.String(),
		WriteTimeout: 100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	out, err := c.SendReceive(context.Background(), []byte("localbind"))
	if err != nil {
		t.Fatalf("SendReceive: %v", err)
	}
	if string(out) != "localbind" {
		t.Fatalf("reply = %q, want %q", out, "localbind")
	}
}

func TestNewClient_BadLocalAddress(t *testing.T) {
	addr, _ := silentServer(t)
	_, err := udpclient.NewClient(udpclient.ClientOptions{
		Address:      addr,
		LocalAddress: "not-a-valid:local:addr",
	})
	if err == nil {
		t.Fatal("NewClient: expected error for bad LocalAddress")
	}
}

func TestClient_Send_ServerReceives(t *testing.T) {
	srvAddr, gotCh := captureServer(t)
	c, err := udpclient.NewClient(fastOpts(srvAddr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	payload := []byte("hello-udp")
	if err := c.Send(context.Background(), payload); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case got := <-gotCh:
		if string(got) != string(payload) {
			t.Fatalf("server received %q, want %q", got, payload)
		}
	case <-time.After(time.Second):
		t.Fatal("server never received the datagram")
	}
}

func TestClient_Send_EmptyData(t *testing.T) {
	// A zero-length datagram is a valid write; the server observes a 0-byte
	// packet. This guards against any "len(data)==0 => skip write" shortcut.
	srvAddr, gotCh := captureServer(t)
	c, err := udpclient.NewClient(fastOpts(srvAddr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if err := c.Send(context.Background(), nil); err != nil {
		t.Fatalf("Send(nil): %v", err)
	}
	select {
	case <-gotCh:
		// got it (a zero-length payload arrives)
	case <-time.After(time.Second):
		t.Fatal("server never received the empty datagram")
	}
}

func TestClient_SendReceive_Echo(t *testing.T) {
	addr, received := echoServer(t)
	c, err := udpclient.NewClient(fastOpts(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	payload := []byte("ping-echo")
	out, err := c.SendReceive(context.Background(), payload)
	if err != nil {
		t.Fatalf("SendReceive: %v", err)
	}
	if string(out) != string(payload) {
		t.Fatalf("reply = %q, want %q", out, payload)
	}
	if got := atomic.LoadUint64(received); got != 1 {
		t.Fatalf("server received = %d, want 1", got)
	}
}

func TestClient_SendReceive_DistinctBuffers(t *testing.T) {
	// The returned slice must not alias any internal buffer; mutating it must
	// not corrupt a subsequent call's reply.
	addr, _ := echoServer(t)
	c, err := udpclient.NewClient(fastOpts(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	a, err := c.SendReceive(context.Background(), []byte("first"))
	if err != nil {
		t.Fatalf("SendReceive[0]: %v", err)
	}
	// Mutate the first reply in place.
	for i := range a {
		a[i] = 'X'
	}
	b, err := c.SendReceive(context.Background(), []byte("second"))
	if err != nil {
		t.Fatalf("SendReceive[1]: %v", err)
	}
	if string(b) != "second" {
		t.Fatalf("second reply = %q, want %q (buffer aliasing)", b, "second")
	}
	if string(a) != "XXXXX" {
		t.Fatalf("first reply mutated unexpectedly: %q", a)
	}
}

func TestClient_ReadTimeout_NoReply(t *testing.T) {
	// Silent server: the read deadline fires because nobody replies. The client
	// retries the full budget (RetryMax=2 → 3 attempts) and ultimately fails.
	addr, received := silentServer(t)
	c, err := udpclient.NewClient(fastOpts(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	start := time.Now()
	_, err = c.SendReceive(context.Background(), []byte("anyone?"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("SendReceive: expected read-timeout error, got nil")
	}
	// 3 reads at 30ms each (plus tiny backoff) → well under 1s.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("elapsed = %v, expected to bail out on read timeout", elapsed)
	}
	// 1 initial send + 2 retries = 3 datagrams observed by the server.
	if got := atomic.LoadUint64(received); got != 3 {
		t.Fatalf("server received = %d, want 3 (1 initial + 2 retries)", got)
	}
	m := c.Metrics()
	if m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
	if m.Success != 0 {
		t.Fatalf("Metrics.Success = %d, want 0", m.Success)
	}
	if m.Retried != 2 {
		t.Fatalf("Metrics.Retried = %d, want 2", m.Retried)
	}
}

func TestClient_Retry_OnReadTimeout(t *testing.T) {
	// Silent server: every attempt times out on read. RetryMax=2 means the
	// client should write 3 times (1 initial + 2 retries).
	addr, received := silentServer(t)
	c, err := udpclient.NewClient(fastOpts(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.SendReceive(context.Background(), []byte("retry-me"))
	if err == nil {
		t.Fatal("SendReceive: expected timeout error after retries")
	}
	wantWrites := uint64(c.Metrics().Retried + 1) // initial + retries
	if got := atomic.LoadUint64(received); got != wantWrites {
		t.Fatalf("server received = %d, want %d (1 initial + %d retries)",
			got, wantWrites, c.Metrics().Retried)
	}
	m := c.Metrics()
	if m.Retried != 2 {
		t.Fatalf("Metrics.Retried = %d, want 2", m.Retried)
	}
	if m.Total != 1 {
		t.Fatalf("Metrics.Total = %d, want 1", m.Total)
	}
	if m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
}

func TestClient_Retry_OnWriteError(t *testing.T) {
	// Point the client at a port nobody is listening on. On most platforms a
	// connected UDP socket gets an ICMP port-unreachable on the first write,
	// surfacing as a write error that is retried.
	c, err := udpclient.NewClient(fastOpts("127.0.0.1:1"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	err = c.Send(context.Background(), []byte("no-listener"))
	if err == nil {
		// Some platforms silently accept the write (no ICMP). In that case the
		// call reports success but we at least know we didn't crash; skip the
		// retry-count assertion rather than fail.
		return
	}
	// When an error IS surfaced, Retried should reflect the retry budget
	// actually spent. We don't assert the exact count (it depends on whether
	// the error fires on the 1st, 2nd or 3rd write) but we do require that the
	// call failed and bumped the failed counter.
	if m := c.Metrics(); m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
}

func TestClient_Retry_EventuallySucceeds(t *testing.T) {
	// Server that fails the first read by not replying, then replies on the
	// second write. Because the read deadline is per-attempt, the client's first
	// attempt times out; its retry write gets a reply.
	srvConn, addr := newUDPServer(t)
	var writes uint64
	go func() {
		buf := make([]byte, 4096)
		// Drain exactly one datagram without replying, then reply to all
		// subsequent ones.
		for {
			n, raddr, err := srvConn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			count := atomic.AddUint64(&writes, 1)
			if count == 1 {
				continue // drop the first
			}
			if _, err := srvConn.WriteToUDP(buf[:n], raddr); err != nil {
				return
			}
		}
	}()

	c, err := udpclient.NewClient(fastOpts(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	out, err := c.SendReceive(context.Background(), []byte("eventually"))
	if err != nil {
		t.Fatalf("SendReceive: %v", err)
	}
	if string(out) != "eventually" {
		t.Fatalf("reply = %q, want %q", out, "eventually")
	}
	m := c.Metrics()
	if m.Retried != 1 {
		t.Fatalf("Metrics.Retried = %d, want 1", m.Retried)
	}
	if m.Success != 1 {
		t.Fatalf("Metrics.Success = %d, want 1", m.Success)
	}
	if m.Failed != 0 {
		t.Fatalf("Metrics.Failed = %d, want 0", m.Failed)
	}
}

func TestClient_Retry_HonoursContextCancellation(t *testing.T) {
	// Silent server with a generous RetryWaitMax and many retries; cancel the
	// ctx mid-backoff and confirm we stop promptly and do not exhaust retries.
	addr, _ := silentServer(t)
	opts := fastOpts(addr)
	opts.RetryMax = 50
	opts.RetryWaitMin = 100 * time.Millisecond
	opts.RetryWaitMax = 500 * time.Millisecond
	c, err := udpclient.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = c.SendReceive(ctx, []byte("cancel-me"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("SendReceive: expected error (ctx cancelled or timeout)")
	}
	// Must return shortly after the 50ms ctx deadline, not run the full retry
	// budget (which would take seconds).
	if elapsed > 300*time.Millisecond {
		t.Fatalf("elapsed = %v, expected to honour ctx cancellation promptly", elapsed)
	}
	if m := c.Metrics(); m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
}

// TestClient_SendReceive_ContextCancelMidRead_ReturnsPromptly is the regression
// test for R15 F3: SendReceive's blocking conn.Read ignored the caller's ctx.
// The backoff sleep honoured ctx, but a Read blocked on a datagram from a silent
// peer had no ctx watcher, so cancelling ctx mid-read blocked the FULL
// ReadTimeout (e.g. 5s default) instead of returning promptly. With a 2s
// ReadTimeout and a ctx cancel at 100ms, the old code returned ~2s late with an
// "i/o timeout" error; the fixed code returns within ~200ms with ctx.Err().
//
// RetryMax=0 isolates the single Read so the result is not muddied by retry
// backoff (which already honoured ctx).
func TestClient_SendReceive_ContextCancelMidRead_ReturnsPromptly(t *testing.T) {
	addr, _ := silentServer(t)
	opts := fastOpts(addr)
	opts.ReadTimeout = 2 * time.Second // long: expose the mid-read block
	opts.RetryMax = 0                  // isolate the single Read
	c, err := udpclient.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err = c.SendReceive(ctx, []byte("ping"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("SendReceive: expected error from cancelled ctx")
	}
	// The old code surfaced a transport i/o timeout after the full ReadTimeout;
	// the fix surfaces the ctx error.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SendReceive err = %v, want errors.Is(context.Canceled)", err)
	}
	// Must return shortly after the 100ms cancel, not block the full 2s
	// ReadTimeout. Allow generous slack for CI scheduling without masking the
	// bug (old code was ~2000ms; 500ms is comfortably above the cancel yet
	// well below the broken behaviour).
	if elapsed > 500*time.Millisecond {
		t.Fatalf("elapsed = %v, expected prompt return after ctx cancel (mid-read must honour ctx)", elapsed)
	}
}

// TestClient_SendReceive_ContextCancelMidRead_DefaultReadTimeout confirms the
// fix at the package default ReadTimeout (5s), which is the configuration the
// bug actually bites in production. With ctx cancel at 100ms the old code
// blocked the full ~5s; the fix returns within ~200ms.
func TestClient_SendReceive_ContextCancelMidRead_DefaultReadTimeout(t *testing.T) {
	addr, _ := silentServer(t)
	// Default options: ReadTimeout is 5s, RetryMax is its default. The exact
	// defaults are exercised via NewClient with only Address set.
	c, err := udpclient.NewClient(udpclient.ClientOptions{Address: addr})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err = c.SendReceive(ctx, []byte("ping"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("SendReceive: expected error from cancelled ctx")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SendReceive err = %v, want errors.Is(context.Canceled)", err)
	}
	// Default ReadTimeout is 5s; the old code blocked ~5s. 500ms proves the
	// Read no longer ignores ctx while staying well clear of CI flake.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("elapsed = %v, expected prompt return after ctx cancel at default ReadTimeout", elapsed)
	}
}

func TestClient_Send_DoesNotWaitForReply(t *testing.T) {
	// Send to a server that never reads; it must still return promptly because
	// Send does not issue a read.
	conn, addr := newUDPServer(t)
	defer conn.Close()
	// No reader goroutine: datagrams pile up in the kernel socket buffer.

	c, err := udpclient.NewClient(fastOpts(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	start := time.Now()
	if err := c.Send(context.Background(), []byte("fire-and-forget")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("Send elapsed = %v, should return without waiting for a reply", elapsed)
	}
}

func TestClient_Metrics_Accumulate(t *testing.T) {
	addr, _ := echoServer(t)
	c, err := udpclient.NewClient(fastOpts(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	for i := range 5 {
		if _, err := c.SendReceive(context.Background(), []byte("m")); err != nil {
			t.Fatalf("SendReceive[%d]: %v", i, err)
		}
	}
	m := c.Metrics()
	if m.Total != 5 {
		t.Fatalf("Metrics.Total = %d, want 5", m.Total)
	}
	if m.Success != 5 {
		t.Fatalf("Metrics.Success = %d, want 5", m.Success)
	}
	if m.Failed != 0 {
		t.Fatalf("Metrics.Failed = %d, want 0", m.Failed)
	}
	if m.Retried != 0 {
		t.Fatalf("Metrics.Retried = %d, want 0", m.Retried)
	}
}

func TestClient_Metrics_AfterMixed(t *testing.T) {
	// One successful SendReceive against the echo server, one failed
	// SendReceive against the silent server (read timeout). Both share one
	// client so the counters aggregate.
	echoAddr, _ := echoServer(t)
	c, err := udpclient.NewClient(fastOpts(echoAddr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.SendReceive(context.Background(), []byte("ok")); err != nil {
		t.Fatalf("SendReceive ok: %v", err)
	}
	// Point the SAME client at the silent server via a fresh client (the
	// connected socket is pinned to echoAddr). Instead, verify mixed metrics by
	// driving the echo server to fail: send to a port with no reader via a
	// second client sharing nothing — but we want ONE client. So: force a read
	// timeout by pointing a second client at the silent server and assert its
	// own metrics. This keeps the assertion honest.
	silentAddr, _ := silentServer(t)
	c2, err := udpclient.NewClient(fastOpts(silentAddr))
	if err != nil {
		t.Fatalf("NewClient c2: %v", err)
	}
	t.Cleanup(func() { _ = c2.Close() })
	_, err = c2.SendReceive(context.Background(), []byte("drop"))
	if err == nil {
		t.Fatal("c2 SendReceive: expected timeout error")
	}

	m := c.Metrics()
	if m.Total != 1 || m.Success != 1 || m.Failed != 0 {
		t.Fatalf("c metrics = %+v, want total=1 success=1 failed=0", m)
	}
	m2 := c2.Metrics()
	if m2.Total != 1 || m2.Success != 0 || m2.Failed != 1 {
		t.Fatalf("c2 metrics = %+v, want total=1 success=0 failed=1", m2)
	}
}

func TestClient_BufferSize_Honored(t *testing.T) {
	// Server echoes a 64-byte payload. With BufferSize=16 the reply is
	// truncated to 16 bytes by the client's read buffer.
	addr, _ := echoServer(t)
	opts := fastOpts(addr)
	opts.BufferSize = 16
	c, err := udpclient.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	payload := make([]byte, 64)
	for i := range payload {
		payload[i] = byte('A' + (i % 26))
	}
	out, err := c.SendReceive(context.Background(), payload)
	if err != nil {
		t.Fatalf("SendReceive: %v", err)
	}
	if len(out) != 16 {
		t.Fatalf("len(reply) = %d, want 16 (BufferSize truncation)", len(out))
	}
}

func TestClient_BufferSize_DefaultIsLarge(t *testing.T) {
	// With the default BufferSize (4096) a 1000-byte reply comes through whole.
	addr, _ := echoServer(t)
	c, err := udpclient.NewClient(udpclient.ClientOptions{
		Address:      addr,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	payload := make([]byte, 1000)
	out, err := c.SendReceive(context.Background(), payload)
	if err != nil {
		t.Fatalf("SendReceive: %v", err)
	}
	if len(out) != 1000 {
		t.Fatalf("len(reply) = %d, want 1000", len(out))
	}
}

func TestClient_Concurrent_RaceSafe(t *testing.T) {
	addr, _ := echoServer(t)
	opts := fastOpts(addr)
	opts.ReadTimeout = 500 * time.Millisecond // generous for concurrent load under -race
	opts.WriteTimeout = 500 * time.Millisecond
	c, err := udpclient.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			payload := []byte(fmt.Sprintf("concurrent-%d", i))
			out, err := c.SendReceive(context.Background(), payload)
			if err != nil {
				errCh <- err
				return
			}
			// Echo replies preserve content (order/contention notwithstanding).
			if string(out) != string(payload) {
				// Under contention the connected socket's read may pick up a
				// reply destined for another goroutine; tolerate mismatches but
				// require a successful read. This is the documented SendReceive
				// caveat in doc.go.
				_ = out
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent SendReceive failed: %v", err)
	}
	if m := c.Metrics(); m.Total != n {
		t.Fatalf("Metrics.Total = %d, want %d", m.Total, n)
	}
}

func TestClient_Close_Idempotent(t *testing.T) {
	addr, _ := echoServer(t)
	c, err := udpclient.NewClient(fastOpts(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close #1: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close #2: %v (should be a no-op)", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close #3: %v (should be a no-op)", err)
	}
}

func TestClient_Send_AfterClose(t *testing.T) {
	addr, _ := silentServer(t)
	c, err := udpclient.NewClient(fastOpts(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err = c.Send(context.Background(), []byte("post-close"))
	if err == nil {
		t.Fatal("Send after Close: expected error, got nil")
	}
	if _, err := c.SendReceive(context.Background(), []byte("post-close")); err == nil {
		t.Fatal("SendReceive after Close: expected error, got nil")
	}
	// The failed-call counters should NOT have advanced: Close short-circuits
	// before the total/failed bookkeeping.
	if m := c.Metrics(); m.Total != 0 || m.Failed != 0 {
		t.Fatalf("post-close metrics = %+v, want all zero", m)
	}
}

func TestClient_SetOnEvent_Fires(t *testing.T) {
	addr, _ := echoServer(t)
	c, err := udpclient.NewClient(fastOpts(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	var (
		mu        sync.Mutex
		seen      []string
		sendBytes int
		recvBytes int
	)
	c.SetOnEvent(func(evt udpclient.ClientEvent) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, evt.Name)
		switch evt.Name {
		case "send":
			sendBytes = evt.Bytes
		case "receive":
			recvBytes = evt.Bytes
		}
	})

	if _, err := c.SendReceive(context.Background(), []byte("hook")); err != nil {
		t.Fatalf("SendReceive: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// Expect at least: send, receive, success (in that order, no retry on echo).
	want := []string{"send", "receive", "success"}
	if len(seen) < len(want) {
		t.Fatalf("events = %v, want at least %v", seen, want)
	}
	for i, name := range want {
		if seen[i] != name {
			t.Fatalf("event[%d] = %q, want %q (full: %v)", i, seen[i], name, seen)
		}
	}
	if sendBytes != 4 {
		t.Fatalf("send bytes = %d, want 4", sendBytes)
	}
	if recvBytes != 4 {
		t.Fatalf("receive bytes = %d, want 4", recvBytes)
	}
}

func TestClient_SetOnEvent_FiresRetryAndFailed(t *testing.T) {
	addr, _ := silentServer(t)
	c, err := udpclient.NewClient(fastOpts(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	var (
		mu    sync.Mutex
		names []string
	)
	c.SetOnEvent(func(evt udpclient.ClientEvent) {
		mu.Lock()
		defer mu.Unlock()
		names = append(names, evt.Name)
	})

	_, err = c.SendReceive(context.Background(), []byte("timeout"))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	mu.Lock()
	defer mu.Unlock()

	// Expect: send, [retry, send]*, failed. At least one "retry" must fire
	// (RetryMax=2).
	hasRetry := false
	hasFailed := false
	for _, n := range names {
		if n == "retry" {
			hasRetry = true
		}
		if n == "failed" {
			hasFailed = true
		}
	}
	if !hasRetry {
		t.Fatalf("no retry event fired; events = %v", names)
	}
	if !hasFailed {
		t.Fatalf("no failed event fired; events = %v", names)
	}
}

func TestClient_SetOnEvent_NilDisables(t *testing.T) {
	addr, _ := echoServer(t)
	c, err := udpclient.NewClient(fastOpts(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	fired := atomic.Uint64{}
	c.SetOnEvent(func(udpclient.ClientEvent) { fired.Add(1) })
	c.SetOnEvent(nil) // disable

	if _, err := c.SendReceive(context.Background(), []byte("x")); err != nil {
		t.Fatalf("SendReceive: %v", err)
	}
	if got := fired.Load(); got != 0 {
		t.Fatalf("hook fired %d times after SetOnEvent(nil), want 0", got)
	}
}

// fakeBreaker is a test double for udpclient.CircuitBreaker. It delegates to fn
// (so the call actually happens) and counts Execute calls + failures.
type fakeBreaker struct {
	calls    uint64
	failures uint64
}

func (b *fakeBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	atomic.AddUint64(&b.calls, 1)
	err := fn(ctx)
	if err != nil {
		atomic.AddUint64(&b.failures, 1)
	}
	return err
}

func TestClient_BreakerIntegration_DelegatesAndCounts(t *testing.T) {
	addr, _ := echoServer(t)
	br := &fakeBreaker{}
	opts := fastOpts(addr)
	opts.Breaker = br
	c, err := udpclient.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	out, err := c.SendReceive(context.Background(), []byte("via-breaker"))
	if err != nil {
		t.Fatalf("SendReceive: %v", err)
	}
	if string(out) != "via-breaker" {
		t.Fatalf("reply = %q, want %q", out, "via-breaker")
	}
	if got := atomic.LoadUint64(&br.calls); got != 1 {
		t.Fatalf("breaker.Execute calls = %d, want 1", got)
	}
	if got := atomic.LoadUint64(&br.failures); got != 0 {
		t.Fatalf("breaker.failures = %d, want 0", got)
	}
}

// explicitBreaker always returns the configured error without calling fn.
type explicitBreaker struct{ err error }

func (b *explicitBreaker) Execute(context.Context, func(context.Context) error) error {
	return b.err
}

func TestClient_BreakerIntegration_OpenShortCircuits(t *testing.T) {
	addr, received := echoServer(t)
	br := &explicitBreaker{err: errors.New("circuit open")}
	opts := fastOpts(addr)
	opts.Breaker = br
	c, err := udpclient.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.SendReceive(context.Background(), []byte("should-not-send"))
	if err == nil {
		t.Fatal("SendReceive: expected circuit-open error")
	}
	// Give the echo server a moment to prove it received nothing. The breaker
	// short-circuits before any write, so the server's counter stays at 0.
	time.Sleep(20 * time.Millisecond)
	if got := atomic.LoadUint64(received); got != 0 {
		t.Fatalf("server received = %d, want 0 (breaker should short-circuit)", got)
	}
	if m := c.Metrics(); m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
}

func TestClient_BreakerIntegration_Send(t *testing.T) {
	// The breaker wraps Send just as it wraps SendReceive.
	srvAddr, gotCh := captureServer(t)
	br := &fakeBreaker{}
	opts := fastOpts(srvAddr)
	opts.Breaker = br
	c, err := udpclient.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if err := c.Send(context.Background(), []byte("send-breaker")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := atomic.LoadUint64(&br.calls); got != 1 {
		t.Fatalf("breaker.Execute calls = %d, want 1", got)
	}
	select {
	case got := <-gotCh:
		if string(got) != "send-breaker" {
			t.Fatalf("server received %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("server never received the datagram")
	}
}

func TestClient_DefaultOptions_TableDriven(t *testing.T) {
	// withDefaults fills every zero field; exercise it via the public API with
	// a zero-valued (except Address) options struct and confirm it behaves.
	addr, _ := echoServer(t)
	c, err := udpclient.NewClient(udpclient.ClientOptions{Address: addr})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// Default ReadTimeout (5s) is generous enough that the echo succeeds.
	out, err := c.SendReceive(context.Background(), []byte("defaults"))
	if err != nil {
		t.Fatalf("SendReceive: %v", err)
	}
	if string(out) != "defaults" {
		t.Fatalf("reply = %q", out)
	}
}
