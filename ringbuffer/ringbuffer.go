// Package ringbuffer is a generic, fixed-capacity ring buffer (circular buffer)
// with blocking and non-blocking push/pop. Pure standard library.
//
// When the buffer is full, Push blocks (backpressure) and TryPush drops. When
// empty, Pop blocks and TryPop returns false. This makes it suitable as a bounded
// channel-like queue between producers and consumers without allocating a Go
// channel's full machinery.
//
// Ad-tech uses: streaming event windows (impression/click rings), bounded metric
// sample buffers, and producer-consumer decoupling where memory must be capped.
package ringbuffer

import (
	"errors"
	"sync"
)

// ErrClosed is returned by Push/Pop after Close.
var ErrClosed = errors.New("ringbuffer: closed")

// RingBuffer is a fixed-capacity circular buffer of type T.
type RingBuffer[T any] struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []T
	head   int // next write position
	tail   int // next read position
	count  int
	cap    int
	closed bool
}

// New builds a ring buffer with the given capacity (must be > 0).
func New[T any](capacity int) *RingBuffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	rb := &RingBuffer[T]{
		buf: make([]T, capacity),
		cap: capacity,
	}
	rb.cond = sync.NewCond(&rb.mu)
	return rb
}

// Push blocks until there is room, then writes item. Returns ErrClosed if the
// buffer is closed.
func (rb *RingBuffer[T]) Push(item T) error {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	for rb.count == rb.cap && !rb.closed {
		rb.cond.Wait()
	}
	if rb.closed {
		return ErrClosed
	}
	rb.buf[rb.head] = item
	rb.head = (rb.head + 1) % rb.cap
	rb.count++
	rb.cond.Broadcast() // wake a blocked Pop
	return nil
}

// TryPush writes item without blocking. Returns false if the buffer is full or
// closed.
func (rb *RingBuffer[T]) TryPush(item T) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.count == rb.cap || rb.closed {
		return false
	}
	rb.buf[rb.head] = item
	rb.head = (rb.head + 1) % rb.cap
	rb.count++
	rb.cond.Broadcast()
	return true
}

// Pop blocks until an item is available. Returns ErrClosed if the buffer is
// closed and empty.
func (rb *RingBuffer[T]) Pop() (T, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	for rb.count == 0 && !rb.closed {
		rb.cond.Wait()
	}
	if rb.count == 0 && rb.closed {
		var zero T
		return zero, ErrClosed
	}
	item := rb.buf[rb.tail]
	rb.buf[rb.tail] = *new(T) // help GC
	rb.tail = (rb.tail + 1) % rb.cap
	rb.count--
	rb.cond.Broadcast() // wake a blocked Push
	return item, nil
}

// TryPop returns an item without blocking. Returns false if empty or closed.
func (rb *RingBuffer[T]) TryPop() (T, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.count == 0 {
		var zero T
		return zero, false
	}
	item := rb.buf[rb.tail]
	rb.buf[rb.tail] = *new(T)
	rb.tail = (rb.tail + 1) % rb.cap
	rb.count--
	rb.cond.Broadcast()
	return item, true
}

// Len returns the current number of items.
func (rb *RingBuffer[T]) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

// Cap returns the capacity.
func (rb *RingBuffer[T]) Cap() int { return rb.cap }

// IsFull reports whether the buffer is at capacity.
func (rb *RingBuffer[T]) IsFull() bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count == rb.cap
}

// IsEmpty reports whether the buffer is empty.
func (rb *RingBuffer[T]) IsEmpty() bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count == 0
}

// Drain removes and returns all items currently in the buffer (non-blocking).
func (rb *RingBuffer[T]) Drain() []T {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	out := make([]T, rb.count)
	for i := range rb.count {
		out[i] = rb.buf[rb.tail]
		rb.buf[rb.tail] = *new(T)
		rb.tail = (rb.tail + 1) % rb.cap
	}
	rb.count = 0
	rb.head = rb.tail // reset write position
	rb.cond.Broadcast()
	return out
}

// Close shuts down the buffer. Pending Push/Pop callers are woken and receive
// ErrClosed (Push) or the remaining items then ErrClosed (Pop). Idempotent.
func (rb *RingBuffer[T]) Close() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.closed = true
	rb.cond.Broadcast() // wake all blocked callers
}
