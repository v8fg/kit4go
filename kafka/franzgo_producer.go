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

	closed   atomic.Bool
	enqueued atomic.Uint64
	success  atomic.Uint64
	failed   atomic.Uint64
	bytes    atomic.Uint64

	onEvent atomic.Pointer[func(ProducerEvent)]
}

func (s *franzProducer) Send(ctx context.Context, msg Message) error {
	if s.closed.Load() {
		return ErrProducerClosed
	}
	r := toKgoRecord(msg, s.opts.Topic)
	s.enqueued.Add(1)
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

func (s *franzProducer) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	s.cl.Close() // flushes in-flight, then closes (void in franz-go)
	s.fire(ProducerEvent{Name: "close"})
	return nil
}

func (s *franzProducer) Metrics() ProducerMetrics {
	return ProducerMetrics{
		Enqueued: s.enqueued.Load(),
		Success:  s.success.Load(),
		Failed:   s.failed.Load(),
		Bytes:    s.bytes.Load(),
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

func (s *franzSyncProducer) Metrics() ProducerMetrics {
	return ProducerMetrics{
		Enqueued: s.enqueued.Load(),
		Success:  s.success.Load(),
		Failed:   s.failed.Load(),
		Bytes:    s.bytes.Load(),
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
