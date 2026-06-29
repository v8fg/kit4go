//go:build franzgo

package kafka

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// franzConsumerGroup is the franz-go ConsumerGroup. Consume runs a PollFetches
// loop (franz-go handles rebalance internally, unlike sarama's manual loop),
// invoking handler per record; MarkCommitRecords on ACK (auto-committed via
// AutoCommitMarks).
type franzConsumerGroup struct {
	opts Options
	cl   *kgo.Client

	closed    atomic.Bool
	received  atomic.Uint64
	acked     atomic.Uint64
	failed    atomic.Uint64
	rebalance atomic.Uint64
	bytes     atomic.Uint64

	errChOnce sync.Once
	errCh     chan error

	onEvent atomic.Pointer[func(ConsumerEvent)]
}

func (s *franzConsumerGroup) Consume(ctx context.Context, topics []string, handler MessageHandler) error {
	if s.closed.Load() {
		return ErrProducerClosed
	}
	// Create the kgo client lazily here (not in the constructor) so we can wire
	// the topics at creation time — franz-go requires ConsumeTopics at client
	// creation for group consuming to work.
	if s.cl == nil {
		kopts := kgoConsumerGroupOpts(s.opts)
		kopts = append(kopts, kgo.ConsumeTopics(topics...))
		cl, err := kgo.NewClient(kopts...)
		if err != nil {
			return err
		}
		s.cl = cl
	}
	for i := 0; ; i++ {
		if ctxDone(ctx) {
			return ctx.Err()
		}
		fetches := s.cl.PollFetches(ctx)
		for _, fe := range fetches.Errors() {
			s.failed.Add(1)
			s.fire(ConsumerEvent{Name: "error", Err: fe.Err})
			s.pushErr(fe.Err)
		}
		iter := fetches.RecordIter()
		for !iter.Done() {
			r := iter.Next()
			msg := fromKgoRecord(r)
			s.received.Add(1)
			s.bytes.Add(uint64(len(r.Value)))
			s.fire(ConsumerEvent{Name: "message", Msg: msg})
			if err := handler(msg); err != nil {
				s.failed.Add(1)
				s.fire(ConsumerEvent{Name: "nack", Msg: msg, Err: err})
				continue // NACK: do not mark → re-delivered next session
			}
			s.cl.MarkCommitRecords(r)
			s.acked.Add(1)
			s.fire(ConsumerEvent{Name: "ack", Msg: msg})
		}
	}
}

func (s *franzConsumerGroup) Errors() <-chan error {
	s.errChOnce.Do(func() { s.errCh = make(chan error, 16) })
	return s.errCh
}

func (s *franzConsumerGroup) pushErr(err error) {
	s.errChOnce.Do(func() { s.errCh = make(chan error, 16) })
	select {
	case s.errCh <- err:
	default:
	}
}

func (s *franzConsumerGroup) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	if s.cl != nil {
		s.cl.Close()
	}
	s.fire(ConsumerEvent{Name: "close"})
	return nil
}

func (s *franzConsumerGroup) Metrics() ConsumerMetrics {
	return ConsumerMetrics{
		Received:  s.received.Load(),
		Acked:     s.acked.Load(),
		Failed:    s.failed.Load(),
		Rebalance: s.rebalance.Load(),
		Bytes:     s.bytes.Load(),
	}
}

func (s *franzConsumerGroup) Snapshot() ConsumerSnapshot {
	return ConsumerSnapshot{
		Name:            s.Name(),
		Backend:         s.Backend(),
		Timestamp:       time.Now().UTC(),
		ConsumerMetrics: s.Metrics(),
	}
}

func (s *franzConsumerGroup) Name() string { return nameOr(s.opts.Name, s.opts.GroupID) }

func (s *franzConsumerGroup) Backend() string { return backendName }

func (s *franzConsumerGroup) SetOnEvent(fn func(ConsumerEvent)) {
	if fn == nil {
		s.onEvent.Store(nil)
		return
	}
	s.onEvent.Store(&fn)
}

