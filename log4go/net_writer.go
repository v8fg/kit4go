package log4go

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// NetWriterOptions configures a NetWriter.
//
// PERFORMANCE WARNING: network I/O is far slower than local File/Console
// (~10K-200K QPS for TCP vs ~3M for async File). NetWriter is async with a
// bounded channel + OverflowPolicy so a network stall cannot block the logger
// hot path, but it is still only appropriate for LOW-VOLUME log collection
// (e.g. shipping to a sidecar collector). For high-throughput log shipping use
// FileWriter + Kafka instead. See PERFORMANCE.md.
type NetWriterOptions struct {
	// Enable gates registration (used by config plumbing; NetWriter itself
	// ignores it).
	Enable bool `json:"enable" mapstructure:"enable"`
	// Network is "tcp" or "udp" (default "tcp").
	Network string `json:"network" mapstructure:"network"`
	// Address is the host:port to dial (e.g. "127.0.0.1:514" or "syslog:514").
	Address string `json:"address" mapstructure:"address"`
	// Level is the text level flag (default DEBUG).
	Level string `json:"level" mapstructure:"level"`
	// BufferSize is the async send channel capacity (<=0 -> 1024). Bounds the
	// worst-case memory and is the primary OOM guard; OverflowPolicy decides
	// what happens when it is full.
	BufferSize int `json:"buffer_size" mapstructure:"buffer_size"`
	// OverflowPolicy: "drop"(default)|"block"|"spill" — behavior when full.
	// "drop" is strongly recommended for net writers so a slow remote cannot
	// back-pressure the application.
	OverflowPolicy string `json:"overflow_policy" mapstructure:"overflow_policy"`
	// SpillSize: ring capacity (records) when policy == "spill".
	SpillSize int `json:"spill_size" mapstructure:"spill_size"`
	// Timeout is the per-write deadline on the conn (<=0 -> 3s). Bounds how long
	// a stuck remote can stall the daemon before the record is dropped and the
	// connection is recycled.
	Timeout time.Duration `json:"timeout" mapstructure:"timeout"`
	// ReconnectBackoff is how long to wait between dial attempts after a
	// disconnect (<=0 -> 1s). The daemon dials lazily on the first record and
	// re-dials after a write/close error.
	ReconnectBackoff time.Duration `json:"reconnect_backoff" mapstructure:"reconnect_backoff"`
}

// NetWriter ships records to a remote TCP/UDP endpoint. It is async by design:
// Write hands the record to a bounded daemon goroutine and returns immediately
// (under the configured OverflowPolicy), so the application's hot path is never
// blocked on network I/O. The daemon serializes each record (formattedBytes when the
// Logger pre-serialized, else Record.String) and writes it with a deadline; on
// any write error it closes the conn and re-dials on the next record (lazy
// reconnect with ReconnectBackoff).
//
// Use cases: shipping logs to a syslog/tcp collector, a sidecar (Fluentd,
// Vector), or a low-volume aggregation service. For high-volume shipping prefer
// FileWriter + Kafka — net throughput is bounded by network RTT and the remote.
type NetWriter struct {
	level   int
	paused  atomic.Bool
	options NetWriterOptions

	policy   OverflowPolicy
	spiller  Spiller[*Record]
	stats    OverflowStats
	sent     uint64
	errored  uint64
	dropped  uint64 // records dropped on write error (distinct from overflow drops)
	messages chan *Record

	timeout          time.Duration
	reconnectBackoff time.Duration

	connMu sync.Mutex
	conn   net.Conn

	run  atomic.Bool
	quit chan struct{}
	stop chan struct{}
	// closing is set (atomic) BEFORE Stop closes n.stop. Once set, producers
	// (Write) stop attempting to enqueue and the daemon's drainSpill stops
	// re-injecting into messages, so nothing sends on messages during shutdown.
	// Stop NEVER closes messages — closing it would race any concurrent Write
	// (close-vs-send is a true memory race and send-on-closed panics). The
	// daemon drains all pending records on the n.stop branch before exiting, so
	// leaving messages open is correct: any record a racing producer slipped in
	// after closing was set is either drained or left for GC (no panic, no race).
	// Mirrors FileWriter's shutdown-safe pattern.
	closing atomic.Bool
}

