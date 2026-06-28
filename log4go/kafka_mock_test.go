package log4go

import (
	"context"
	"errors"
	"sync"

	"github.com/v8fg/kit4go/kafka"
)

var errMockKafka = errors.New("mock kafka fail")

// mockKafkaProducer is a test double for kafka.Producer, used by KafKaWriter
// tests to verify the daemon / overflow / spill logic without a real broker.
// Send records every message; if fail is set it fires the "error" OnEvent so
// the daemon's error accounting is exercised.
type mockKafkaProducer struct {
	mu      sync.Mutex
	sent    []kafka.Message
	onEvent func(kafka.ProducerEvent)
	closed  bool
	fail    bool
}

func newMockKafkaProducer() *mockKafkaProducer { return &mockKafkaProducer{} }

func (m *mockKafkaProducer) Send(_ context.Context, msg kafka.Message) error {
	m.mu.Lock()
	m.sent = append(m.sent, msg)
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
	return kafka.ProducerMetrics{Enqueued: uint64(len(m.sent))}
}

func (m *mockKafkaProducer) SetOnEvent(fn func(kafka.ProducerEvent)) { m.onEvent = fn }
func (m *mockKafkaProducer) Name() string                            { return "mock" }
func (m *mockKafkaProducer) Backend() string                         { return "mock" }

func (m *mockKafkaProducer) SendBatch(_ context.Context, msgs []kafka.Message) error {
	m.mu.Lock()
	m.sent = append(m.sent, msgs...)
	m.mu.Unlock()
	return nil
}

func (m *mockKafkaProducer) Snapshot() kafka.ProducerSnapshot {
	return kafka.ProducerSnapshot{
		Name:            "mock",
		Backend:         "mock",
		ProducerMetrics: m.Metrics(),
	}
}

func (m *mockKafkaProducer) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}
