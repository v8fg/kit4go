package log4go

import (
	"bufio"
	"context"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/v8fg/kit4go/kafka"
)

// Cover the defensive recover() branches at the top of each writer daemon:
//   alert.go:161   (WebhookAlertSink.daemon)
//   file_writer.go:635  (FileWriter.daemon)
//   net_writer.go:230   (NetWriter.daemon)
//   kafka_writer.go:550 (KafkaWriter.daemon)
//
// Each daemon wraps its loop in `defer func() { if r := recover(); r != nil {
// recordDaemonPanic(...) } }()`. These branches exist precisely so a bug inside
// the loop degrades to a counted panic (RuntimeStats) instead of crashing the
// process. We exercise them by injecting a dependency whose method panics, then
// asserting daemonPanics advanced and the daemon exited (quit signalled).

// snapshotDaemonPanics returns the current counter and a restore func, mirroring
// snapshotMarshalPanics so each test asserts an isolated delta. Same-package
// tests run sequentially, so the snapshot/restore pair is race-free.
func snapshotDaemonPanics(t *testing.T) (before uint64, restore func()) {
	t.Helper()
	b := atomic.LoadUint64(&daemonPanics)
	return b, func() { atomic.StoreUint64(&daemonPanics, b) }
}

// panicKafkaProducer is a kafka.Producer (pointer) whose Send panics, to drive
// the kafka daemon's recover branch from inside the loop. It must be a pointer
// so producerNotNil's reflect.IsNil works (a struct value would panic there at
// daemon start instead of in the loop — still recovered, but less clean).
type panicKafkaProducer struct{}

func (*panicKafkaProducer) Send(context.Context, kafka.Message) error {
	panic("kafka-daemon-boom")
}
func (*panicKafkaProducer) SendBatch(context.Context, []kafka.Message) error {
	panic("not reached")
}
func (*panicKafkaProducer) Close() error                   { return nil }
func (*panicKafkaProducer) Metrics() kafka.ProducerMetrics { return kafka.ProducerMetrics{} }
func (*panicKafkaProducer) Snapshot() kafka.ProducerSnapshot {
	return kafka.ProducerSnapshot{}
}
func (*panicKafkaProducer) SetOnEvent(func(kafka.ProducerEvent)) {}
func (*panicKafkaProducer) Name() string                         { return "panic" }
func (*panicKafkaProducer) Backend() string                      { return "panic" }

// Test_KafkaWriter_daemon_RecoversPanic covers kafka_writer.go:550. A panicking
// producer Send makes sendOne panic inside the daemon loop; the recover records
// it. After recovery the daemon returns (the recover is the outermost defer, so
// it does NOT signal quit) — we assert solely on the daemonPanics counter.
func Test_KafkaWriter_daemon_RecoversPanic(t *testing.T) {
	before, restore := snapshotDaemonPanics(t)
	defer restore()

	w := &KafkaWriter{
		level:         INFO,
		policy:        OverflowDrop,
		messages:      make(chan kafka.Message, 4),
		producer:      &panicKafkaProducer{},
		drainInterval: 200 * time.Millisecond,
		quit:          make(chan struct{}, 1),
	}
	w.run.Store(true)
	go w.daemon()

	w.messages <- spillerMsg("t", "boom") // sendOne -> producer.Send panics

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadUint64(&daemonPanics) >= before+1 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if got := atomic.LoadUint64(&daemonPanics); got != before+1 {
		t.Fatalf("daemonPanics=%d want %d (kafka panic should be counted)", got, before+1)
	}
}

// Test_WebhookAlertSink_daemon_RecoversPanic covers alert.go:161. A panicking
// formatter makes the daemon's formatter(...) call panic; recover records it.
func Test_WebhookAlertSink_daemon_RecoversPanic(t *testing.T) {
	before, restore := snapshotDaemonPanics(t)
	defer restore()

	w := &WebhookAlertSink{
		url:       "http://example/x",
		client:    nil, // not reached: formatter panics first
		formatter: func(AlertLevel, string, string) (string, []byte) { panic("alert-daemon-boom") },
		ch:        make(chan alertMsg, 4),
		quit:      make(chan struct{}),
	}
	w.wg.Add(1) // daemon does defer w.wg.Done(); balance the WaitGroup (matches FileWriter panic test)
	go w.daemon()

	w.ch <- alertMsg{level: AlertError, kind: "k", text: "boom"} // formatter panics

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadUint64(&daemonPanics) >= before+1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	// signal shutdown so the daemon (if it somehow survived) doesn't leak.
	w.once.Do(func() { close(w.quit) })

	if got := atomic.LoadUint64(&daemonPanics); got != before+1 {
		t.Fatalf("daemonPanics=%d want %d (alert panic should be counted)", got, before+1)
	}
}