// NewNetWriter builds a NetWriter from options. It does NOT dial yet — the
// connection is opened lazily by the daemon on the first record (so a
// misconfigured remote does not block Init/Register).
func NewNetWriter(options NetWriterOptions) *NetWriter {
	defaultLevel := DEBUG
	if len(options.Level) > 0 {
		defaultLevel = getLevelDefault(options.Level, defaultLevel, "net_writer")
	}
	size := options.BufferSize
	if size <= 1 {
		size = 1024
	}
	w := &NetWriter{
		options:          options,
		level:            defaultLevel,
		policy:           ParseOverflowPolicy(options.OverflowPolicy),
		quit:             make(chan struct{}),
		stop:             make(chan struct{}),
		timeout:          options.Timeout,
		reconnectBackoff: options.ReconnectBackoff,
		// Allocate the bounded channel at construction (not in Init) so the
		// daemon — started by Init — reads an already-set field with no
		// Init-vs-daemon data race on the messages channel header.
		messages: make(chan *Record, size),
	}
	if w.timeout <= 0 {
		w.timeout = 3 * time.Second
	}
	if w.reconnectBackoff <= 0 {
		w.reconnectBackoff = 1 * time.Second
	}
	if w.policy == OverflowSpill {
		w.spiller = NewRingSpiller[*Record](options.SpillSize)
	}
	w.stats.SetAlertEvery(1000, 1000)
	return w
}

// Init starts the async daemon. It does NOT dial — the first dial happens
// lazily inside the daemon, so a down/misconfigured remote does not fail
// Logger.Register (which would otherwise panic).
func (n *NetWriter) Init() error {
	go n.daemon()
	return nil
}

// Name returns WriterNameNet.
func (n *NetWriter) Name() string { return WriterNameNet }

// Pause drops incoming records without removing the writer or closing the conn.
func (n *NetWriter) Pause() { n.paused.Store(true) }

// Resume restores delivery after Pause.
func (n *NetWriter) Resume() { n.paused.Store(false) }

// Paused reports whether the writer is currently paused.
func (n *NetWriter) Paused() bool { return n.paused.Load() }

// Write enqueues a private copy of r for async send under the OverflowPolicy.
// It NEVER blocks the caller under drop/spill (the block policy blocks, as the
// name promises). The record is copied because the bootstrap goroutine returns
// it to the record pool after Write returns.
//
// Shutdown safety: Stop sets n.closing and closes n.stop, but NEVER closes
// n.messages (see Stop docs). So Write can never panic on a closed channel. The
// closing fast path drops records once shutdown begins (keeping Stop bounded),
// and under OverflowBlock Write also selects on n.stop so a producer is unblocked
// when the daemon is winding down instead of waiting forever on a channel the
// daemon has stopped consuming. Mirrors FileWriter.send.
func (n *NetWriter) Write(r *Record) error {
	if n.paused.Load() {
		return nil
	}
	if r.level > n.level {
		return nil
	}
	if n.closing.Load() {
		// Shutdown in progress: drop instead of racing a send against the daemon
		// winding down. Keeps Stop bounded and avoids any send-after-stop hazard.
		n.stats.IncDropped()
		return nil
	}
	rc := *r // private copy for the daemon
	switch n.policy {
	case OverflowBlock:
		select {
		case n.messages <- &rc:
		case <-n.stop:
			n.stats.IncDropped()
		}
	default: // drop / spill
		select {
		case n.messages <- &rc:
		case <-n.stop:
			n.stats.IncDropped()
		default:
			if n.policy == OverflowSpill && n.spiller != nil && n.spiller.Push(&rc) {
				n.stats.IncSpilled()
			} else {
				n.stats.IncDropped()
			}
		}
	}
	return nil
}

