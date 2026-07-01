// Package fanout broadcasts a message to N subscribers (pub/sub fan-out).
// Each subscriber gets its own buffered channel; Publish is non-blocking (drops
// to a slow subscriber's full channel). Pure standard library.
//
// Ad-tech uses: broadcast a win/impression/click event to multiple downstream
// pipelines (logging, analytics, billing, attribution) from a single publish
// point — without coupling the producer to the consumers.
package fanout

import (
	"context"
	"sync"
	"sync/atomic"
)

// Fanout manages a set of subscribers and broadcasts messages to all of them.
type Fanout[T any] struct {
	mu         sync.RWMutex
	subs       map[uint64]*Subscription[T]
	nextID     uint64
	bufferSize int
	dropped    atomic.Uint64
	published  atomic.Uint64
	closed     atomic.Bool
}

// Subscription is a single subscriber's receive channel + metadata.
type Subscription[T any] struct {
	ID     uint64
	Ch     chan T
	fanout *Fanout[T]
}

// Option configures the Fanout.
type Option[T any] func(*Fanout[T])

// WithBufferSize sets the per-subscriber channel capacity (default 16).
func WithBufferSize[T any](n int) Option[T] {
	return func(f *Fanout[T]) {
		if n > 0 {
			f.bufferSize = n
		}
	}
}

// New builds a Fanout.
func New[T any](opts ...Option[T]) *Fanout[T] {
	f := &Fanout[T]{subs: make(map[uint64]*Subscription[T]), bufferSize: 16}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Subscribe registers a new subscriber and returns its subscription. The caller
// reads from sub.Ch. Unsubscribe via sub.Cancel() or fanout.Unsubscribe(sub).
func (f *Fanout[T]) Subscribe() *Subscription[T] {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	sub := &Subscription[T]{
		ID:     f.nextID,
		Ch:     make(chan T, f.bufferSize),
		fanout: f,
	}
	f.subs[sub.ID] = sub
	return sub
}

// Unsubscribe removes a subscriber and closes its channel.
func (f *Fanout[T]) Unsubscribe(sub *Subscription[T]) {
	if sub == nil {
		return
	}
	f.mu.Lock()
	s, ok := f.subs[sub.ID]
	if ok {
		delete(f.subs, sub.ID)
	}
	f.mu.Unlock()
	if ok {
		close(s.Ch)
	}
}

// Publish broadcasts msg to all subscribers. Non-blocking: if a subscriber's
// channel is full, the message is dropped for that subscriber (dropped counter
// incremented). Returns the number of subscribers that received the message.
// Safe for concurrent use with Close (holds a read lock during delivery).
func (f *Fanout[T]) Publish(msg T) int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.closed.Load() {
		return 0
	}
	f.published.Add(1)
	delivered := 0
	for _, s := range f.subs {
		select {
		case s.Ch <- msg:
			delivered++
		default:
			f.dropped.Add(1)
		}
	}
	return delivered
}

// PublishBlocking broadcasts msg and blocks until all subscribers have received
// it or ctx is cancelled. Holds a read lock during delivery.
func (f *Fanout[T]) PublishBlocking(ctx context.Context, msg T) (int, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.closed.Load() {
		return 0, false
	}
	f.published.Add(1)
	delivered := 0
	for _, s := range f.subs {
		select {
		case s.Ch <- msg:
			delivered++
		case <-ctx.Done():
			return delivered, false
		}
	}
	return delivered, true
}

// Subscribers returns the current subscriber count.
func (f *Fanout[T]) Subscribers() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.subs)
}

// Published returns the total messages published.
func (f *Fanout[T]) Published() uint64 { return f.published.Load() }

// Dropped returns the total messages dropped (subscriber channel full).
func (f *Fanout[T]) Dropped() uint64 { return f.dropped.Load() }

// Close unsubscribes everyone and closes all channels. Idempotent.
func (f *Fanout[T]) Close() {
	if !f.closed.CompareAndSwap(false, true) {
		return
	}
	f.mu.Lock()
	subs := f.subs
	f.subs = make(map[uint64]*Subscription[T])
	f.mu.Unlock()
	for _, s := range subs {
		close(s.Ch)
	}
}

// Cancel removes this subscription from the fanout and closes its channel.
func (s *Subscription[T]) Cancel() {
	if s.fanout != nil {
		s.fanout.Unsubscribe(s)
	}
}
