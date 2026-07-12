//go:build !franzgo

package kafka

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

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

	errChOnce  sync.Once
	errCh      chan error
	msgChOnce  sync.Once
	msgCh      chan Message
	pumpCancel context.CancelFunc // set inside msgChOnce; cancelling stops the channel-mode pump so Close closes msgCh and a ranging caller unblocks

	received  atomic.Uint64
	acked     atomic.Uint64
	failed    atomic.Uint64
	recovered atomic.Uint64 // consumer handler panics recovered (observable; L5)
	bytes     atomic.Uint64

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
	cfg, err := buildSaramaConfig(o, false) // consumer: producer Flush section is unused
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
		// Cancellable ctx (not Background) so Close can stop the pump — otherwise
		// a pump blocked sending to a full msgCh can never be interrupted and the
		// caller ranging over Messages() hangs after Close.
		ctx, cancel := context.WithCancel(context.Background())
		s.pumpCancel = cancel
		go func() {
			_ = s.pump(ctx, nil, s.msgCh)
		}()
	})
	return s.msgCh
}

// pump drives the partition stream: callback mode (handler != nil) calls
// handler per message; channel mode (out != nil) forwards to out. It returns
// when ctx is cancelled or the stream closes.
func (s *saramaPartitionConsumer) pump(ctx context.Context, handler MessageHandler, out chan Message) error {
	if out != nil {
		// Channel mode: the pump is the sole sender to out, so closing it on exit
		// lets a caller ranging over Messages() unblock when the stream ends or
		// Close stops the pump. Skipped in callback mode (out == nil).
		defer close(out)
	}
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
				// safeHandlerCall runs the user handler with panic recovery: a
				// panicking handler is recovered (counted in Recovered()),
				// surfaced as a "nack" event, and the goroutine stays alive —
				// kit4go library-owned-worker convention (mirrors workerpool
				// safeCall). Without this a bad handler crashes the pump.
				if err := s.safeHandlerCall(handler, msg); err != nil {
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
	// Synchronise with channel-mode setup: if Messages() started the pump, stop
	// it before closing sarama's channels so a send blocked on a full msgCh
	// unblocks and the pump exits to close msgCh (mirrors the franz-go backend).
	s.msgChOnce.Do(func() {})
	if s.pumpCancel != nil {
		s.pumpCancel()
	}
	err := s.pc.Close()
	if err2 := s.consumer.Close(); err2 != nil && err == nil {
		err = err2
	}
	s.fire(ConsumerEvent{Name: "close"})
	return err
}

func (s *saramaPartitionConsumer) Metrics() ConsumerMetrics {
	return ConsumerMetrics{
		Received:  s.received.Load(),
		Acked:     s.acked.Load(),
		Failed:    s.failed.Load(),
		Recovered: s.recovered.Load(),
		Bytes:     s.bytes.Load(),
	}
}

// Recovered returns the total number of consumer-handler panics recovered since
// the consumer started. A panicking handler is recovered (counted here),
// surfaced as a "nack" ConsumerEvent, and the pump goroutine stays alive (does
// NOT re-panic). kit4go library-owned-worker recover convention; mirrors
// workerpool safeCall.
func (s *saramaPartitionConsumer) Recovered() uint64 { return s.recovered.Load() }

// safeHandlerCall invokes the user MessageHandler with panic recovery. A panic
// is turned into a "kafka: consumer handler panic" error, counted in recovered,
// and the goroutine survives. The error path then records the message as a NACK.
func (s *saramaPartitionConsumer) safeHandlerCall(handler MessageHandler, msg Message) (err error) {
	defer func() {
		if r := recover(); r != nil {
			s.recovered.Add(1)
			err = fmt.Errorf("kafka: consumer handler panic: %v", r)
		}
	}()
	return handler(msg)
}

func (s *saramaPartitionConsumer) Snapshot() ConsumerSnapshot {
	return ConsumerSnapshot{
		Name:            s.Name(),
		Backend:         s.Backend(),
		Timestamp:       time.Now().UTC(),
		ConsumerMetrics: s.Metrics(),
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
