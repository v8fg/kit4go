//go:build franzgo

package kafka

import (
	"context"
	"sync/atomic"

	"github.com/twmb/franz-go/pkg/kgo"
)

// franzProducer is the franz-go async Producer implementation. Send calls
// client.Produce with a promise that updates Metrics — no separate drain
// goroutine (cleaner than sarama's channel-based Successes/Errors drain).
type franzProducer struct {
	opts Options
	cl   *kgo.Client

	closed        atomic.Bool
	enqueued      atomic.Uint64
	success       atomic.Uint64
	failed        atomic.Uint64
	bytes         atomic.Uint64
	batchCount    atomic.Uint64
	batchMax      atomic.Uint64
	bytesEnqueued atomic.Uint64

	onEvent atomic.Pointer[func(ProducerEvent)]
}

func (s *franzProducer) Send(ctx context.Context, msg Message) error {
	if s.closed.Load() {
		return ErrProducerClosed
	}
	r := toKgoRecord(msg, s.opts.Topic)
	s.enqueued.Add(1)
	s.bytesEnqueued.Add(uint64(len(msg.Value)))
	s.fire(ProducerEvent{Name: "send", Topic: r.Topic, Bytes: len(msg.Value)})
	// Produce is async: the promise fires on broker ack (or error). franz-go
	// does NOT close the client's internal channels on a full buffer, so there
	// is no send-on-closed panic risk; Produce blocks internally under backpres.
	s.cl.Produce(ctx, r, func(rec *kgo.Record, err error) {
		if err != nil {
			s.failed.Add(1)
			s.fire(ProducerEvent{Name: "error", Topic: r.Topic, Err: err})
			return
		}
		s.success.Add(1)
		n := uint64(len(rec.Value))
		s.bytes.Add(n)
		s.fire(ProducerEvent{Name: "success", Topic: rec.Topic, Partition: rec.Partition, Offset: rec.Offset, Bytes: int(n)})
	})
	return nil
}

func (s *franzProducer) SendBatch(ctx context.Context, msgs []Message) error {
	if s.closed.Load() {
		return ErrProducerClosed
	}
	n := uint64(len(msgs))
	s.batchCount.Add(1)
	for cur := n; cur > s.batchMax.Load(); s.batchMax.Store(cur) {
	}
	for _, msg := range msgs {
		r := toKgoRecord(msg, s.opts.Topic)
		s.cl.Produce(ctx, r, func(rec *kgo.Record, err error) {
			if err != nil {
				s.failed.Add(1)
				s.fire(ProducerEvent{Name: "error", Topic: r.Topic, Err: err})
				return
			}
			s.success.Add(1)
			nn := uint64(len(rec.Value))
			s.bytes.Add(nn)
			s.fire(ProducerEvent{Name: "success", Topic: rec.Topic, Partition: rec.Partition, Offset: rec.Offset, Bytes: int(nn)})
		})
	}
	for _, msg := range msgs {
		s.bytesEnqueued.Add(uint64(len(msg.Value)))
	}
	s.enqueued.Add(n)
	return nil
}

func (s *franzProducer) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	s.cl.Close() // flushes in-flight, then closes (void in franz-go)
	s.fire(ProducerEvent{Name: "close"})
	return nil
}

func (s *franzProducer) Metrics() ProducerMetrics {
	e, su, f := s.enqueued.Load(), s.success.Load(), s.failed.Load()
	be := s.bytesEnqueued.Load()
	ba := s.bytes.Load()
	return ProducerMetrics{
		Enqueued:      e,
		Success:       su,
		Failed:        f,
		Bytes:         ba,
		BytesEnqueued: be,
		BatchCount:    s.batchCount.Load(),
		BatchMax:      s.batchMax.Load(),
		InFlight:      ComputeInFlight(e, su, f),
		BufferedBytes: ComputeBufferedBytes(be, ba),
	}
}

func (s *franzProducer) Snapshot() ProducerSnapshot {
	return ProducerSnapshot{
		Name:             s.Name(),
		Backend:          s.Backend(),
		ProducerMetrics:  s.Metrics(),
		Linger:           s.opts.ProducerLinger,
		MaxBufferedRecs:  s.opts.MaxBufferedRecords,
		BatchMaxBytesCfg: s.opts.BatchMaxBytes,
	}
}

func (s *franzProducer) Name() string { return nameOr(s.opts.Name, s.opts.Topic) }

