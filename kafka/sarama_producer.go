//go:build !franzgo

package kafka

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
)

// asyncProducerFactory is the seam used to inject a mock AsyncProducer in
// tests. The default is sarama.NewAsyncProducer.
type asyncProducerFactory func(brokers []string, cfg *sarama.Config) (sarama.AsyncProducer, error)

// saramaProducer is the default async Producer implementation.
type saramaProducer struct {
	opts    Options
	topic   string
	cfg     *sarama.Config
	factory asyncProducerFactory

	p sarama.AsyncProducer

	// close coordination: Send takes RLock (concurrent sends); Close takes Lock
	// (waits for in-flight sends to drain), flips closed, then closes the
	// producer OUTSIDE the lock. New Sends RLock, see closed, return. No
	// send-on-closed-channel panic.
	mu     sync.RWMutex
	closed bool

	enqueued      atomic.Uint64
	success       atomic.Uint64
	failed        atomic.Uint64
	bytes         atomic.Uint64
	batchCount    atomic.Uint64
	batchMax      atomic.Uint64
	bytesEnqueued atomic.Uint64
	bytesFailed   atomic.Uint64

	history *snapshotHistory // nil when WithSnapshotHistory not set (zero overhead)

	onEvent atomic.Pointer[func(ProducerEvent)]
}

// NewProducer builds an async Producer backed by sarama. opts are applied with
// defaults; only WithBrokers is required.
func NewProducer(opts ...Option) (Producer, error) {
	o := applyOptions(opts)
	o = o.withDefaults()
	if err := o.validate("producer"); err != nil {
		return nil, err
	}
	logConfig("async-producer", o)
	return newSaramaProducer(o, nil)
}

func newSaramaProducer(o Options, factory asyncProducerFactory) (*saramaProducer, error) {
	cfg, err := buildSaramaConfig(o, false)
	if err != nil {
		return nil, err
	}
	if factory == nil {
		factory = sarama.NewAsyncProducer
	}
	ap, err := factory(o.Brokers, cfg)
	if err != nil {
		return nil, err
	}
	s := &saramaProducer{opts: o, topic: o.Topic, cfg: cfg, factory: factory, p: ap, history: newSnapshotHistory(o.SnapshotHistory)}
	s.startDrains()
	return s, nil
}

// startDrains spins the Successes/Errors drain goroutines (engine-master
// producerProxy pattern). They update Metrics and fire OnEvent, exiting when
// the producer is Closed (channels close).
func (s *saramaProducer) startDrains() {
	go func() {
		for pm := range s.p.Successes() {
			s.success.Add(1)
			n := uint64(encLen(pm.Value))
			s.bytes.Add(n)
			s.fire(ProducerEvent{Name: "success", Topic: pm.Topic, Partition: pm.Partition, Offset: pm.Offset, Bytes: int(n)})
		}
	}()
	go func() {
		for pe := range s.p.Errors() {
			s.failed.Add(1)
			s.bytesFailed.Add(uint64(encLen(pe.Msg.Value)))
			s.fire(ProducerEvent{Name: "error", Topic: pe.Msg.Topic, Err: pe.Err})
		}
	}()
}

func (s *saramaProducer) Send(ctx context.Context, msg Message) error {
	if !s.opts.ReturnSuccesses {
		// Without the success path we still accept the send; accounting of
		// success is simply unavailable (MatchesSuccesses=false would panic at
		// construction, prevented by withDefaults).
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrProducerClosed
	}
	if s.opts.Codec != nil {
		// Codec is applied by the caller normally (raw bytes win); this branch
		// is a no-op placeholder kept for a future "Send any" API.
	}
	pm := toSaramaProducerMessage(msg, s.topic)
	select {
	case s.p.Input() <- pm:
	case <-ctx.Done():
		return ctx.Err()
	}
	s.enqueued.Add(1)
	s.bytesEnqueued.Add(uint64(len(msg.Value)))
	s.fire(ProducerEvent{Name: "send", Topic: pm.Topic, Bytes: len(msg.Value)})
	return nil
}

func (s *saramaProducer) Close() error {
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

func (s *saramaProducer) SendBatch(ctx context.Context, msgs []Message) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrProducerClosed
	}
	n := uint64(len(msgs))
	s.batchCount.Add(1)
	for cur := n; cur > s.batchMax.Load(); s.batchMax.Store(cur) {
	} // CAS-free max update (single-writer Send path under RLock)
	for _, msg := range msgs {
		pm := toSaramaProducerMessage(msg, s.topic)
		select {
		case s.p.Input() <- pm:
		case <-ctx.Done():
			return ctx.Err()
		}
		// Count per-message INSIDE the loop, immediately after a successful
		// Input() push. Counting after the loop (the old form) lost every
		// already-pushed message when ctx cancelled mid-batch: Enqueued stayed
		// 0 while Success drained them — a metrics inversion (R21-F4).
		s.enqueued.Add(1)
		s.bytesEnqueued.Add(uint64(len(msg.Value)))
	}
	s.fire(ProducerEvent{Name: "send", Topic: s.topic, Bytes: 0})
	return nil
}

func (s *saramaProducer) Metrics() ProducerMetrics {
	e, su, f := s.enqueued.Load(), s.success.Load(), s.failed.Load()
	be := s.bytesEnqueued.Load()
	ba := s.bytes.Load()
	return ProducerMetrics{
		Enqueued:      e,
		Success:       su,
		Failed:        f,
		Bytes:         ba,
		BytesFailed:   s.bytesFailed.Load(),
		BytesEnqueued: be,
		BatchCount:    s.batchCount.Load(),
		BatchMax:      s.batchMax.Load(),
		InFlight:      ComputeInFlight(e, su, f),
		BufferedBytes: ComputeBufferedBytes(be, ba, s.bytesFailed.Load()),
	}
}

func (s *saramaProducer) Snapshot() ProducerSnapshot {
	snap := ProducerSnapshot{
		Name:             s.Name(),
		Backend:          s.Backend(),
		Timestamp:        time.Now().UTC(),
		ProducerMetrics:  s.Metrics(),
		Linger:           effectiveLinger(s.opts.ProducerLinger),
		MaxBufferedRecs:  s.opts.MaxBufferedRecords,
		BatchMaxBytesCfg: s.opts.BatchMaxBytes,
	}
	s.history.record(snap) // no-op when history disabled (nil)
	return snap
}

// History implements the optional SnapshotHistory interface (present only when
// WithSnapshotHistory is set). Returns retained samples oldest→newest, or nil.
func (s *saramaProducer) History() []ProducerSnapshot { return s.history.snapshot() }

// Name returns the instance name (WithName, else the topic) for monitoring.
func (s *saramaProducer) Name() string { return nameOr(s.opts.Name, s.opts.Topic) }

// Backend returns the underlying client library ("sarama").
func (s *saramaProducer) Backend() string { return backendName }

func (s *saramaProducer) SetOnEvent(fn func(ProducerEvent)) {
	if fn == nil {
		s.onEvent.Store(nil)
		return
	}
	s.onEvent.Store(&fn)
}

// fire invokes the OnEvent hook if installed; zero-cost when nil (a single
// atomic Pointer load).
func (s *saramaProducer) fire(e ProducerEvent) {
	if fnp := s.onEvent.Load(); fnp != nil {
		(*fnp)(e)
	}
}

// encLen returns the encoded length of a sarama Encoder (0 for nil/unknown).
func encLen(v sarama.Encoder) int {
	if v == nil {
		return 0
	}
	return v.Length()
}
