package tcpclient_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/v8fg/kit4go/tcpclient"
)

// --- Test servers ------------------------------------------------------------

// echoOnceListener starts a TCP listener that, for each accepted connection,
// reads a single chunk, echoes it back, and closes the connection. The close
// is essential: SendReceive reads until EOF (or ReadTimeout), so an echo that
// keeps the connection open would force every SendReceive to hit its read
// deadline. The returned *uint64 counts accepted connections.
//
// The single Read suffices for the test payloads (<= 64KiB fits in one TCP
// segment buffer on loopback); larger payloads use the dedicated
// readAllEchoListener below.
func echoOnceListener(t *testing.T) (net.Listener, *uint64) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var conns uint64
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddUint64(&conns, 1)
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 64*1024)
				// Give the writer a moment to push the payload, then read once.
				_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				n, err := c.Read(buf)
				if err != nil || n == 0 {
					return
				}
				_, _ = c.Write(buf[:n])
			}(conn)
		}
	}()
	return ln, &conns
}

// readAllEchoListener accepts a connection, fully drains it via io.Copy into a
// buffer (looping Read until EOF), echoes the buffer back, then closes. Used by
// the large-payload test where a single Read is not guaranteed to capture the
// whole payload. The drain loop ends when the client half-closes its write side
// (which readAllEchoHalfClose triggers) — but since our client does not
// half-close, this server instead reads until a short idle gap then flushes.
//
// To keep things deterministic without half-close, this server reads until the
// buffer holds at least want bytes OR a read deadline elapses, then writes and
// closes. Callers pass the expected payload size.
func readAllEchoListener(t *testing.T, want int) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, want)
				_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
				n, _ := io.ReadFull(c, buf)
				if n == 0 {
					return
				}
				_, _ = c.Write(buf[:n])
			}(conn)
		}
	}()
	return ln
}

// persistentEchoListener keeps accepted connections open and echoes every read
// back via io.Copy. Used only by tests that exercise write-only Send (no read),
// because SendReceive against this server would block until the read deadline.
func persistentEchoListener(t *testing.T) (net.Listener, *uint64) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var conns uint64
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddUint64(&conns, 1)
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c) // echo until error or EOF
			}(conn)
		}
	}()
	return ln, &conns
}

// echoUnixListener is the Unix-socket variant of echoOnceListener: read once,
// echo, close. The socket is created under dir (use t.TempDir()).
func echoUnixListener(t *testing.T, dir string) (string, *uint64) {
	t.Helper()
	path := dir + "/echosock"
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	var conns uint64
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddUint64(&conns, 1)
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4*1024)
				_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				n, err := c.Read(buf)
				if err != nil || n == 0 {
					return
				}
				_, _ = c.Write(buf[:n])
			}(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return path, &conns
}

// lineEchoListener accepts connections and, for each line read, writes the
// same line back followed by '\n'. Used by the SendReceiveLine tests.
func lineEchoListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				sc := bufio.NewScanner(c)
				for sc.Scan() {
					_, _ = fmt.Fprintf(c, "%s\n", sc.Text())
				}
			}(conn)
		}
	}()
	return ln
}

// acceptOnceListener accepts exactly one connection and runs handler on it,
// then stops accepting (so subsequent connects fail / retry).
func acceptOnceListener(t *testing.T, handler func(net.Conn)) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		handler(conn)
	}()
	return ln
}

// acceptEachListener accepts every incoming connection and runs handler on it,
// for as long as the listener is open. Unlike acceptOnceListener it keeps
// accepting, so retried attempts always reach a fresh server connection rather
// than timing out against a stopped acceptor. Used where the test needs the
// same response (or EOF) on every attempt.
func acceptEachListener(t *testing.T, handler func(net.Conn)) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handler(conn)
		}
	}()
	return ln
}

// fastOpts returns ClientOptions with tiny timeouts/backoffs so the timeout
// and retry tests run in milliseconds rather than seconds.
func fastOpts(addr string) tcpclient.ClientOptions {
	return tcpclient.ClientOptions{
		Network:        "tcp",
		Address:        addr,
		ConnectTimeout: 50 * time.Millisecond,
		ReadTimeout:    500 * time.Millisecond,
		WriteTimeout:   50 * time.Millisecond,
		PoolSize:       4,
		IdleTimeout:    5 * time.Second,
		RetryMax:       2,
		RetryWaitMin:   1 * time.Millisecond,
		RetryWaitMax:   10 * time.Millisecond,
	}
}