// serialize returns the bytes to send for a record: the Logger's pre-serialized
// formattedBytes (FormatJSON) when present, else the text String() form. This makes
// NetWriter honor the Logger's format without its own format logic.
func (n *NetWriter) serialize(r *Record) []byte {
	if len(r.formattedBytes) > 0 {
		return r.formattedBytes
	}
	return []byte(r.String())
}

// dial opens the connection (tcp/udp). Caller must hold connMu.
func (n *NetWriter) dialLocked() error {
	network := n.options.Network
	if network == "" {
		network = "tcp"
	}
	d := net.Dialer{Timeout: n.timeout}
	c, err := d.Dial(network, n.options.Address)
	if err != nil {
		return err
	}
	n.conn = c
	return nil
}

// writeOne sends a single record, lazily (re)dialing as needed. On any error it
// closes the conn so the next call re-dials (lazy reconnect).
func (n *NetWriter) writeOne(r *Record) error {
	n.connMu.Lock()
	defer n.connMu.Unlock()

	if n.conn == nil {
		if err := n.dialLocked(); err != nil {
			atomic.AddUint64(&n.errored, 1)
			return err
		}
	}
	_ = n.conn.SetWriteDeadline(time.Now().Add(n.timeout))
	payload := n.serialize(r)
	if _, err := n.conn.Write(payload); err != nil {
		// write failed: close so the next writeOne re-dials. Drop this record
		// (it's already off the hot path; re-queuing risks a tight loop against
		// a persistently-down remote).
		_ = n.conn.Close()
		n.conn = nil
		atomic.AddUint64(&n.errored, 1)
		atomic.AddUint64(&n.dropped, 1)
		return err
	}
	atomic.AddUint64(&n.sent, 1)
	return nil
}

// daemon drains the messages channel (and spill store) and writes each record
// to the remote.
//
// Shutdown is driven by n.stop (closed by Stop), NOT by closing n.messages.
// This is the key to race-free shutdown: nothing ever closes messages, so there
// is no close-vs-send race with a concurrent Write. When stop fires the daemon
// drains everything still queued in messages (non-blocking) plus the entire
// spill store (written directly via writeOne), signals quit, and exits. The
// drainSpill re-inject path is gated on closing so it never runs during
// shutdown. The messages !ok branch is defensive only — messages is never closed
// in normal operation.
func (n *NetWriter) daemon() {
	defer func() {
		if r := recover(); r != nil {
			recordDaemonPanic("net", r)
		}
	}()
	n.run.Store(true)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case r, ok := <-n.messages:
			if !ok {
				// Defensive: messages should never be closed in normal operation
				// (Stop does not close it). If it ever is, treat as shutdown.
				n.drainAll()
				n.quit <- struct{}{}
				return
			}
			_ = n.writeOne(r)
		case <-ticker.C:
			n.drainSpill()
		case <-n.stop:
			n.drainAll()
			n.quit <- struct{}{}
			return
		}
	}
}

// drainSpill re-injects recovered records from the spill store (non-blocking).
//
// Once Stop has set closing, drainSpill is a no-op: the shutdown path is handled
// by drainAll (which writes directly via writeOne, bypassing messages), so
// re-injecting here would race the daemon winding down. Gated on closing to
// match FileWriter.drainSpill.
func (n *NetWriter) drainSpill() {
	if n.closing.Load() {
		return
	}
	if n.spiller == nil || n.spiller.Len() == 0 {
		return
	}
	for _, r := range n.spiller.Drain() {
		select {
		case n.messages <- r:
		default:
			_ = n.spiller.Push(r)
			return
		}
	}
}

