// Package ringbuf provides a generic circular buffer (ring buffer) with a fixed
// capacity that OVERWRITES the oldest element when full — distinct from
// [ringbuffer] which blocks on full (producer/consumer backpressure).
//
// Use ringbuf when you need the last N values: sliding-window metrics, rolling
// logs, audio/streaming sample buffers, "recent items" displays. O(1) Push and
// O(1) At. Not safe for concurrent use (protect with a mutex).
//
// Pure standard library.
package ringbuf

import "errors"

// ErrEmpty is returned by Pop/At when the buffer is empty.
var ErrEmpty = errors.New("ringbuf: empty")

// CircularBuffer is a fixed-capacity ring buffer that overwrites the oldest
// element when full.
type CircularBuffer[T any] struct {
	buf  []T
	head int // index of the oldest element
	size int // number of valid elements (0..cap)
	cap  int
}

// New creates a CircularBuffer with the given capacity. Panics if cap <= 0.
func New[T any](capacity int) *CircularBuffer[T] {
	if capacity <= 0 {
		panic("ringbuf: capacity must be > 0")
	}
	return &CircularBuffer[T]{buf: make([]T, capacity), cap: capacity}
}

// Push appends v, overwriting the oldest element if the buffer is full.
func (c *CircularBuffer[T]) Push(v T) {
	if c.size < c.cap {
		idx := (c.head + c.size) % c.cap
		c.buf[idx] = v
		c.size++
	} else {
		c.buf[c.head] = v
		c.head = (c.head + 1) % c.cap
	}
}

// Pop removes and returns the oldest element. Returns ErrEmpty if empty.
func (c *CircularBuffer[T]) Pop() (T, error) {
	var zero T
	if c.size == 0 {
		return zero, ErrEmpty
	}
	v := c.buf[c.head]
	c.buf[c.head] = zero // help GC
	c.head = (c.head + 1) % c.cap
	c.size--
	return v, nil
}

// At returns the i-th element from oldest (0) to newest (size-1).
// Returns ErrEmpty if i is out of range.
func (c *CircularBuffer[T]) At(i int) (T, error) {
	var zero T
	if i < 0 || i >= c.size {
		return zero, ErrEmpty
	}
	return c.buf[(c.head+i)%c.cap], nil
}

// Latest returns the most recently pushed element. Returns ErrEmpty if empty.
func (c *CircularBuffer[T]) Latest() (T, error) {
	return c.At(c.size - 1)
}

// Len returns the number of valid elements.
func (c *CircularBuffer[T]) Len() int { return c.size }

// Cap returns the capacity.
func (c *CircularBuffer[T]) Cap() int { return c.cap }

// IsFull reports whether the buffer is at capacity.
func (c *CircularBuffer[T]) IsFull() bool { return c.size == c.cap }

// IsEmpty reports whether the buffer has no elements.
func (c *CircularBuffer[T]) IsEmpty() bool { return c.size == 0 }

// Clear removes all elements.
func (c *CircularBuffer[T]) Clear() {
	clear(c.buf)
	c.head = 0
	c.size = 0
}

// ToSlice returns all valid elements from oldest to newest.
func (c *CircularBuffer[T]) ToSlice() []T {
	out := make([]T, c.size)
	for i := range c.size {
		out[i] = c.buf[(c.head+i)%c.cap]
	}
	return out
}