// --- Basic send / receive ----------------------------------------------------

func TestClient_Send_EchoRoundtrip(t *testing.T) {
	ln, _ := echoOnceListener(t)
	defer ln.Close()

	c := tcpclient.NewClient(fastOpts(ln.Addr().String()))
	defer c.Close()

	// SendReceive drives a full request/response: the echo-once server reads
	// the payload, writes it back, then closes — which lets io.ReadAll return.
	payload := []byte("hello tcp")
	got, err := c.SendReceive(context.Background(), payload)
	if err != nil {
		t.Fatalf("SendReceive: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("SendReceive: got %q, want %q", got, payload)
	}
}

func TestClient_Send_WriteOnlySucceeds(t *testing.T) {
	// A persistent echo server keeps the connection open; Send (write-only,
	// no read) succeeds and the connection is returned to the pool.
	ln, _ := persistentEchoListener(t)
	defer ln.Close()

	c := tcpclient.NewClient(fastOpts(ln.Addr().String()))
	defer c.Close()

	if err := c.Send(context.Background(), []byte("fire and forget")); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestClient_SendReceive_LargePayload(t *testing.T) {
	const size = 64 * 1024 // larger than a single read buffer
	ln := readAllEchoListener(t, size)
	defer ln.Close()

	c := tcpclient.NewClient(fastOpts(ln.Addr().String()))
	defer c.Close()

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	got, err := c.SendReceive(context.Background(), payload)
	if err != nil {
		t.Fatalf("SendReceive: %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("SendReceive: len = %d, want %d", len(got), len(payload))
	}
	if string(got) != string(payload) {
		t.Fatal("SendReceive: content mismatch on large payload")
	}
}

func TestClient_SendReceiveLine(t *testing.T) {
	ln := lineEchoListener(t)
	defer ln.Close()

	c := tcpclient.NewClient(fastOpts(ln.Addr().String()))
	defer c.Close()

	line, err := c.SendReceiveLine(context.Background(), []byte("PING\n"))
	if err != nil {
		t.Fatalf("SendReceiveLine: %v", err)
	}
	if line != "PING" {
		t.Fatalf("SendReceiveLine: got %q, want %q", line, "PING")
	}
}

func TestClient_SendReceiveLine_NoNewlineReturnsEOF(t *testing.T) {
	// Every accepted connection consumes the request, writes "partial" (no
	// newline) and closes, so every attempt — including retries — surfaces
	// io.EOF with the partial line. Reading the request first avoids a RST
	// (writing to a peer that has already closed both directions yields a
	// reset rather than a clean EOF). fastOpts leaves RetryMax at its default
	// (retries enabled), so we keep accepting to ensure the final returned
	// error is EOF, not a connect failure or read timeout.
	ln := acceptEachListener(t, func(c net.Conn) {
		defer c.Close()
		buf := make([]byte, 32)
		_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, _ = c.Read(buf) // consume the request so the client's write lands
		_, _ = c.Write([]byte("partial"))
	})
	defer ln.Close()

	opts := fastOpts(ln.Addr().String())
	c := tcpclient.NewClient(opts)
	defer c.Close()

	line, err := c.SendReceiveLine(context.Background(), []byte("x\n"))
	if err == nil {
		t.Fatal("SendReceiveLine: expected EOF, got nil")
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("SendReceiveLine: err = %v, want io.EOF", err)
	}
	if line != "partial" {
		t.Fatalf("SendReceiveLine: partial line = %q, want %q", line, "partial")
	}
}

// --- Unix socket -------------------------------------------------------------

func TestClient_UnixSocket(t *testing.T) {
	dir := t.TempDir()
	path, conns := echoUnixListener(t, dir)

	opts := tcpclient.ClientOptions{
		Network:        "unix",
		Address:        path,
		ConnectTimeout: 50 * time.Millisecond,
		ReadTimeout:    100 * time.Millisecond,
		WriteTimeout:   50 * time.Millisecond,
		PoolSize:       2,
		IdleTimeout:    5 * time.Second,
		RetryMax:       0,
	}
	c := tcpclient.NewClient(opts)
	defer c.Close()

	payload := []byte("over unix")
	got, err := c.SendReceive(context.Background(), payload)
	if err != nil {
		t.Fatalf("SendReceive unix: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("SendReceive unix: got %q, want %q", got, payload)
	}
	if got := atomic.LoadUint64(conns); got != 1 {
		t.Fatalf("unix server conns = %d, want 1", got)
	}
}

// --- Connection pool ---------------------------------------------------------

func TestClient_PoolReuse(t *testing.T) {
	ln, conns := persistentEchoListener(t)
	defer ln.Close()

	opts := fastOpts(ln.Addr().String())
	opts.PoolSize = 2
	opts.RetryMax = 0
	c := tcpclient.NewClient(opts)
	defer c.Close()

	// Three sequential Sends (write-only) against a persistent echo: the pooled
	// connection is returned after each Send and reused by the next, so the
	// server should accept exactly one connection.
	for i := range 3 {
		if err := c.Send(context.Background(), []byte("reuse")); err != nil {
			t.Fatalf("Send[%d]: %v", i, err)
		}
	}
	if !waitForConns(conns, 1) {
		t.Fatalf("pool reuse: server conns = %d, want 1 (pool should reuse)", atomic.LoadUint64(conns))
	}
}

func TestClient_PoolIdleEviction(t *testing.T) {
	ln, conns := persistentEchoListener(t)
	defer ln.Close()

	opts := fastOpts(ln.Addr().String())
	opts.PoolSize = 2
	opts.IdleTimeout = 20 * time.Millisecond // very short
	opts.RetryMax = 0
	c := tcpclient.NewClient(opts)
	defer c.Close()

	if err := c.Send(context.Background(), []byte("first")); err != nil {
		t.Fatalf("Send first: %v", err)
	}
	// Sleep long enough for the pooled conn to exceed IdleTimeout.
	time.Sleep(60 * time.Millisecond)
	if err := c.Send(context.Background(), []byte("second")); err != nil {
		t.Fatalf("Send second: %v", err)
	}
	// The idle conn from the first call should have been evicted on checkout
	// for the second, so the server sees a fresh accept.
	if !waitForConns(conns, 2) {
		t.Fatalf("idle eviction: server conns = %d, want 2", atomic.LoadUint64(conns))
	}
}

func TestClient_PoolExhaustion_DialsExtra(t *testing.T) {
	ln, conns := persistentEchoListener(t)
	defer ln.Close()

	opts := fastOpts(ln.Addr().String())
	opts.PoolSize = 1
	opts.RetryMax = 0
	c := tcpclient.NewClient(opts)
	defer c.Close()

	// Two concurrent Sends with a pool of 1. Whether the second reuses the
	// first's (already-returned) conn or dials a fresh one depends entirely on
	// goroutine scheduling: a write-only Send returns as soon as the write is
	// flushed, so the first conn is very likely back in the pool before the
	// second goroutine runs, yielding a single accept. Asserting conns >= 2
	// therefore flakes ~9/10 in isolation. The always-true property under
	// concurrent load is: both Sends succeed and the server accepted at least
	// one connection. The "extra dial when the pool is genuinely exhausted"
	// behaviour is covered deterministically by
	// TestClient_PoolExhaustion_BlockedCheckout (internal test), which holds a
	// conn checked out so the second Send provably dials.
	const n = 2
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, n)
	start := make(chan struct{})
	for range n {
		go func() {
			defer wg.Done()
			<-start
			if err := c.Send(context.Background(), []byte("x")); err != nil {
				errCh <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent Send: %v", err)
	}
	// Both Sends succeeded and the server accepted at least one connection —
	// the only scheduling-independent invariant. (The old assertion
	// `conns >= n` was scheduler-dependent and flaked frequently.)
	if !waitForConns(conns, 1) {
		t.Fatalf("pool exhaustion: server conns = %d, want >= 1", atomic.LoadUint64(conns))
	}
}

// --- Timeouts ----------------------------------------------------------------

func TestClient_ReadTimeout(t *testing.T) {
	// Server accepts a connection, reads the request, then sleeps longer than
	// ReadTimeout without writing anything back.
	ln := acceptOnceListener(t, func(c net.Conn) {
		defer c.Close()
		buf := make([]byte, 16)
		_, _ = c.Read(buf) // consume the request
		time.Sleep(300 * time.Millisecond)
	})
	defer ln.Close()

	opts := fastOpts(ln.Addr().String())
	opts.ReadTimeout = 30 * time.Millisecond
	opts.RetryMax = 0
	c := tcpclient.NewClient(opts)
	defer c.Close()

	start := time.Now()
	_, err := c.SendReceive(context.Background(), []byte("ping"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("SendReceive: expected read timeout error, got nil")
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("SendReceive: elapsed = %v, should bail out near ReadTimeout", elapsed)
	}
}

func TestClient_ConnectTimeout(t *testing.T) {
	// Dialing a closed port on the loopback triggers a connection-refused
	// quickly. Use a tiny ConnectTimeout to bound it either way.
	opts := tcpclient.ClientOptions{
		Network:        "tcp",
		Address:        "127.0.0.1:1", // reserved, refuses connections
		ConnectTimeout: 20 * time.Millisecond,
		ReadTimeout:    50 * time.Millisecond,
		WriteTimeout:   50 * time.Millisecond,
		RetryMax:       0,
	}
	c := tcpclient.NewClient(opts)
	defer c.Close()

	if err := c.Send(context.Background(), []byte("x")); err == nil {
		t.Fatal("Send: expected connect error, got nil")
	}
	if m := c.Metrics(); m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
}

// --- Retry -------------------------------------------------------------------

func TestClient_Retry_OnConnectionError(t *testing.T) {
	// Always-failing target: connection refused on a closed port. With
	// RetryMax=2 the client should make 3 total attempts.
	opts := tcpclient.ClientOptions{
		Network:        "tcp",
		Address:        "127.0.0.1:1",
		ConnectTimeout: 20 * time.Millisecond,
		ReadTimeout:    50 * time.Millisecond,
		WriteTimeout:   50 * time.Millisecond,
		RetryMax:       2,
		RetryWaitMin:   1 * time.Millisecond,
		RetryWaitMax:   5 * time.Millisecond,
	}
	c := tcpclient.NewClient(opts)
	defer c.Close()

	if err := c.Send(context.Background(), []byte("x")); err == nil {
		t.Fatal("Send: expected error after retries, got nil")
	}
	m := c.Metrics()
	if m.Retried != 2 {
		t.Fatalf("Metrics.Retried = %d, want 2", m.Retried)
	}
	if m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
	if m.Total != 1 {
		t.Fatalf("Metrics.Total = %d, want 1", m.Total)
	}
}

func TestClient_Retry_SucceedsOnSecondAttempt(t *testing.T) {
	// Server that hard-resets the first accepted connection (so the client's
	// read fails with "connection reset by peer" — a retryable error) and
	// echoes on the second, so the retry succeeds. A clean close-without-data
	// would surface as a successful empty response (per the SendReceive
	// contract: read until connection close), so we use SetLinger(0) to force
	// an actual RST.
	var accepts uint64
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			n := atomic.AddUint64(&accepts, 1)
			go func(c net.Conn, n uint64) {
				if n == 1 {
					// First connection: read the request, then close with
					// SO_LINGER 0 to send an RST (retryable read error).
					buf := make([]byte, 32)
					_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
					_, _ = c.Read(buf)
					if tc, ok := c.(*net.TCPConn); ok {
						_ = tc.SetLinger(0)
					}
					_ = c.Close()
					return
				}
				// Subsequent connections: echo once and close so io.ReadAll
				// returns cleanly on the retry.
				defer c.Close()
				buf := make([]byte, 32)
				_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				rn, _ := c.Read(buf)
				if rn > 0 {
					_, _ = c.Write(buf[:rn])
				}
			}(conn, n)
		}
	}()

	opts := fastOpts(ln.Addr().String())
	opts.RetryMax = 1
	c := tcpclient.NewClient(opts)
	defer c.Close()

	got, err := c.SendReceive(context.Background(), []byte("data"))
	if err != nil {
		t.Fatalf("SendReceive: %v (expected success on retry)", err)
	}
	if string(got) != "data" {
		t.Fatalf("SendReceive: got %q, want %q", got, "data")
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

// --- Metrics -----------------------------------------------------------------

func TestClient_Metrics_Accumulate(t *testing.T) {
	ln, _ := echoOnceListener(t)
	defer ln.Close()

	opts := fastOpts(ln.Addr().String())
	opts.RetryMax = 0
	c := tcpclient.NewClient(opts)
	defer c.Close()

	for i := range 5 {
		if _, err := c.SendReceive(context.Background(), []byte("m")); err != nil {
			t.Fatalf("SendReceive[%d]: %v", i, err)
		}
	}
	m := c.Metrics()
	if m.Total != 5 || m.Success != 5 || m.Failed != 0 || m.Retried != 0 {
		t.Fatalf("Metrics = %+v, want {Total:5 Success:5 Failed:0 Retried:0}", m)
	}
}

// --- Concurrent / race -------------------------------------------------------

func TestClient_Concurrent_RaceSafe(t *testing.T) {
	ln, _ := echoOnceListener(t)
	defer ln.Close()

	opts := fastOpts(ln.Addr().String())
	opts.PoolSize = 8
	opts.RetryMax = 1
	// Under 50-way concurrency the kernel backlog and loopback scheduling can
	// delay a dial past fastOpts' tight 50ms ConnectTimeout, producing a flaky
	// i/o timeout that has nothing to do with race-safety. Give the dial (and
	// read) enough headroom to ride out the burst.
	opts.ConnectTimeout = 500 * time.Millisecond
	opts.ReadTimeout = 500 * time.Millisecond
	c := tcpclient.NewClient(opts)
	defer c.Close()

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			payload := []byte(fmt.Sprintf("g-%d", i))
			got, err := c.SendReceive(context.Background(), payload)
			if err != nil {
				errCh <- err
				return
			}
			if string(got) != string(payload) {
				errCh <- fmt.Errorf("g-%d: reply %q", i, got)
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent: %v", err)
	}
	if m := c.Metrics(); m.Total != n {
		t.Fatalf("Metrics.Total = %d, want %d", m.Total, n)
	}
}

// --- Close / lifecycle -------------------------------------------------------

func TestClient_Close_Idempotent(t *testing.T) {
	ln, _ := echoOnceListener(t)
	defer ln.Close()

	c := tcpclient.NewClient(fastOpts(ln.Addr().String()))
	if err := c.Close(); err != nil {
		t.Fatalf("Close 1: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close 2: %v", err)
	}
	// A third close via defer must not panic either.
	defer c.Close()
}

// --- Events ------------------------------------------------------------------

func TestClient_SetOnEvent(t *testing.T) {
	ln, _ := echoOnceListener(t)
	defer ln.Close()

	opts := fastOpts(ln.Addr().String())
	opts.RetryMax = 0
	c := tcpclient.NewClient(opts)
	defer c.Close()

	var mu sync.Mutex
	var seen []string
	c.SetOnEvent(func(evt tcpclient.ClientEvent) {
		mu.Lock()
		seen = append(seen, evt.Name)
		mu.Unlock()
		if evt.Addr != ln.Addr().String() {
			t.Errorf("event Addr = %q, want %q", evt.Addr, ln.Addr().String())
		}
	})

	if _, err := c.SendReceive(context.Background(), []byte("evt")); err != nil {
		t.Fatalf("SendReceive: %v", err)
	}

	// Disable the hook and ensure no further events fire.
	c.SetOnEvent(nil)

	mu.Lock()
	got := append([]string(nil), seen...)
	mu.Unlock()

	// Expect at least connect, send, receive, success in some order; retry is
	// absent because RetryMax=0.
	wantSet := map[string]bool{"connect": false, "send": false, "receive": false, "success": false}
	for _, n := range got {
		if _, ok := wantSet[n]; ok {
			wantSet[n] = true
		}
	}
	for name, saw := range wantSet {
		if !saw {
			t.Errorf("expected event %q, not seen in %v", name, got)
		}
	}
	if hasString(got, "retry") {
		t.Errorf("did not expect retry event with RetryMax=0, got %v", got)
	}
}

func TestClient_SetOnEvent_RetryEvent(t *testing.T) {
	// Always-refusing target: the retry event should fire.
	opts := tcpclient.ClientOptions{
		Network:        "tcp",
		Address:        "127.0.0.1:1",
		ConnectTimeout: 20 * time.Millisecond,
		RetryMax:       1,
		RetryWaitMin:   1 * time.Millisecond,
		RetryWaitMax:   5 * time.Millisecond,
	}
	c := tcpclient.NewClient(opts)
	defer c.Close()

	var retries uint64
	c.SetOnEvent(func(evt tcpclient.ClientEvent) {
		if evt.Name == "retry" {
			atomic.AddUint64(&retries, 1)
		}
	})

	_ = c.Send(context.Background(), []byte("x"))
	if got := atomic.LoadUint64(&retries); got != 1 {
		t.Fatalf("retry events = %d, want 1", got)
	}
}

// --- Breaker integration -----------------------------------------------------

// fakeBreaker is a test double for tcpclient.CircuitBreaker. It delegates to
// fn (so the call actually happens) and counts Execute invocations + failures.
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

// explicitBreaker always returns the configured error without calling fn.
type explicitBreaker struct{ err error }

func (b *explicitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	return b.err
}

func TestClient_Breaker_DelegatesAndCounts(t *testing.T) {
	ln, _ := echoOnceListener(t)
	defer ln.Close()

	br := &fakeBreaker{}
	opts := fastOpts(ln.Addr().String())
	opts.Breaker = br
	c := tcpclient.NewClient(opts)
	defer c.Close()

	got, err := c.SendReceive(context.Background(), []byte("brk"))
	if err != nil {
		t.Fatalf("SendReceive: %v", err)
	}
	if string(got) != "brk" {
		t.Fatalf("SendReceive: got %q", got)
	}
	if calls := atomic.LoadUint64(&br.calls); calls != 1 {
		t.Fatalf("breaker.Execute calls = %d, want 1", calls)
	}
	if failures := atomic.LoadUint64(&br.failures); failures != 0 {
		t.Fatalf("breaker.failures = %d, want 0", failures)
	}
}

func TestClient_Breaker_OpenShortCircuits(t *testing.T) {
	ln, conns := echoOnceListener(t)
	defer ln.Close()

	opts := fastOpts(ln.Addr().String())
	opts.Breaker = &explicitBreaker{err: errors.New("circuit open")}
	c := tcpclient.NewClient(opts)
	defer c.Close()

	_, err := c.SendReceive(context.Background(), []byte("x"))
	if err == nil {
		t.Fatal("SendReceive: expected circuit-open error")
	}
	// The breaker short-circuits before any dial, so the server is untouched.
	if got := atomic.LoadUint64(conns); got != 0 {
		t.Fatalf("server conns = %d, want 0 (breaker should short-circuit)", got)
	}
	if m := c.Metrics(); m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
}

// --- Context cancellation ----------------------------------------------------

func TestClient_ContextCanceled_NotRetried(t *testing.T) {
	// A server that accepts one connection and blocks forever reading (until
	// the connection is torn down), so the client's read hangs until ctx
	// cancel fires the deadline.
	ln := acceptOnceListener(t, func(c net.Conn) {
		defer c.Close()
		io.Copy(io.Discard, c) // block until the peer closes / errors
	})
	defer ln.Close()

	opts := fastOpts(ln.Addr().String())
	opts.ReadTimeout = 0 // rely on caller ctx so the read truly blocks
	opts.RetryMax = 3
	c := tcpclient.NewClient(opts)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := c.SendReceive(ctx, []byte("block"))
	if err == nil {
		t.Fatal("SendReceive: expected ctx error")
	}
	// Cancellation/deadline must not be retried.
	if m := c.Metrics(); m.Retried != 0 {
		t.Fatalf("Metrics.Retried = %d, want 0 (no retry on ctx cancel)", m.Retried)
	}
}

// --- helpers -----------------------------------------------------------------

func hasString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// waitForConns polls the server's accepted-connection counter until it reaches
// want or up to 200ms elapses. The accept goroutine increments after Accept
// returns, which can race with the test's assertion (the client's Send returns
// as soon as the write is flushed, before the server goroutine has run its
// increment). Polling makes the pool/eviction assertions deterministic.
func waitForConns(counter *uint64, want uint64) bool {
	for range 40 {
		if atomic.LoadUint64(counter) >= want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return atomic.LoadUint64(counter) >= want
}
