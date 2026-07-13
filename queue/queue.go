// Package queue provides a generic FIFO queue backed by a slice with a sliding
// head index (amortized O(1) enqueue/dequeue, periodic compaction).
//
// A zero-value Queue is ready to use (empty). Distinct from kit4go's
// [ringbuffer], which is a bounded, channel-based ring for producer/consumer
// handoff; queue is unbounded and synchronous (no blocking, no channels).
//
// Concurrency: not safe for concurrent use — protect with a sync.Mutex or use a
// channel.
//
// Pure standard library. Ad-tech / finance uses: BFS traversal, request
// pipelining, task scheduling, breadth-first graph exploration, batch buffering
// without backpressure.
package queue

// Queue is a first-in-first-out collection of values.
type Queue[T any] struct {
	items []T
	head  int // index of the front element
}

// New builds a Queue pre-seeded with vals (vals[0] at the front).
func New[T any](vals ...T) *Queue[T] {
	q := &Queue[T]{}
	q.items = append(q.items, vals...)
	return q
}

// Enqueue adds v to the back.
func (q *Queue[T]) Enqueue(v T) { q.items = append(q.items, v) }

// Dequeue removes and returns the front element. ok is false if empty.
func (q *Queue[T]) Dequeue() (val T, ok bool) {
	if q.head >= len(q.items) {
		return val, false
	}
	v := q.items[q.head]
	q.items[q.head] = *new(T) // help GC
	q.head++
	// Compact: when the wasted prefix exceeds half the backing slice, slide
	// remaining elements to the front. Amortized O(1) over a sequence of
	// enqueue/dequeue.
	if q.head > len(q.items)/2 {
		n := copy(q.items, q.items[q.head:])
		q.items = q.items[:n]
		q.head = 0
	}
	return v, true
}

// Front returns the front element without removing it. ok is false if empty.
func (q *Queue[T]) Front() (val T, ok bool) {
	if q.head >= len(q.items) {
		return val, false
	}
	return q.items[q.head], true
}

// Back returns the last element without removing it. ok is false if empty.
func (q *Queue[T]) Back() (val T, ok bool) {
	if q.head >= len(q.items) {
		return val, false
	}
	return q.items[len(q.items)-1], true
}

// Len returns the number of elements.
func (q *Queue[T]) Len() int { return len(q.items) - q.head }

// IsEmpty reports whether the queue has no elements.
func (q *Queue[T]) IsEmpty() bool { return q.head >= len(q.items) }

// Clear removes all elements.
func (q *Queue[T]) Clear() {
	clear(q.items)
	q.items = q.items[:0]
	q.head = 0
}

// ToSlice returns a copy of the queue contents from front to back.
func (q *Queue[T]) ToSlice() []T {
	n := q.Len()
	out := make([]T, n)
	copy(out, q.items[q.head:])
	return out
}