// panicConn is a net.Conn whose every method panics, to drive the net daemon's
// recover branch. writeOne calls conn.SetWriteDeadline then conn.Write.
type panicConn struct{}

func (panicConn) Read([]byte) (int, error)         { panic("net-daemon-boom") }
func (panicConn) Write([]byte) (int, error)        { panic("net-daemon-boom") }
func (panicConn) Close() error                     { panic("net-daemon-boom") }
func (panicConn) LocalAddr() net.Addr              { return nil }
func (panicConn) RemoteAddr() net.Addr             { return nil }
func (panicConn) SetDeadline(time.Time) error      { panic("net-daemon-boom") }
func (panicConn) SetReadDeadline(time.Time) error  { panic("net-daemon-boom") }
func (panicConn) SetWriteDeadline(time.Time) error { panic("net-daemon-boom") }

// Test_NetWriter_daemon_RecoversPanic covers net_writer.go:230. A pre-set
// panicking conn makes writeOne panic on SetWriteDeadline; recover records it.
func Test_NetWriter_daemon_RecoversPanic(t *testing.T) {
	before, restore := snapshotDaemonPanics(t)
	defer restore()

	n := &NetWriter{
		messages: make(chan *Record, 4),
		quit:     make(chan struct{}, 1),
		stop:     make(chan struct{}),
		conn:     panicConn{}, // writeOne -> SetWriteDeadline panics
	}
	go n.daemon()

	n.messages <- &Record{level: INFO, msg: "boom"} // writeOne -> conn panic

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadUint64(&daemonPanics) >= before+1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	// best-effort cleanup so the daemon can't leak.
	select {
	case n.stop <- struct{}{}:
	default:
	}

	if got := atomic.LoadUint64(&daemonPanics); got != before+1 {
		t.Fatalf("daemonPanics=%d want %d (net panic should be counted)", got, before+1)
	}
}

// Test_FileWriter_daemon_RecoversPanic covers file_writer.go:635. We construct
// a bare FileWriter (bypassing the constructor + package logger bootstrap) and
// run daemon() directly, then send a Record with an out-of-range level. writeOne
// calls r.String(), which indexes LevelFlags[r.level] and panics out of bounds.
// The daemon's recover records it. After recovery the daemon returns (the
// recover is the outermost defer); we assert solely on the daemonPanics counter.
func Test_FileWriter_daemon_RecoversPanic(t *testing.T) {
	before, restore := snapshotDaemonPanics(t)
	defer restore()

	f, err := os.CreateTemp(t.TempDir(), "panic*.log")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()

	w := &FileWriter{
		messages:      make(chan *Record, 4),
		flushInterval: 10 * time.Second, // long: we panic before any tick
		quit:          make(chan struct{}, 1),
		stop:          make(chan struct{}),
		flushSig:      make(chan struct{}, 1),
		fileBufWriter: bufio.NewWriter(f), // non-nil so writeOne reaches r.String()
	}
	w.wg.Add(1) // daemon does defer w.wg.Done()
	go w.daemon()

	// An out-of-range level indexes LevelFlags out of bounds inside r.String()
	// (writeOne calls it when formattedBytes is empty), panicking out of writeOne
	// and tripping the daemon's recover.
	w.messages <- &Record{msg: "boom", level: len(LevelFlags) + 100}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadUint64(&daemonPanics) >= before+1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	w.wg.Wait()

	if got := atomic.LoadUint64(&daemonPanics); got != before+1 {
		t.Fatalf("daemonPanics=%d want %d (file panic should be counted)", got, before+1)
	}
}
