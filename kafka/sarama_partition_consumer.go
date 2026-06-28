//go:build !franzgo

package kafka

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/IBM/sarama"
)

// consumerFactory is the test seam for the partition consumer (default
// sarama.NewConsumer).
type consumerFactory func(brokers []string, cfg *sarama.Config) (sarama.Consumer, error)

// saramaPartitionConsumer is the default PartitionConsumer implementation. It
// mirrors engine-master's inverted_file_listener: ConsumePartition on a fixed
// partition+offset, then deliver via callback (Consume) or channel (Messages).
type saramaPartitionConsumer struct {
	opts     Options
	cfg      *sarama.Config
	factory  consumerFactory
	consumer sarama.Consumer
	pc       sarama.PartitionConsumer

	mu     sync.Mutex
	closed bool

	errChOnce sync.Once
	errCh     chan error
	msgChOnce sync.Once
	msgCh     chan Message

	received atomic.Uint64
	acked    atomic.Uint64
	failed   atomic.Uint64
	bytes    atomic.Uint64

	onEvent atomic.Pointer[func(ConsumerEvent)]
}

// NewPartitionConsumer builds a single-partition consumer backed by sarama.
// WithBrokers, WithTopic and WithPartition are required. WithOffset selects the
// start point — OffsetNewest / OffsetOldest, or a concrete int64 >= 0 (0 is the
// first message, a valid absolute offset, NOT a sentinel).
func NewPartitionConsumer(opts ...Option) (PartitionConsumer, error) {
	o := applyOptions(opts)
	o = o.withDefaults()
	if err := o.validate("partition-consumer"); err != nil {
		return nil, err
	}
	return newSaramaPartitionConsumer(o, nil)
}

func newSaramaPartitionConsumer(o Options, factory consumerFactory) (*saramaPartitionConsumer, error) {
	cfg, err := buildSaramaConfig(o)
	if err != nil {
		return nil, err
	}
	if factory == nil {
		factory = sarama.NewConsumer
	}
	c, err := factory(o.Brokers, cfg)
	if err != nil {
		return nil, err
	}
	pc, err := c.ConsumePartition(o.Topic, o.Partition, mapOffsetInitial(o.Offset))
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	return &saramaPartitionConsumer{opts: o, cfg: cfg, factory: factory, consumer: c, pc: pc}, nil
}

// Consume invokes handler for each message on the configured partition. It
// blocks until ctx is cancelled. Errors from the partition stream are surfaced
// via Errors() and counted, but do not stop the loop.
func (s *saramaPartitionConsumer) Consume(ctx context.Context, handler MessageHandler) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrProducerClosed
	}
	s.mu.Unlock()
	return s.pump(ctx, handler, nil)
}

// Messages returns the message channel in channel delivery mode (started
// lazily on first call), or nil in callback mode. When started, a goroutine
// forwards the partition stream into the channel until Close.
func (s *saramaPartitionConsumer) Messages() <-chan Message {
	if s.opts.DeliveryMode != "channel" {
		return nil
	}
	s.msgChOnce.Do(func() {
		s.msgCh = make(chan Message, 64)
		go func() {
			_ = s.pump(context.Background(), nil, s.msgCh)
		}()
	})
	return s.msgCh
}

// pump drives the partition stream: callback mode (handler != nil) calls
// handler per message; channel mode (out != nil) forwards to out. It returns
// when ctx is cancelled or the stream closes.
func (s *saramaPartitionConsumer) pump(ctx context.Context, handler MessageHandler, out chan Message) error {
	for {
		select {
		case cm, ok := <-s.pc.Messages():
			if !ok {
				return nil
			}
			msg := fromSaramaConsumerMessage(cm)
			s.received.Add(1)
			s.bytes.Add(uint64(len(cm.Value)))
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
		case perr, ok := <-s.pc.Errors():
			if !ok {
				return nil
			}
			s.failed.Add(1)
			s.fire(ConsumerEvent{Name: "error", Err: perr.Err})
			s.pushErr(perr.Err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Errors returns a channel of background errors (partition stream errors).
func (s *saramaPartitionConsumer) Errors() <-chan error {
	s.errChOnce.Do(func() { s.errCh = make(chan error, 16) })
	return s.errCh
}

func (s *saramaPartitionConsumer) pushErr(err error) {
	s.errChOnce.Do(func() { s.errCh = make(chan error, 16) })
	select {
	case s.errCh <- err:
	default:
	}
}

func (s *saramaPartitionConsumer) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	err := s.pc.Close()
	if err2 := s.consumer.Close(); err2 != nil && err == nil {
		err = err2
	}
	s.fire(ConsumerEvent{Name: "close"})
	return err
}

func (s *saramaPartitionConsumer) Metrics() ConsumerMetrics {
	return ConsumerMetrics{
		Received: s.received.Load(),
		Acked:    s.acked.Load(),
		Failed:   s.failed.Load(),
		Bytes:    s.bytes.Load(),
	}
}

func (s *saramaPartitionConsumer) Name() string { return nameOr(s.opts.Name, s.opts.Topic) }

func (s *saramaPartitionConsumer) Backend() string { return backendName }

func (s *saramaPartitionConsumer) SetOnEvent(fn func(ConsumerEvent)) {
	if fn == nil {
		s.onEvent.Store(nil)
		return
	}
	s.onEvent.Store(&fn)
}

func (s *saramaPartitionConsumer) fire(e ConsumerEvent) {
	if fnp := s.onEvent.Load(); fnp != nil {
		(*fnp)(e)
	}
}
