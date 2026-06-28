//go:build !franzgo

package kafka

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/IBM/sarama"
)

// syncProducerFactory is the test seam for the sync producer (default
// sarama.NewSyncProducer).
type syncProducerFactory func(brokers []string, cfg *sarama.Config) (sarama.SyncProducer, error)

// saramaSyncProducer is the default SyncProducer implementation.
type saramaSyncProducer struct {
	opts    Options
	topic   string
	cfg     *sarama.Config
	factory syncProducerFactory

	p sarama.SyncProducer

	mu     sync.RWMutex
	closed bool

	enqueued atomic.Uint64
	success  atomic.Uint64
	failed   atomic.Uint64
	bytes    atomic.Uint64

	onEvent atomic.Pointer[func(ProducerEvent)]
}

// NewSyncProducer builds a sync Producer (each Send blocks until the broker
// acks) backed by sarama. opts are applied with defaults; only WithBrokers is
// required. ReturnSuccesses is forced on (sarama requires it for SyncProducer).
func NewSyncProducer(opts ...Option) (SyncProducer, error) {
	o := applyOptions(opts)
	o.ReturnSuccesses = true
	o = o.withDefaults()
	if err := o.validate("producer"); err != nil {
		return nil, err
	}
	return newSaramaSyncProducer(o, nil)
}

func newSaramaSyncProducer(o Options, factory syncProducerFactory) (*saramaSyncProducer, error) {
	cfg, err := buildSaramaConfig(o)
	if err != nil {
		return nil, err
	}
	if factory == nil {
		factory = sarama.NewSyncProducer
	}
	sp, err := factory(o.Brokers, cfg)
	if err != nil {
		return nil, err
	}
	return &saramaSyncProducer{opts: o, topic: o.Topic, cfg: cfg, factory: factory, p: sp}, nil
}

func (s *saramaSyncProducer) Send(ctx context.Context, msg Message) (int32, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return 0, 0, ErrProducerClosed
	}
	pm := toSaramaProducerMessage(msg, s.topic)
	s.enqueued.Add(1)
	s.fire(ProducerEvent{Name: "send", Topic: pm.Topic, Bytes: len(msg.Value)})
	partition, offset, err := s.p.SendMessage(pm)
	if err != nil {
		s.failed.Add(1)
		s.fire(ProducerEvent{Name: "error", Topic: pm.Topic, Err: err})
		return 0, 0, err
	}
	s.success.Add(1)
	s.bytes.Add(uint64(len(msg.Value)))
	s.fire(ProducerEvent{Name: "success", Topic: pm.Topic, Partition: partition, Offset: offset, Bytes: len(msg.Value)})
	return partition, offset, nil
}

func (s *saramaSyncProducer) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	err := s.p.Close()
	s.fire(ProducerEvent{Name: "close"})
	return err
}

func (s *saramaSyncProducer) SendBatch(ctx context.Context, msgs []Message) error {
	// sarama's SendMessages pushes all records to Input() in a burst (internal
	// batching via Flush.Frequency/Messages/Bytes can combine them), then waits
	// for ALL acks — functionally equivalent to franz-go's variadic ProduceSync.
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrProducerClosed
	}
	n := len(msgs)
	pms := make([]*sarama.ProducerMessage, n)
	totalBytes := uint64(0)
	for i, msg := range msgs {
		pms[i] = toSaramaProducerMessage(msg, s.topic)
		totalBytes += uint64(len(msg.Value))
	}
	s.enqueued.Add(uint64(n))
	if err := s.p.SendMessages(pms); err != nil {
		// SendMessages returns on first error; we don't know exactly how many
		// succeeded — conservatively count all as failed.
		s.failed.Add(uint64(n))
		return err
	}
	s.success.Add(uint64(n))
	s.bytes.Add(totalBytes)
	return nil
}

func (s *saramaSyncProducer) Metrics() ProducerMetrics {
	e, su, f := s.enqueued.Load(), s.success.Load(), s.failed.Load()
	return ProducerMetrics{
		Enqueued: e,
		Success:  su,
		Failed:   f,
		Bytes:    s.bytes.Load(),
		InFlight: ComputeInFlight(e, su, f),
	}
}

func (s *saramaSyncProducer) Name() string { return nameOr(s.opts.Name, s.opts.Topic) }

func (s *saramaSyncProducer) Backend() string { return backendName }

func (s *saramaSyncProducer) SetOnEvent(fn func(ProducerEvent)) {
	if fn == nil {
		s.onEvent.Store(nil)
		return
	}
	s.onEvent.Store(&fn)
}

func (s *saramaSyncProducer) fire(e ProducerEvent) {
	if fnp := s.onEvent.Load(); fnp != nil {
		(*fnp)(e)
	}
}