func (s *franzProducer) Backend() string { return backendName }

func (s *franzProducer) SetOnEvent(fn func(ProducerEvent)) {
	if fn == nil {
		s.onEvent.Store(nil)
		return
	}
	s.onEvent.Store(&fn)
}

func (s *franzProducer) fire(e ProducerEvent) {
	if fnp := s.onEvent.Load(); fnp != nil {
		(*fnp)(e)
	}
}

// franzSyncProducer is the franz-go sync Producer: Send blocks via ProduceSync.
type franzSyncProducer struct {
	opts Options
	cl   *kgo.Client

	closed   atomic.Bool
	enqueued atomic.Uint64
	success  atomic.Uint64
	failed   atomic.Uint64
	bytes    atomic.Uint64

	onEvent atomic.Pointer[func(ProducerEvent)]
}

func (s *franzSyncProducer) Send(ctx context.Context, msg Message) (int32, int64, error) {
	if s.closed.Load() {
		return 0, 0, ErrProducerClosed
	}
	r := toKgoRecord(msg, s.opts.Topic)
	s.enqueued.Add(1)
	s.fire(ProducerEvent{Name: "send", Topic: r.Topic, Bytes: len(msg.Value)})
	res := s.cl.ProduceSync(ctx, r)
	pr, perr := res.First()
	if perr != nil {
		s.failed.Add(1)
		s.fire(ProducerEvent{Name: "error", Topic: r.Topic, Err: perr})
		return 0, 0, perr
	}
	s.success.Add(1)
	s.bytes.Add(uint64(len(msg.Value)))
	s.fire(ProducerEvent{Name: "success", Topic: r.Topic, Partition: pr.Partition, Offset: pr.Offset, Bytes: len(msg.Value)})
	return pr.Partition, pr.Offset, nil
}

func (s *franzSyncProducer) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	s.cl.Close()
	s.fire(ProducerEvent{Name: "close"})
	return nil
}

func (s *franzSyncProducer) SendBatch(ctx context.Context, msgs []Message) error {
	if s.closed.Load() {
		return ErrProducerClosed
	}
	n := len(msgs)
	// franz-go's ProduceSync accepts variadic records → ONE FetchRecords request
	// to the broker for ALL records (real sync batching, unlike sarama's
	// one-at-a-time SendMessage). This is the sync batch advantage.
	records := make([]*kgo.Record, n)
	for i, msg := range msgs {
		records[i] = toKgoRecord(msg, s.opts.Topic)
	}
	s.enqueued.Add(uint64(n))
	res := s.cl.ProduceSync(ctx, records...)
	// Count per-record success/failure from ProduceResults.
	var firstErr error
	for i, pr := range res {
		if pr.Err != nil {
			s.failed.Add(1)
			if firstErr == nil {
				firstErr = pr.Err
			}
		} else {
			s.success.Add(1)
			s.bytes.Add(uint64(len(msgs[i].Value)))
		}
	}
	return firstErr // nil if all succeeded
}

func (s *franzSyncProducer) Metrics() ProducerMetrics {
	e, su, f := s.enqueued.Load(), s.success.Load(), s.failed.Load()
	be, ba := s.bytes.Load(), s.bytes.Load() // sync: enqueued ≈ acked (blocks per send)
	return ProducerMetrics{
		Enqueued:      e,
		Success:       su,
		Failed:        f,
		Bytes:         ba,
		BytesEnqueued: be,
		InFlight:      ComputeInFlight(e, su, f),
		BufferedBytes: 0, // sync: no buffered bytes (blocks per send)
	}
}

func (s *franzSyncProducer) Snapshot() ProducerSnapshot {
	return ProducerSnapshot{
		Name:            s.Name(),
		Backend:         s.Backend(),
		ProducerMetrics: s.Metrics(),
	}
}

func (s *franzSyncProducer) Name() string { return nameOr(s.opts.Name, s.opts.Topic) }

func (s *franzSyncProducer) Backend() string { return backendName }

func (s *franzSyncProducer) SetOnEvent(fn func(ProducerEvent)) {
	if fn == nil {
		s.onEvent.Store(nil)
		return
	}
	s.onEvent.Store(&fn)
}

func (s *franzSyncProducer) fire(e ProducerEvent) {
	if fnp := s.onEvent.Load(); fnp != nil {
		(*fnp)(e)
	}
}
