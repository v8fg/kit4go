package log4go

import (
	"errors"
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
	wg   sync.WaitGroup
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

// Write enqueues r for async send under the OverflowPolicy. It NEVER blocks the
// caller under drop/spill (block policy blocks, as the name promises). The
// record is copied because the bootstrap goroutine returns it to the record
// pool after Write returns.
func (n *NetWriter) Write(r *Record) error {
	if r.level > n.level {
		return nil
	}
	rc := *r // private copy for the daemon
	switch n.policy {
	case OverflowBlock:
		n.messages <- &rc
	default: // drop / spill
		select {
		case n.messages <- &rc:
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
// to the remote. It runs for the life of the writer; Stop closes the messages
// channel to signal shutdown, and the daemon drains any remaining queued +
// spilled records before exiting.
func (n *NetWriter) daemon() {
	n.run.Store(true)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case r, ok := <-n.messages:
			if !ok {
				// channel closed (Stop): drain any remaining records, then exit.
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
func (n *NetWriter) drainSpill() {
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

// drainAll flushes queued messages and the spill store on shutdown. It is
// called after the messages channel is closed, so it must distinguish a real
// queued record from the zero-value nil returned by a closed channel.
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

// Stop shuts down the daemon gracefully: it closes the messages channel, the
// daemon drains queued + spilled records, then closes the conn. Safe to call
// once; a no-op if the daemon never started.
func (n *NetWriter) Stop() {
	if !n.run.Load() {
		return
	}
	close(n.messages)
	<-n.quit
	n.connMu.Lock()
	if n.conn != nil {
		_ = n.conn.Close()
		n.conn = nil
	}
	n.connMu.Unlock()
	n.run.Store(false) // mark stopped so a second Stop (e.g. via Logger.Close) is a no-op
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

// errNetWriterClosed is returned by callers that need a sentinel; the daemon
// itself never returns it (it logs and continues).
var errNetWriterClosed = errors.New("log4go: net writer closed")

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
