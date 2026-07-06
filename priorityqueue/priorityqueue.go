// Package priorityqueue is a generic max-heap priority queue backed by
// container/heap. Items with the highest Priority value dequeue first.
// Pure standard library.
//
// The queue is NOT safe for concurrent use. Callers that share a Queue across
// goroutines must wrap every method in an external sync.Mutex (or use a
// dedicated owner goroutine). This mirrors Go's own container/heap, which is
// also lock-free by design — keeping the heap a plain data structure lets the
// caller pick the right synchronisation strategy (mutex, channel hand-off, or
// single-owner) instead of paying for locking it may not need.
//
// Update lets a queued item's priority be changed in place and the heap
// re-heapified in O(log n), which is required for Dijkstra / A* / scheduler
// use-cases where priorities change after enqueue.
//
// Ad-tech uses: bid prioritisation in auction ranking, rate-limiter fairness
// queues, weighted round-robin selectors, and any scenario where the next item
// to process is the highest-weighted rather than the oldest.
package priorityqueue

import "container/heap"

// Item is a single entry in the queue. The index field is managed internally by
// the heap implementation and must not be set or read by callers.
type Item[T any] struct {
	Value    T
	Priority int
	index    int // heap-managed; kept in sync by Swap
}

// Queue is a generic max-heap priority queue. The zero value is NOT ready to
// use; construct one with New.
//
// The exported Push/Pop/Peek/Update methods are the public API. Internally a
// separate itemHeap type implements heap.Interface (Push/Pop with any-typed
// signatures) — Go does not allow a single method name to carry two different
// signatures on one type, so the heap.Interface plumbing lives on itemHeap
// while Queue exposes ergonomic value/priority methods.
type Queue[T any] struct {
	h itemHeap[T]
}

// itemHeap implements heap.Interface over a slice of *Item[T]. It is the
// container/heap plumbing; callers use Queue's higher-level methods instead.
type itemHeap[T any] []*Item[T]

func (h itemHeap[T]) Len() int { return len(h) }

// Less is a max-heap: higher Priority ranks first.
func (h itemHeap[T]) Less(i, j int) bool { return h[i].Priority > h[j].Priority }

func (h itemHeap[T]) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *itemHeap[T]) Push(x any) {
	item := x.(*Item[T])
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *itemHeap[T]) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // help GC
	item.index = -1
	*h = old[:n-1]
	return item
}

// New returns an empty priority queue.
func New[T any]() *Queue[T] {
	return &Queue[T]{}
}

// Len returns the number of items in the queue.
func (q *Queue[T]) Len() int { return q.h.Len() }

// Push enqueues value with the given priority and returns the resulting Item
// (which may be passed to Update later to change its priority). Highest
// priority dequeues first.
func (q *Queue[T]) Push(value T, priority int) *Item[T] {
	item := &Item[T]{Value: value, Priority: priority}
	heap.Push(&q.h, item)
	return item
}

// Pop removes and returns the highest-priority value, its priority, and true.
// If the queue is empty it returns the zero value of T, 0, and false.
func (q *Queue[T]) Pop() (T, int, bool) {
	if q.h.Len() == 0 {
		var zero T
		return zero, 0, false
	}
	item := heap.Pop(&q.h).(*Item[T])
	return item.Value, item.Priority, true
}

// Peek returns the highest-priority value, its priority, and true without
// removing it. If the queue is empty it returns the zero value of T, 0, and
// false.
func (q *Queue[T]) Peek() (T, int, bool) {
	if q.h.Len() == 0 {
		var zero T
		return zero, 0, false
	}
	top := q.h[0]
	return top.Value, top.Priority, true
}

// Update changes a queued item's priority to the given value and re-heapifies
// so the queue invariant is restored in O(log n). The item must be one returned
// by Push and must still be in the queue; passing an item from another queue or
// one already popped has undefined behaviour.
func (q *Queue[T]) Update(item *Item[T], priority int) {
	if item == nil || item.index < 0 || item.index >= q.h.Len() {
		return // nil, foreign, or already-popped item: no-op (out of contract)
	}
	item.Priority = priority
	heap.Fix(&q.h, item.index)
}