func (s *franzConsumerGroup) fire(e ConsumerEvent) {
	if fnp := s.onEvent.Load(); fnp != nil {
		(*fnp)(e)
	}
}

// franzPartitionConsumer consumes one partition from a fixed offset (no group,
// no commits). Callback mode (Consume) or channel mode (Messages).
type franzPartitionConsumer struct {
	opts Options
	cl   *kgo.Client

	closed   atomic.Bool
	received atomic.Uint64
	acked    atomic.Uint64
	failed   atomic.Uint64
	bytes    atomic.Uint64

	errChOnce sync.Once
	errCh     chan error
	msgChOnce sync.Once
	msgCh     chan Message

	onEvent atomic.Pointer[func(ConsumerEvent)]
}

func (s *franzPartitionConsumer) Consume(ctx context.Context, handler MessageHandler) error {
	if s.closed.Load() {
		return ErrProducerClosed
	}
	return s.pump(ctx, handler, nil)
}

func (s *franzPartitionConsumer) Messages() <-chan Message {
	if s.opts.DeliveryMode != "channel" {
		return nil
	}
	s.msgChOnce.Do(func() {
		s.msgCh = make(chan Message, 64)
		go func() { _ = s.pump(context.Background(), nil, s.msgCh) }()
	})
	return s.msgCh
}

func (s *franzPartitionConsumer) pump(ctx context.Context, handler MessageHandler, out chan Message) error {
	for i := 0; ; i++ {
		if ctxDone(ctx) {
			return ctx.Err()
		}
		fetches := s.cl.PollFetches(ctx)
		for _, fe := range fetches.Errors() {
			s.failed.Add(1)
			s.fire(ConsumerEvent{Name: "error", Err: fe.Err})
			s.pushErr(fe.Err)
		}
		iter := fetches.RecordIter()
		for !iter.Done() {
			r := iter.Next()
			msg := fromKgoRecord(r)
			s.received.Add(1)
			s.bytes.Add(uint64(len(r.Value)))
			s.fire(ConsumerEvent{Name: "message", Msg: msg})
			if handler != nil {
				if err := handler(msg); err != nil {
					s.failed.Add(1)
					s.fire(ConsumerEvent{Name: "nack", Msg: msg, Err: err})
					continue
				}
				s.acked.Add(1)
				s.fire(ConsumerEvent{Name: "ack", Msg: msg})
			} else {
				select {
				case out <- msg:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}

func (s *franzPartitionConsumer) Errors() <-chan error {
	s.errChOnce.Do(func() { s.errCh = make(chan error, 16) })
	return s.errCh
}

func (s *franzPartitionConsumer) pushErr(err error) {
	s.errChOnce.Do(func() { s.errCh = make(chan error, 16) })
	select {
	case s.errCh <- err:
	default:
	}
}

func (s *franzPartitionConsumer) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	if s.cl != nil {
		s.cl.Close()
	}
	s.fire(ConsumerEvent{Name: "close"})
	return nil
}

func (s *franzPartitionConsumer) Metrics() ConsumerMetrics {
	return ConsumerMetrics{
		Received: s.received.Load(),
		Acked:    s.acked.Load(),
		Failed:   s.failed.Load(),
		Bytes:    s.bytes.Load(),
	}
}

func (s *franzPartitionConsumer) Snapshot() ConsumerSnapshot {
	return ConsumerSnapshot{
		Name:            s.Name(),
		Backend:         s.Backend(),
		Timestamp:       time.Now().UTC(),
		ConsumerMetrics: s.Metrics(),
	}
}

func (s *franzPartitionConsumer) Name() string { return nameOr(s.opts.Name, s.opts.Topic) }

func (s *franzPartitionConsumer) Backend() string { return backendName }

func (s *franzPartitionConsumer) SetOnEvent(fn func(ConsumerEvent)) {
	if fn == nil {
		s.onEvent.Store(nil)
		return
	}
	s.onEvent.Store(&fn)
}

func (s *franzPartitionConsumer) fire(e ConsumerEvent) {
	if fnp := s.onEvent.Load(); fnp != nil {
		(*fnp)(e)
	}
}
