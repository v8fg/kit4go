//go:build !franzgo

package kafka

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/IBM/sarama"
)

// consumerGroupFactory is the test seam for the consumer group (default
// sarama.NewConsumerGroup).
type consumerGroupFactory func(brokers []string, groupID string, cfg *sarama.Config) (sarama.ConsumerGroup, error)

// saramaConsumerGroup is the default ConsumerGroup implementation. It mirrors
// engine-master's consumerGroupProxy: Consume runs the infinite Consume loop
// (recreating the session after a server-side rebalance), and Close cancels the
// context and gracefully leaves the group.
type saramaConsumerGroup struct {
	opts    Options
	cfg     *sarama.Config
	factory consumerGroupFactory
	cg      sarama.ConsumerGroup

	mu     sync.Mutex
	closed bool

	// errCh fans out background errors (rebalance / broker errors from the
	// group's own Errors() channel). Created lazily so a consumer that nobody
	// listens to does not block.
	errChOnce sync.Once
	errCh     chan error

	received  atomic.Uint64
	acked     atomic.Uint64
	failed    atomic.Uint64
	rebalance atomic.Uint64
	bytes     atomic.Uint64

	onEvent atomic.Pointer[func(ConsumerEvent)]
}

// NewConsumerGroup builds a rebalance-aware ConsumerGroup backed by sarama.
// WithBrokers and WithGroupID are required.
func NewConsumerGroup(opts ...Option) (ConsumerGroup, error) {
	o := applyOptions(opts)
	o = o.withDefaults()
	if err := o.validate("consumer-group"); err != nil {
		return nil, err
	}
	return newSaramaConsumerGroup(o, nil)
}

func newSaramaConsumerGroup(o Options, factory consumerGroupFactory) (*saramaConsumerGroup, error) {
	cfg, err := buildSaramaConfig(o)
	if err != nil {
		return nil, err
	}
	if factory == nil {
		factory = sarama.NewConsumerGroup
	}
	cg, err := factory(o.Brokers, o.GroupID, cfg)
	if err != nil {
		return nil, err
	}
	s := &saramaConsumerGroup{opts: o, cfg: cfg, factory: factory, cg: cg}
	go s.drainErrors()
	return s, nil
}

// drainErrors forwards the group's background errors to errCh (and OnEvent).
func (s *saramaConsumerGroup) drainErrors() {
	for err := range s.cg.Errors() {
		s.fire(ConsumerEvent{Name: "error", Err: err})
		s.pushErr(err)
	}
}

func (s *saramaConsumerGroup) pushErr(err error) {
	s.errChOnce.Do(func() { s.errCh = make(chan error, 16) })
	select {
	case s.errCh <- err:
	default: // errCh full: drop to avoid blocking the drain goroutine
	}
}

// Consume subscribes to topics and invokes handler per message, in a loop that
// survives server-side rebalances (the session is recreated automatically by
// re-calling sarama's Consume). Returns ctx.Err() when ctx is cancelled, or a
// non-nil error on a fatal consume failure.
func (s *saramaConsumerGroup) Consume(ctx context.Context, topics []string, handler MessageHandler) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrProducerClosed // reuse the "closed" sentinel
	}
	s.mu.Unlock()

	h := &cgHandler{parent: s, handler: handler}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.cg.Consume(ctx, topics, h); err != nil {
			s.fire(ConsumerEvent{Name: "error", Err: err})
			return err
		}
		// Consume returned cleanly → a rebalance happened (session ended); the
		// loop recreates the session. Count it and continue unless ctx is done.
		if err := ctx.Err(); err != nil {
			return err
		}
		s.rebalance.Add(1)
		s.fire(ConsumerEvent{Name: "rebalance"})
	}
}

// Errors returns a channel of background errors. It is safe to ignore.
func (s *saramaConsumerGroup) Errors() <-chan error {
	s.errChOnce.Do(func() { s.errCh = make(chan error, 16) })
	return s.errCh
}

func (s *saramaConsumerGroup) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	err := s.cg.Close()
	s.fire(ConsumerEvent{Name: "close"})
	return err
}

func (s *saramaConsumerGroup) Metrics() ConsumerMetrics {
	return ConsumerMetrics{
		Received:  s.received.Load(),
		Acked:     s.acked.Load(),
		Failed:    s.failed.Load(),
		Rebalance: s.rebalance.Load(),
		Bytes:     s.bytes.Load(),
	}
}

// bumpReceived records a received message's bytes (called from the cgHandler).
func (s *saramaConsumerGroup) bumpReceived(valueLen int) {
	s.received.Add(1)
	s.bytes.Add(uint64(valueLen))
}

func (s *saramaConsumerGroup) Name() string { return nameOr(s.opts.Name, s.opts.GroupID) }

func (s *saramaConsumerGroup) Backend() string { return backendName }

func (s *saramaConsumerGroup) SetOnEvent(fn func(ConsumerEvent)) {
	if fn == nil {
		s.onEvent.Store(nil)
		return
	}
	s.onEvent.Store(&fn)
}

func (s *saramaConsumerGroup) fire(e ConsumerEvent) {
	if fnp := s.onEvent.Load(); fnp != nil {
		(*fnp)(e)
	}
}
