// Package stack provides a generic LIFO stack backed by a slice.
//
// A zero-value Stack is ready to use (empty). Push/Pop/Peek are O(1) amortized.
//
// Concurrency: not safe for concurrent use — protect with a sync.Mutex or use a
// channel.
//
// Pure standard library. Ad-tech / finance uses: expression evaluation, undo
// history, depth-first traversal, bracket matching, call-stack simulation.
package stack

// Stack is a last-in-first-out collection of values.
type Stack[T any] struct {
	items []T
}

// New builds a Stack pre-seeded with vals (vals[0] at the bottom, vals[-1] on
// top).
func New[T any](vals ...T) *Stack[T] {
	s := &Stack[T]{}
	s.items = append(s.items, vals...)
	return s
}

// Push adds v to the top.
func (s *Stack[T]) Push(v T) { s.items = append(s.items, v) }

// Pop removes and returns the top element. ok is false if empty.
func (s *Stack[T]) Pop() (val T, ok bool) {
	n := len(s.items)
	if n == 0 {
		return val, false
	}
	v := s.items[n-1]
	s.items[n-1] = *new(T) // help GC
	s.items = s.items[:n-1]
	return v, true
}

// Peek returns the top element without removing it. ok is false if empty.
func (s *Stack[T]) Peek() (val T, ok bool) {
	n := len(s.items)
	if n == 0 {
		return val, false
	}
	return s.items[n-1], true
}

// Len returns the number of elements.
func (s *Stack[T]) Len() int { return len(s.items) }

// IsEmpty reports whether the stack has no elements.
func (s *Stack[T]) IsEmpty() bool { return len(s.items) == 0 }

// Clear removes all elements.
func (s *Stack[T]) Clear() { clear(s.items); s.items = s.items[:0] }

// ToSlice returns a copy of the stack contents from bottom to top.
func (s *Stack[T]) ToSlice() []T {
	out := make([]T, len(s.items))
	copy(out, s.items)
	return out
}

// WithCapacity builds an empty Stack with a pre-allocated backing slice of the
// given capacity — avoids slice growth/realloc during a known-size push sequence.
func WithCapacity[T any](cap int) *Stack[T] {
	return &Stack[T]{items: make([]T, 0, cap)}
}
