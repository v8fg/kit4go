package log4go

import (
	"bytes"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// localTCPListener starts a TCP listener on an ephemeral port and returns it.
// Each accepted conn is handed to onConn for the test to read payloads.
func localTCPListener(t *testing.T, onConn func(net.Conn)) *net.TCPListener {
	t.Helper()
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	ln, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			onConn(c)
		}
	}()
	return ln
}

// readOneConn reads until the connection yields at least one byte or errors,
// then signals done with whatever was read. This avoids tests hanging on a
// daemon that writes but never closes the conn (the daemon keeps the conn open
// for reuse, so a naive io.Copy would block forever).
func readOneConn(c net.Conn, mu *sync.Mutex, got *bytes.Buffer, done chan<- struct{}) {
	tmp := make([]byte, 256)
	n, err := c.Read(tmp)
	if n > 0 {
		mu.Lock()
		got.Write(tmp[:n])
		mu.Unlock()
	}
	_ = err
	_ = c.Close()
	if done != nil {
		close(done)
	}
}

// Test_NetWriter_TCPEndToEnd starts a local TCP server, registers a NetWriter
// against it through a Logger, emits a record, and confirms the server receives
// the serialized line.
func Test_NetWriter_TCPEndToEnd(t *testing.T) {
	var (
		mu   sync.Mutex
		got  bytes.Buffer
		done = make(chan struct{})
	)
	ln := localTCPListener(t, func(c net.Conn) {
		readOneConn(c, &mu, &got, done)
	})
	defer ln.Close()
	addr := ln.Addr().String()

	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)

	w := NewNetWriter(NetWriterOptions{
		Network:        "tcp",
		Address:        addr,
		BufferSize:     16,
		Timeout:        time.Second,
		OverflowPolicy: "drop",
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer w.Stop()
	root.Register(w)

	root.Info("net-writer hello %d", 1)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("server never saw a connection")
	}
	mu.Lock()
	out := got.String()
	mu.Unlock()
	if !strings.Contains(out, "net-writer hello 1") {
		t.Errorf("server did not receive line; got: %q", out)
	}
}

// Test_NetWriter_LazyReconnect confirms the writer survives a missing server on
// Init (no dial) and reconnects lazily: start with a closed listener address,
// then bring up a real server and confirm records flow.
func Test_NetWriter_LazyReconnect(t *testing.T) {
	// pick an ephemeral port we control: listen, grab addr, close so it's "down".
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	w := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: addr,
		BufferSize: 8, Timeout: 200 * time.Millisecond,
		OverflowPolicy: "drop", ReconnectBackoff: 50 * time.Millisecond,
	})
	// Init must NOT dial (lazy), so it succeeds even though addr is down.
	if err := w.Init(); err != nil {
		t.Fatalf("Init should not dial: %v", err)
	}

	// emit while down -> records queue (then dropped on write error). No panic,
	// no block. Direct writes so we don't depend on the bootstrap goroutine.
	for range 3 {
		_ = w.Write(&Record{level: INFO, time: "t", file: "f", msg: "down"})
	}

	// bring the server up at the same addr
	srv, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("re-listen: %v", err)
	}
	defer srv.Close()
	var (
		mu   sync.Mutex
		got  bytes.Buffer
		done = make(chan struct{})
	)
	go func() {
		c, err := srv.Accept()
		if err != nil {
			return
		}
		readOneConn(c, &mu, &got, done)
	}()

	// emit a record; the daemon should (re)dial and deliver. Once the server is
	// back up, any queued or new records flow through the re-established conn.
	// We assert the connection was re-established and data arrived (the exact
	// record delivered depends on race between the down-records draining and the
	// reconnect; both prove the lazy reconnect works).
	_ = w.Write(&Record{level: INFO, time: "t", file: "f", msg: "reconnected-line"})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		w.Stop()
		t.Fatal("server never saw a reconnect connection")
	}
	w.Stop()
	mu.Lock()
	out := got.String()
	mu.Unlock()
	if out == "" {
		t.Fatal("writer did not reconnect after server came up")
	}
	// the reconnected-line may or may not be the first to land (down records
	// can win the race), but the connection must have carried log data.
	if !strings.Contains(out, "[INFO]") {
		t.Errorf("reconnect delivered unexpected data: %q", out)
	}
}

