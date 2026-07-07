package log4go

// Coverage boosters for net_writer.go targeting the remaining uncovered
// branches: NewNetWriter spill ctor, Write level-filter + OverflowBlock,
// dialLocked default-network + dial error, writeOne write-error path,
// daemon stop-channel path, drainAll empty-channel exit, Stop no-daemon
// guard, Metrics spiller branch.
//
// Determinism ground rules (no flakes):
//   - local TCP listeners on 127.0.0.1:0 only, closed via t.Cleanup;
//   - unroutable dials use a freshly-closed listener address (ECONNREFUSED),
//     not a far-away host — so dial fails in milliseconds;
//   - generous 3s waits on ready-channels, never on fixed sleeps;
//   - explicit w.Stop() / close(n.stop) to synchronize daemon shutdown.

import (
	"bytes"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// netTestDeadline is the budget for polling an async NetWriter daemon under
// -race/CI load. The daemon writes through a network conn whose scheduling can
// lag under contention; 5s (up from 3s) keeps the polling-based waits robust
// without masking genuine hangs.
const netTestDeadline = 5 * time.Second

// waitForSent polls Metrics().Sent until it reaches want or the deadline
// passes (fails the test on timeout). Lets tests deterministically observe an
// async daemon write without sleeping a fixed interval.
func waitForSent(t *testing.T, w *NetWriter, want uint64) {
	t.Helper()
	deadline := time.Now().Add(netTestDeadline)
	for time.Now().Before(deadline) {
		if w.Metrics().Sent >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("Metrics.Sent=%d want %d", w.Metrics().Sent, want)
}

// waitForErr polls Metrics().Errored until it reaches want or the deadline
// passes. Used to confirm a write/dial error path fired.
func waitForErr(t *testing.T, w *NetWriter, want uint64) {
	t.Helper()
	deadline := time.Now().Add(netTestDeadline)
	for time.Now().Before(deadline) {
		if w.Metrics().Errored >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("Metrics.Errored=%d want %d", w.Metrics().Errored, want)
}

// waitForChan polls ch (closed/signalled by an async daemon) until it is ready
// or the deadline passes. This replaces the `select { case <-done: case
// <-time.After(3s) }` pattern whose fixed deadline was the flake source under
// -race/CI load — polling keeps the same upper bound but is consistent with the
// waitForSent/waitForErr style used across these coverage tests.
func waitForChan(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	deadline := time.Now().Add(netTestDeadline)
	for {
		select {
		case <-ch:
			return
		default:
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("%s not signalled within %s", what, netTestDeadline)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Test_NetWriterCov_NewNetWriter_SpillCtor covers NewNetWriter's
// OverflowSpill constructor branch (line 116-118): with OverflowPolicy="spill"
// the writer must allocate a RingSpiller so Metrics().SpillLen is wired.
func Test_NetWriterCov_NewNetWriter_SpillCtor(t *testing.T) {
	w := NewNetWriter(NetWriterOptions{
		Network:        "tcp",
		Address:        "127.0.0.1:1",
		BufferSize:     4,
		OverflowPolicy: "spill",
		SpillSize:      8,
		Level:          "warn", // exercises getLevelDefault non-empty path too
	})
	t.Cleanup(func() {
		// daemon not started: Stop is a no-op, but call it for symmetry.
		w.Stop()
	})
	if w.spiller == nil {
		t.Fatal("spill policy must allocate a spiller")
	}
	// Push one record then confirm Metrics reports the spill length (covers
	// Metrics() spiller!=nil branch at net_writer.go:322).
	w.spiller.Push(&Record{level: INFO, msg: "spilled"})
	if got := w.Metrics().SpillLen; got != 1 {
		t.Errorf("SpillLen=%d want 1", got)
	}
}

// Test_NetWriterCov_NewNetWriter_Defaults covers the defaulting branches in
// NewNetWriter when Network/BufferSize/Timeout/ReconnectBackoff are all
// zero/empty: BufferSize<=1 -> 1024, Timeout<=0 -> 3s, backoff<=0 -> 1s, and
// the empty-Level path keeps DEBUG.
func Test_NetWriterCov_NewNetWriter_Defaults(t *testing.T) {
	w := NewNetWriter(NetWriterOptions{Address: "127.0.0.1:1"})
	if cap(w.messages) != 1024 {
		t.Errorf("default BufferSize: cap=%d want 1024", cap(w.messages))
	}
	if w.timeout != 3*time.Second {
		t.Errorf("default timeout=%v want 3s", w.timeout)
	}
	if w.reconnectBackoff != time.Second {
		t.Errorf("default backoff=%v want 1s", w.reconnectBackoff)
	}
	if w.level != DEBUG {
		t.Errorf("empty level default level=%d want DEBUG(%d)", w.level, DEBUG)
	}
}

// Test_NetWriterCov_Write_LevelFilter covers Write's early return at
// net_writer.go:137 (record level > writer level -> nil, never enqueued).
func Test_NetWriterCov_Write_LevelFilter(t *testing.T) {
	w := NewNetWriter(NetWriterOptions{
		Address:        "127.0.0.1:1",
		Level:          "warn", // level == WARN
		BufferSize:     4,
		OverflowPolicy: "drop",
	})
	// DEBUG record below WARN threshold: filtered out, channel stays empty.
	if err := w.Write(&Record{level: DEBUG, msg: "filtered"}); err != nil {
		t.Fatalf("Write returned err: %v", err)
	}
	if len(w.messages) != 0 {
		t.Errorf("DEBUG record below WARN leaked into queue: len=%d", len(w.messages))
	}
}

// Test_NetWriterCov_Write_BlockPolicy covers Write's OverflowBlock branch at
// net_writer.go:142 (blocking send into messages). Because the daemon is NOT
// started, the channel must have capacity and the send must succeed promptly
// (no real block). We assert the record is enqueued.
func Test_NetWriterCov_Write_BlockPolicy(t *testing.T) {
	w := &NetWriter{
		level:    DEBUG,
		policy:   OverflowBlock,
		messages: make(chan *Record, 2),
	}
	// Under block policy the default/empty branch is skipped; the blocking send
	// at line 142 is taken.
	if err := w.Write(&Record{level: INFO, msg: "block-1"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Write(&Record{level: INFO, msg: "block-2"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(w.messages) != 2 {
		t.Errorf("block policy enqueued=%d want 2", len(w.messages))
	}
	// drain to avoid leaking the channel (no daemon running).
	<-w.messages
	<-w.messages
}

// Test_NetWriterCov_Write_SpillPushFail covers Write's spill branch when the
// spiller rejects (nil spiller): the else-branch counts a drop instead of a
// spill. Constructed so policy==OverflowSpill but spiller==nil forces the
// drop fallback at net_writer.go:149-151.
func Test_NetWriterCov_Write_SpillPushFail(t *testing.T) {
	w := &NetWriter{
		level:    DEBUG,
		policy:   OverflowSpill,
		spiller:  nil, // forces the drop fallback
		messages: make(chan *Record, 1),
	}
	w.messages <- &Record{level: INFO, msg: "fill"} // channel full
	if err := w.Write(&Record{level: INFO, msg: "overflow"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := w.stats.Dropped(); got != 1 {
		t.Errorf("drop fallback dropped=%d want 1", got)
	}
	if got := w.stats.Spilled(); got != 0 {
		t.Errorf("spilled=%d want 0 (no spiller)", got)
	}
	<-w.messages
}

// Test_NetWriterCov_dialLocked_DefaultNetwork covers dialLocked's default
// network branch (net_writer.go:171): with Network empty it must dial "tcp".
// We point it at a live TCP listener so the dial succeeds and confirm a
// record flows through writeOne to the server.
func Test_NetWriterCov_dialLocked_DefaultNetwork(t *testing.T) {
	var (
		mu   sync.Mutex
		got  bytes.Buffer
		done = make(chan struct{})
	)
	ln := localTCPListener(t, func(c net.Conn) {
		readOneConn(c, &mu, &got, done)
	})
	t.Cleanup(func() { ln.Close() })

	w := NewNetWriter(NetWriterOptions{
		// Network intentionally empty -> dialLocked defaults to "tcp".
		Address:    ln.Addr().String(),
		BufferSize: 4,
		Timeout:    time.Second,
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { w.Stop() })

	_ = w.Write(&Record{level: INFO, time: "t", file: "f", msg: "default-net"})
	waitForChan(t, done, "default-network dial record delivery")
	mu.Lock()
	out := got.String()
	mu.Unlock()
	if !strings.Contains(out, "default-net") {
		t.Errorf("default-network dial lost payload: %q", out)
	}
}

// Test_NetWriterCov_dialLocked_DialError covers dialLocked's error-return
// branch (net_writer.go:175-177) and writeOne's dial-failure branch
// (net_writer.go:189-192): an unroutable address makes dial fail, writeOne
// increments errored and returns the dial error.
func Test_NetWriterCov_dialLocked_DialError(t *testing.T) {
	// Reserve then release a port so dial gets ECONNREFUSED fast (no real
	// network wait, no far-host flakiness).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	w := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: addr,
		BufferSize: 4, Timeout: 200 * time.Millisecond,
		ReconnectBackoff: 10 * time.Millisecond,
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { w.Stop() })

	// Enqueue a record; the daemon's writeOne must fail to dial, increment
	// errored, and drop the record (no panic, no block).
	_ = w.Write(&Record{level: INFO, time: "t", file: "f", msg: "no-dial"})
	waitForErr(t, w, 1)

	m := w.Metrics()
	if m.Errored == 0 {
		t.Errorf("Errored=0 want >=1 after dial failure")
	}
}

// Test_NetWriterCov_writeOne_WriteError covers writeOne's write-error path
// (net_writer.go:197-204): we accept a conn then immediately Close it so the
// next Write on the (now half-closed) conn fails. The writer must close its
// conn, nil it, and increment both errored and dropped.
func Test_NetWriterCov_writeOne_WriteError(t *testing.T) {
	// A listener that accepts exactly one conn and closes it right away. The
	// first record dials successfully, then the server-side RST makes the
	// client Write fail -> writeOne error path.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		// Close immediately to force a write error (EPIPE / broken pipe /
		// connection reset) on the client's next Write.
		_ = c.Close()
	}()

	w := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: ln.Addr().String(),
		BufferSize: 8, Timeout: time.Second,
		ReconnectBackoff: 10 * time.Millisecond,
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { w.Stop() })

	// Keep writing until at least one write fails (the first dial may race the
	// server-side close; the RST usually lands on the 2nd write). Loop with a
	// hard cap so it can never hang.
	for i := 0; i < 50 && w.Metrics().Errored == 0; i++ {
		_ = w.Write(&Record{level: INFO, time: "t", file: "f", msg: "will-fail"})
		time.Sleep(10 * time.Millisecond)
	}
	m := w.Metrics()
	if m.Errored == 0 {
		t.Fatal("writeOne never saw a write error")
	}
	if m.Dropped == 0 {
		t.Error("writeOne write-error path did not increment Dropped")
	}
}

// Test_NetWriterCov_daemon_StopChannel covers daemon's `case <-n.stop:`
// branch (net_writer.go:231-234). The stop channel is an alternate shutdown
// signal kept for completeness; closing it makes the daemon drainAll + signal
// quit. We exercise it directly (it is not reachable through the public Stop
// API, which closes messages instead).
func Test_NetWriterCov_daemon_StopChannel(t *testing.T) {
	// Unroutable address so writeOne never succeeds (keeps the daemon alive in
	// the select loop until we close n.stop).
	w := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: "127.0.0.1:1",
		BufferSize: 4, Timeout: 100 * time.Millisecond,
		ReconnectBackoff: 10 * time.Millisecond,
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Give the daemon a moment to enter its select loop, then trigger the
	// stop-channel branch.
	close(n_stop_safe(w))

	// daemon should drain (nothing queued) and signal quit. Wait for it with a
	// generous timeout. After this, n.run may still read true (we bypassed
	// Stop()), so mark it false to avoid leaking Stop() state in case a later
	// test reuses this writer — simplest is to not call Stop() at all.
	waitForChan(t, n_quit_safe(w), "daemon quit on stop-channel close")
}

// n_stop_safe / n_quit_safe return the writer's internal shutdown channels.
// Defined as thin helpers so the test reads clearly; the fields are already
// in-package accessible.
func n_stop_safe(w *NetWriter) chan struct{}   { return w.stop }
func n_quit_safe(w *NetWriter) <-chan struct{} { return w.quit }

// Test_NetWriterCov_drainAll_EmptyChannelExit covers drainAll's `default:
// goto spill` exit (net_writer.go:264) which fires when drainAll is called
// with an empty messages channel. The cleanest deterministic trigger is
// daemon shutdown via the stop channel with NO records queued: the daemon
// hits `case <-n.stop`, calls drainAll, the messages channel is empty so the
// non-blocking default fires, then it exits. We additionally assert no
// records were written (server never sees data).
func Test_NetWriterCov_drainAll_EmptyChannelExit(t *testing.T) {
	sawData := make(chan struct{}, 1)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 1)
		if n, _ := c.Read(buf); n > 0 {
			select {
			case sawData <- struct{}{}:
			default:
			}
		}
		_ = c.Close()
	}()

	w := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: ln.Addr().String(),
		BufferSize: 4, Timeout: 200 * time.Millisecond,
		ReconnectBackoff: 10 * time.Millisecond,
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// No records queued -> drainAll's messages loop hits the default branch on
	// the first iteration and falls through to spill (spiller is nil -> return).
	close(n_stop_safe(w))
	waitForChan(t, n_quit_safe(w), "daemon exit after drainAll on empty channel")
	// Give a brief window for any spurious dial; the server must NOT have seen
	// data because nothing was queued.
	select {
	case <-sawData:
		t.Error("drainAll wrote a record despite empty queue")
	case <-time.After(150 * time.Millisecond):
	}
}

// Test_NetWriterCov_drainAll_SpillFlush covers drainAll's spill-flush loop
// (net_writer.go:269-277) and the nil-record guard (line 274) deterministically.
// We build a writer WITHOUT starting the daemon, pre-seed its spiller directly,
// attach a live conn, and call drainAll by hand. This avoids the daemon's
// 200ms ticker / write-rate race that makes the spill branch hard to hit
// deterministically through the public API.
func Test_NetWriterCov_drainAll_SpillFlush(t *testing.T) {
	var (
		mu   sync.Mutex
		got  bytes.Buffer
		done = make(chan struct{})
		once sync.Once
	)
	ln := localTCPListener(t, func(c net.Conn) {
		buf := make([]byte, 4096)
		for {
			n, err := c.Read(buf)
			if n > 0 {
				mu.Lock()
				got.Write(buf[:n])
				mu.Unlock()
			}
			if err != nil {
				break
			}
		}
		once.Do(func() { close(done) })
	})
	t.Cleanup(func() { ln.Close() })

	// Build manually so no daemon runs; attach a live conn by dialing ourselves.
	// messages has capacity 2 and holds ONE real record so drainAll's messages
	// loop receives it and calls writeOne (covers net_writer.go:264), then on
	// the next iteration the channel is empty -> default -> goto spill.
	w := &NetWriter{
		level:            DEBUG,
		policy:           OverflowSpill,
		spiller:          NewRingSpiller[*Record](8),
		messages:         make(chan *Record, 2),
		timeout:          time.Second,
		reconnectBackoff: 10 * time.Millisecond,
		options:          NetWriterOptions{Network: "tcp", Address: ln.Addr().String()},
	}

	// Dial the live server and stash the conn so writeOne skips dialing and
	// writes straight through (covers the spill-flush write path).
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	w.connMu.Lock()
	w.conn = c
	w.connMu.Unlock()

	// Park one real record in the messages channel: drainAll receives it
	// (ok=true, r!=nil) and calls writeOne -> net_writer.go:264 covered.
	w.messages <- &Record{level: INFO, time: "t", file: "f", msg: "queued-A"}
	// Seed the spiller with two real records + one nil. The nil exercises the
	// `if r != nil` guard at net_writer.go:274 (skip without writing); the real
	// records must be written to the conn during the spill flush.
	w.spiller.Push(&Record{level: INFO, time: "t", file: "f", msg: "spilled-A"})
	w.spiller.Push(nil) // exercises the nil guard
	w.spiller.Push(&Record{level: INFO, time: "t", file: "f", msg: "spilled-B"})

	w.drainAll()

	// Close the conn so the server's read loop sees EOF and signals done. The
	// daemon is not running, so we close the conn we dialed by hand.
	w.connMu.Lock()
	if w.conn != nil {
		_ = w.conn.Close()
	}
	w.connMu.Unlock()

	waitForChan(t, done, "server receiving flushed spill records")
	mu.Lock()
	out := got.String()
	mu.Unlock()
	if !strings.Contains(out, "queued-A") {
		t.Errorf("drainAll messages loop lost queued-A: %q", out)
	}
	if !strings.Contains(out, "spilled-A") {
		t.Errorf("spill flush lost spilled-A: %q", out)
	}
	if !strings.Contains(out, "spilled-B") {
		t.Errorf("spill flush lost spilled-B: %q", out)
	}
	// Sanity: the daemon was never started, so run is false and Stop is a no-op.
	w.Stop()
}

// Test_NetWriterCov_drainAll_EmptyOpenChannel covers drainAll's `default:
// goto spill` exit (net_writer.go:264). drainAll is only invoked by the
// daemon after a shutdown signal; on the public Stop path the messages
// channel is closed first, so the messages loop hits ok=false (not default).
// To reach the non-blocking default we call drainAll directly with an
// OPEN, EMPTY messages channel: the select falls through to default ->
// goto spill -> spiller is nil -> return. Nothing is written.
func Test_NetWriterCov_drainAll_EmptyOpenChannel(t *testing.T) {
	w := &NetWriter{
		level:    DEBUG,
		policy:   OverflowDrop,
		messages: make(chan *Record, 2), // open + empty
		// spiller left nil so the spill section returns immediately.
	}
	// Calling drainAll on an empty open channel must hit the default branch
	// (line 264) and return without blocking or writing.
	done := make(chan struct{})
	go func() {
		w.drainAll()
		close(done)
	}()
	waitForChan(t, done, "drainAll return on empty open channel (default branch)")
}

// Test_NetWriterCov_daemon_TickerDrainSpill covers the daemon's ticker branch
// (net_writer.go:229-230): a running daemon with a live 200ms ticker must
// fire `case <-ticker.C` and call drainSpill. We keep the daemon alive well
// past one ticker interval (>250ms) before shutting down, guaranteeing the
// branch executes at least once.
func Test_NetWriterCov_daemon_TickerDrainSpill(t *testing.T) {
	// Unroutable address so writeOne fails fast and the daemon stays parked in
	// its select loop (where the ticker can fire) until we stop it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	w := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: addr,
		BufferSize: 4, Timeout: 100 * time.Millisecond,
		ReconnectBackoff: 10 * time.Millisecond,
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Sleep >2x the ticker interval (200ms) so the ticker fires at least once
	// and drainSpill runs (with a nil spiller it's a fast no-op, but the branch
	// at net_writer.go:230 is exercised). 300ms is comfortably past one tick.
	time.Sleep(300 * time.Millisecond)

	// Shut the daemon down via the stop channel and confirm it exits.
	close(n_stop_safe(w))
	waitForChan(t, n_quit_safe(w), "daemon exit after ticker test")
}

// Test_NetWriterCov_Stop_NoDaemon covers Stop's early return at
// net_writer.go:285 (run==false -> no-op). A freshly built writer whose Init
// was never called must make Stop a harmless no-op.
func Test_NetWriterCov_Stop_NoDaemon(t *testing.T) {
	w := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: "127.0.0.1:1",
		BufferSize: 4,
	})
	// Init NOT called -> n.run is false -> Stop returns immediately. Must not
	// panic (close on an unclosed messages channel) or block.
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()
	waitForChan(t, done, "Stop return on a never-started daemon")
}

// Test_NetWriterCov_Stop_DrainsAndClosesConn covers Stop's normal shutdown
// path (net_writer.go:287-294): with the daemon running, Stop closes the
// messages channel, waits for quit, and closes the live conn. We confirm (a)
// the daemon drains a queued record before exiting and (b) the conn field is
// nil after Stop (proving the conn-close branch at line 290-292 ran).
func Test_NetWriterCov_Stop_DrainsAndClosesConn(t *testing.T) {
	var (
		mu   sync.Mutex
		got  bytes.Buffer
		done = make(chan struct{})
		once sync.Once
	)
	ln := localTCPListener(t, func(c net.Conn) {
		buf := make([]byte, 1024)
		for {
			n, err := c.Read(buf)
			if n > 0 {
				mu.Lock()
				got.Write(buf[:n])
				mu.Unlock()
			}
			if err != nil {
				break
			}
		}
		once.Do(func() { close(done) })
	})
	t.Cleanup(func() { ln.Close() })

	w := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: ln.Addr().String(),
		BufferSize: 4, Timeout: time.Second,
		ReconnectBackoff: 10 * time.Millisecond,
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// NOTE: no t.Cleanup(w.Stop) here — the test body calls w.Stop() exactly
	// once below; registering it again would double-close the messages channel.

	// Enqueue then wait for the write to land so a conn is open.
	_ = w.Write(&Record{level: INFO, time: "t", file: "f", msg: "before-stop"})
	waitForSent(t, w, 1)

	// Stop must drain any remaining queued records and close the conn.
	w.Stop()

	// Non-fatal: the daemon may have already closed the conn before the server
	// read saw EOF; the payload assertion below is authoritative. Bound the wait
	// at the same netTestDeadline used elsewhere for consistency under load.
	select {
	case <-done:
	case <-time.After(netTestDeadline):
	}
	mu.Lock()
	out := got.String()
	mu.Unlock()
	if !strings.Contains(out, "before-stop") {
		t.Errorf("Stop did not drain queued record: %q", out)
	}
	// After Stop the conn must be nil (the close branch at 290-292 ran).
	w.connMu.Lock()
	connNil := w.conn == nil
	w.connMu.Unlock()
	if !connNil {
		t.Error("conn not nil after Stop (close-conn branch did not run)")
	}
	// Note: we intentionally do NOT call w.Stop() a second time here. Stop
	// leaves n.run true after shutdown (it does not flip it), so a second
	// Stop would re-enter close(n.messages) and panic. The "Stop is safe to
	// call once" contract is documented on Stop itself; the no-daemon guard
	// (run==false) is covered by Test_NetWriterCov_Stop_NoDaemon.
}

// Test_NetWriterCov_String_AndMetricsFields exercises String() and the full
// Metrics() snapshot (Sent/Errored/Dropped/Queued/Overflow*) on a live
// writer, and confirms a down remote surfaces a non-zero Errored in Metrics
// (the branch where queued is read from a non-nil messages channel is
// exercised on every live writer).
func Test_NetWriterCov_String_AndMetricsFields(t *testing.T) {
	// Down address: dial fails so Errored increments; Queued reads len() of a
	// non-nil messages channel.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	w := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: addr,
		BufferSize: 4, Timeout: 100 * time.Millisecond,
		ReconnectBackoff: 10 * time.Millisecond,
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { w.Stop() })

	_ = w.Write(&Record{level: INFO, time: "t", file: "f", msg: "x"})
	waitForErr(t, w, 1)

	s := w.String()
	if !strings.Contains(s, "tcp://") || !strings.Contains(s, addr) {
		t.Errorf("String() missing network/address: %q", s)
	}
	m := w.Metrics()
	if m.Errored == 0 {
		t.Errorf("Metrics.Errored=0 want >=1")
	}
	// Queued is read from len(messages) — sanity that it is a sensible int.
	if m.Queued < 0 {
		t.Errorf("Metrics.Queued=%d negative", m.Queued)
	}
}

// Test_NetWriterCov_Write_DropNonSpillPolicy covers the drop-policy drop
// counter increment at net_writer.go:150 in a scenario where the channel is
// full and policy is OverflowDrop (not spill), confirming the else-branch
// (drop) is taken rather than the spill branch.
func Test_NetWriterCov_Write_DropNonSpillPolicy(t *testing.T) {
	w := &NetWriter{
		level:    DEBUG,
		policy:   OverflowDrop,
		messages: make(chan *Record, 1),
	}
	w.messages <- &Record{level: INFO, msg: "fill"}
	if err := w.Write(&Record{level: INFO, msg: "drop-me"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := w.stats.Dropped(); got != 1 {
		t.Errorf("dropped=%d want 1", got)
	}
	<-w.messages
}
