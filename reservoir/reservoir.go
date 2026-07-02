// Package reservoir implements reservoir sampling (Algorithm R): maintain a
// uniform random sample of k items from a stream of unknown length, in O(1)
// per item and O(k) memory. Pure standard library.
//
// Ad-tech uses: sample a representative subset of bid requests / impressions /
// clicks for offline analysis, A/B testing, or quality auditing — without
// buffering the full stream. Each item has an equal probability (k/n) of being
// in the final sample regardless of stream order.
package reservoir

import (
	"math/rand/v2"
	"sync"
)

// Sample holds a reservoir sample of at most k items of type T.
//
// Concurrency: safe for concurrent use. Offer, Sample, Count, and Reset each
// acquire an internal sync.Mutex (serialised). The accept/draw decision uses
// math/rand under the lock; WithSeed makes it deterministic for tests.
type Sample[T any] struct {
	mu    sync.Mutex
	items []T
	k     int
	count int // total items offered
	rng   *rand.Rand
}

// New builds a reservoir with capacity k (must be > 0). Panics otherwise.
func New[T any](k int) *Sample[T] {
	if k <= 0 {
		panic("reservoir: k must be > 0")
	}
	return &Sample[T]{
		items: make([]T, 0, k),
		k:     k,
		rng:   rand.New(rand.NewPCG(0, 0)), // deterministic default; use WithSeed for randomness
	}
}

// Option configures a Sample.
type Option[T any] func(*Sample[T])

// WithSeed sets the RNG seed for reproducible sampling (tests).
func WithSeed[T any](seed1, seed2 uint64) Option[T] {
	return func(s *Sample[T]) { s.rng = rand.New(rand.NewPCG(seed1, seed2)) }
}

// NewWithOpts builds a reservoir with options.
func NewWithOpts[T any](k int, opts ...Option[T]) *Sample[T] {
	s := New[T](k)
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Offer presents an item to the reservoir. The first k items fill the reservoir;
// each subsequent item n (n > k) replaces a random slot with probability k/n.
func (s *Sample[T]) Offer(item T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	if len(s.items) < s.k {
		s.items = append(s.items, item)
		return
	}
	// Algorithm R: replace slot j with probability k/count.
	j := s.rng.IntN(s.count)
	if j < s.k {
		s.items[j] = item
	}
}

// Sample returns a copy of the current reservoir contents. The sample size may
// be smaller than k if fewer than k items have been offered.
func (s *Sample[T]) Sample() []T {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]T, len(s.items))
	copy(out, s.items)
	return out
}

// Count returns the total number of items offered.
func (s *Sample[T]) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

// Cap returns the reservoir capacity k.
func (s *Sample[T]) Cap() int { return s.k }

// Reset clears the reservoir and counter (start a new sample).
func (s *Sample[T]) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = s.items[:0]
	s.count = 0
}