// Test_NetWriter_DropOnFull confirms the drop policy drops (not blocks) when the
// channel is full and counts the drop.
func Test_NetWriter_DropOnFull(t *testing.T) {
	w := &NetWriter{
		level:    DEBUG,
		policy:   OverflowDrop,
		messages: make(chan *Record, 1),
	}
	w.messages <- &Record{level: INFO, msg: "fill"} // full
	w.Write(&Record{level: INFO, msg: "overflow"})  // must drop, not block
	if got := w.stats.Dropped(); got != 1 {
		t.Errorf("dropped=%d want 1", got)
	}
}

// Test_NetWriter_SpillOnFull confirms the spill policy routes overflow to the
// ring spiller.
func Test_NetWriter_SpillOnFull(t *testing.T) {
	w := &NetWriter{
		level:    DEBUG,
		policy:   OverflowSpill,
		spiller:  NewRingSpiller[*Record](8),
		messages: make(chan *Record, 1),
	}
	w.messages <- &Record{level: INFO, msg: "fill"} // full
	w.Write(&Record{level: INFO, msg: "overflow"})  // must spill
	if got := w.stats.Spilled(); got != 1 {
		t.Errorf("spilled=%d want 1", got)
	}
	if w.spiller.Len() != 1 {
		t.Errorf("spiller len=%d want 1", w.spiller.Len())
	}
}

// Test_NetWriter_NoGoroutineBurst confirms Write does not spawn per-record
// goroutines (the async daemon is the only goroutine).
func Test_NetWriter_NoGoroutineBurst(t *testing.T) {
	w := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: "127.0.0.1:1", // unroutable, but lazy Init won't dial
		BufferSize: 2048, OverflowPolicy: "drop",
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer w.Stop()
	before := runtime.NumGoroutine()
	for range 1000 {
		_ = w.Write(&Record{level: INFO, msg: "burst"})
	}
	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Errorf("Write spawned goroutines: before=%d after=%d", before, after)
	}
}

// Test_NetWriter_JSONFastPath confirms NetWriter honors r.formattedBytes when set
// (FormatJSON records ship as JSON, not text).
func Test_NetWriter_JSONFastPath(t *testing.T) {
	var (
		mu   sync.Mutex
		got  bytes.Buffer
		done = make(chan struct{})
	)
	ln := localTCPListener(t, func(c net.Conn) {
		readOneConn(c, &mu, &got, done)
	})
	defer ln.Close()

	w := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: ln.Addr().String(),
		BufferSize: 4, Timeout: time.Second, OverflowPolicy: "drop",
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer w.Stop()

	r := &Record{
		level: INFO, time: "t", file: "f", msg: "m",
		formattedBytes: []byte(`{"time":"t","level":"INFO","msg":"m"}` + "\n"),
	}
	_ = w.Write(r)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("server never saw a connection")
	}
	mu.Lock()
	out := got.String()
	mu.Unlock()
	if !strings.Contains(out, `"msg":"m"`) {
		t.Errorf("did not ship JSON; got: %q", out)
	}
	if strings.Contains(out, "[INFO]") {
		t.Errorf("shipped text form instead of JSON: %q", out)
	}
}

// Test_NetWriter_ImplementsWriter confirms the compile-time interface assertion
// holds (NetWriter is a valid Writer + Flusher). If the assertion in the source
// fails this test won't compile, which is the point.
func Test_NetWriter_ImplementsWriter(t *testing.T) {
	var _ Writer = (*NetWriter)(nil)
	var _ Flusher = (*NetWriter)(nil)
}

// Test_NetWriter_Metrics confirms the metrics snapshot reflects sent/error
// counts after a successful write.
func Test_NetWriter_Metrics(t *testing.T) {
	var (
		mu  sync.Mutex
		got bytes.Buffer
	)
	ln := localTCPListener(t, func(c net.Conn) {
		readOneConn(c, &mu, &got, nil)
	})
	defer ln.Close()

	w := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: ln.Addr().String(),
		BufferSize: 4, Timeout: time.Second, OverflowPolicy: "drop",
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer w.Stop()
	_ = w.Write(&Record{level: INFO, time: "t", file: "f", msg: "m"})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if w.Metrics().Sent > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if w.Metrics().Sent == 0 {
		t.Error("Metrics.Sent never incremented")
	}
}