// drainAll flushes queued messages and the spill store on shutdown. It is called
// from the daemon's shutdown branch (n.stop, or the defensive messages !ok
// path). It non-blocking-drains anything still queued in messages, then writes
// the entire spill store directly via writeOne (bypassing messages so it cannot
// race a producer that slipped past the closing gate).
func (n *NetWriter) drainAll() {
	for {
		select {
		case r, ok := <-n.messages:
			if !ok || r == nil {
				goto spill
			}
			_ = n.writeOne(r)
		default:
			goto spill
		}
	}
spill:
	if n.spiller == nil {
		return
	}
	for _, r := range n.spiller.Drain() {
		if r != nil {
			_ = n.writeOne(r)
		}
	}
}

// Stop shuts down the daemon gracefully.
//
// Race-free ordering (mirrors FileWriter.Stop):
//  1. closing=true -> new producers (Write) and the daemon's drainSpill stop
//     touching messages immediately; drainSpill returns early so it can never
//     re-inject into messages during shutdown.
//  2. close(stop)  -> unblocks any producer waiting in Write's OverflowBlock
//     branch, AND wakes the daemon's shutdown branch (it already selects on
//     n.stop).
//  3. wait <-quit   -> the daemon has drained every queued message + the entire
//     spill store (written directly via writeOne) and exited; then the conn is
//     closed.
//
// Crucially Stop NEVER closes n.messages — nothing does. Closing it would race
// any concurrent Write (close-vs-send is a true memory race and send-on-closed
// panics). The old code closed messages here, which panicked under concurrent
// Write+Stop. Safe to call once; a no-op if the daemon never started.
func (n *NetWriter) Stop() {
	if !n.run.CompareAndSwap(true, false) {
		return // already stopped, or another Stop in flight — atomic claim avoids a double close
	}
	n.closing.Store(true)
	close(n.stop)
	waitQuit("net", n.quit, defaultShutdownTimeout)
	n.connMu.Lock()
	if n.conn != nil {
		_ = n.conn.Close()
		n.conn = nil
	}
	n.connMu.Unlock()
}

// Flush is a no-op for NetWriter (writes are flushed inline by the daemon on
// each record; there is no buffered bufio to drain). Implementing Flusher keeps
// the bootstrap flush timer happy.
func (n *NetWriter) Flush() error { return nil }

// NetWriterMetrics is a point-in-time snapshot of NetWriter operational
// counters for monitoring.
type NetWriterMetrics struct {
	Sent            uint64 // records successfully written to the conn
	Errored         uint64 // write/dial errors
	Dropped         uint64 // records dropped on write error (after dequeue)
	Queued          int    // records currently buffered in the channel
	SpillLen        int    // records currently held in the spill store
	OverflowDropped uint64 // dropped on full channel (overflow policy)
	OverflowSpilled uint64 // moved to the spill store (spill policy)
}

// Metrics returns a snapshot of operational counters.
func (n *NetWriter) Metrics() NetWriterMetrics {
	queued := 0
	if n.messages != nil {
		queued = len(n.messages)
	}
	spillLen := 0
	if n.spiller != nil {
		spillLen = n.spiller.Len()
	}
	return NetWriterMetrics{
		Sent:            atomic.LoadUint64(&n.sent),
		Errored:         atomic.LoadUint64(&n.errored),
		Dropped:         atomic.LoadUint64(&n.dropped),
		Queued:          queued,
		SpillLen:        spillLen,
		OverflowDropped: n.stats.Dropped(),
		OverflowSpilled: n.stats.Spilled(),
	}
}

// String for debug.
func (n *NetWriter) String() string {
	m := n.Metrics()
	return fmt.Sprintf("NetWriter(%s://%s) sent=%d err=%d drop=%d queued=%d",
		n.options.Network, n.options.Address, m.Sent, m.Errored, m.OverflowDropped, m.Queued)
}

// compile-time: NetWriter implements Writer + Flusher.
var (
	_ Writer  = (*NetWriter)(nil)
	_ Flusher = (*NetWriter)(nil)
)
