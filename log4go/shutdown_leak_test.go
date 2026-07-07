package log4go

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/v8fg/kit4go/kafka"
)

// This file rigorously verifies shutdown cleanliness: no goroutine, channel, or
// resource (kafka producer, file handle, net conn) survives a correct shutdown.
// goleak.VerifyNone with IgnoreCurrent snapshots the goroutines alive at test
// entry (e.g. the package singleton's idle bootstrap) and only fails if a
// goroutine STARTED during the test is still alive at exit — i.e. a real leak
// attributable to the writer under test.

// newStartedMockKafka builds a KafkaWriter whose Start() succeeds with no broker
// (a no-op mock producer is injected), so its daemon + success/error drainer
// goroutines are live — the realistic "writer running" state.
func newStartedMockKafka(t *testing.T) *KafkaWriter {
	t.Helper()
	kw := NewKafkaWriter(KafkaWriterOptions{ProducerTopic: "t", BufferSize: 16})
	kw.producerFactory = func() (kafka.Producer, error) {
		return func() *mockKafkaProducer { m := newMockKafkaProducer(); m.fail = true; return m }(), nil
	}
	if err := kw.Start(); err != nil {
		t.Fatalf("kafka Start: %v", err)
	}
	return kw
}

// sinkListener accepts and immediately closes conns so NetWriter can dial it
// without a real peer. The accept loop exits when the listener is closed.
func sinkListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return ln
}

// waitNetRunning polls n.run until the daemon marks itself running; NetWriter.Stop
// is a no-op until run is true, so an immediate Stop would skip shutdown.
func waitNetRunning(t *testing.T, n *NetWriter) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !n.run.Load() {
		time.Sleep(time.Millisecond)
	}
	if !n.run.Load() {
		t.Fatal("net daemon did not mark itself running within 2s")
	}
}

// TestShutdown_Stop_NoGoroutineLeak verifies each writer's Stop() fully reclaims
// its goroutines, channels, and resources. This exercises the KafkaWriter
// shutdown path after the dead `case <-k.stop` arm was removed: shutdown now
// flows solely through close(messages) -> daemon sees ok=false -> quit signal ->
// producer.Close() (which closes Successes/Errors, ending the two drainers).
func TestShutdown_Stop_NoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	kw := newStartedMockKafka(t)
	waitDaemonRunning(t, kw) // defined in coverage_boost_kafka_test.go

	fw := NewFileWriterWithOptions(FileWriterOptions{
		Filename:        filepath.Join(t.TempDir(), "f.log"),
		Async:           true,
		AsyncBufferSize: 16,
	})
	if err := fw.Init(); err != nil {
		t.Fatalf("file Init: %v", err)
	}

	ln := sinkListener(t)
	defer ln.Close() // runs before the goleak defer (LIFO), ending the accept loop
	nw := NewNetWriter(NetWriterOptions{
		Network:    "tcp",
		Address:    ln.Addr().String(),
		BufferSize: 16,
	})
	if err := nw.Init(); err != nil {
		t.Fatalf("net Init: %v", err)
	}
	waitNetRunning(t, nw)

	time.Sleep(30 * time.Millisecond) // let daemons settle / drain any handshake

	kw.Stop()
	fw.Stop()
	nw.Stop()
	// Deferred goleak.VerifyNone asserts no goroutine survives: kafka daemon +
	// 2 drainers, the mock producer worker, the file daemon, the net daemon, and
	// the accept loop must all be gone.
}

// TestShutdown_LoggerClose_StopsWriters verifies Logger.Close() stops every
// registered Stopper writer (File/Kafka/Net), so a single log4go.Close()
// reclaims all writer daemons/handles/conns — no manual Stop() needed and no
// leak. Asserted by checking the FileWriter daemon is gone after Close (its
// messages channel is nil'd by Stop) and by goleak finding no survivors.
func TestShutdown_LoggerClose_StopsWriters(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	l := newLoggerWithRecords(make(chan *Record, 16))
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Filename:        filepath.Join(t.TempDir(), "f.log"),
		Async:           true,
		AsyncBufferSize: 8,
	})
	l.Register(fw) // Register calls Init() once -> single async daemon

	l.Close()

	if fw.messages != nil {
		t.Fatalf("Logger.Close did not stop FileWriter daemon (messages still open) — leak")
	}
	// No manual Stop() is required: Logger.Close stopped it. Idempotency means a
	// later explicit Stop() is also safe (no close-of-closed-channel panic).
	fw.Stop()
}
