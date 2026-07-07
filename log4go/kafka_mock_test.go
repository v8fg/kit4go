package log4go

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/v8fg/kit4go/kafka"
)

var errMockKafka = errors.New("mock kafka fail")

// mockKafkaProducer is a test double for kafka.Producer, used by KafkaWriter
// tests to verify the daemon / overflow / spill logic without a real broker.
// Send records every message; if fail is set it fires the "error" OnEvent so
// the daemon's error accounting is exercised.
type mockKafkaProducer struct {
	mu             sync.Mutex
	sent           []kafka.Message
	sendCalls      int // per-record Send invocations
	sendBatchCalls int // SendBatch invocations
	sendBatchRecs  int // total records delivered via SendBatch
	onEvent        func(kafka.ProducerEvent)
	closed         bool
	fail           bool
	// callDelay simulates per-CALL cost (network/serialization/initiation) — slept
	// ONCE per Send/SendBatch call (not per record), so SendBatch amortizes it
	// across the whole batch. Default 0 (no impact on other tests). Used to show
	// that BatchMode wins when the producer call is the bottleneck.
	callDelay time.Duration
	// sendErr, when non-nil, makes Send return this error synchronously (the
	// record is NOT appended). Exercises the writer's client-side error paths
	// (sendOne err branch, drainSpillToProducer no-op).
	sendErr error
	// sendBatchErr, when non-nil, makes SendBatch return this error
	// synchronously. Exercises the batch flush error/drop branches.
	sendBatchErr error
}

func newMockKafkaProducer() *mockKafkaProducer { return &mockKafkaProducer{} }

func (m *mockKafkaProducer) Send(_ context.Context, msg kafka.Message) error {
	if m.callDelay > 0 {
		time.Sleep(m.callDelay) // per-call cost (once per Send)
	}
	if m.sendErr != nil {
		m.mu.Lock()
		m.sendCalls++
		m.mu.Unlock()
		return m.sendErr
	}
	m.mu.Lock()
	m.sent = append(m.sent, msg)
	m.sendCalls++
	m.mu.Unlock()
	if m.fail && m.onEvent != nil {
		m.onEvent(kafka.ProducerEvent{Name: "error", Err: errMockKafka, Topic: msg.Topic})
	}
	return nil
}

func (m *mockKafkaProducer) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockKafkaProducer) Metrics() kafka.ProducerMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	var bytes uint64
	for _, msg := range m.sent {
		bytes += uint64(len(msg.Value))
	}
	n := uint64(len(m.sent))
	return kafka.ProducerMetrics{
		Enqueued:      n,
		InFlight:      n, // mock: all sent records treated as in-flight (no ack tracking)
		BytesEnqueued: bytes,
		BufferedBytes: bytes,
	}
}

func (m *mockKafkaProducer) SetOnEvent(fn func(kafka.ProducerEvent)) { m.onEvent = fn }
func (m *mockKafkaProducer) Name() string                            { return "mock" }
func (m *mockKafkaProducer) Backend() string                         { return "mock" }

func (m *mockKafkaProducer) SendBatch(_ context.Context, msgs []kafka.Message) error {
	if m.callDelay > 0 {
		time.Sleep(m.callDelay) // per-call cost ONCE (amortized across the batch)
	}
	if m.sendBatchErr != nil {
		m.mu.Lock()
		m.sendBatchCalls++
		m.mu.Unlock()
		return m.sendBatchErr
	}
	m.mu.Lock()
	m.sent = append(m.sent, msgs...)
	m.sendBatchCalls++
	m.sendBatchRecs += len(msgs)
	m.mu.Unlock()
	return nil
}

// callCounts returns (sendCalls, sendBatchCalls) for verifying which daemon
// path was used (per-record Send vs SendBatch).
func (m *mockKafkaProducer) callCounts() (sends, batches int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sendCalls, m.sendBatchCalls
}

func (m *mockKafkaProducer) Snapshot() kafka.ProducerSnapshot {
	return kafka.ProducerSnapshot{
		Name:            "mock",
		Backend:         "mock",
		Timestamp:       time.Now().UTC(), // faithful to the real producers
		ProducerMetrics: m.Metrics(),
	}
}

func (m *mockKafkaProducer) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}
