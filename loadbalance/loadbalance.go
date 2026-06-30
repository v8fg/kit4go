// Package loadbalance picks an item from a weighted set under a selectable
// strategy — smooth weighted round-robin (the nginx algorithm), plain
// round-robin, uniform random, or weighted random.
//
// It is generic over the item type and safe for concurrent use. Ad-tech uses:
// distributing requests across upstream SSP endpoints or bidder instances in
// proportion to their capacity, with smooth interleaving (no traffic bursts to
// a single upstream).
package loadbalance

import (
	"fmt"
	"math/rand/v2"
	"sync"
)

// Entry pairs a value with its weight. A weight <= 0 is treated as 1.
type Entry[T any] struct {
	Value  T
	Weight int
}

// Strategy selects how Next chooses among entries.
type Strategy int

const (
	// StrategySmoothWeightedRR is the nginx smooth weighted round-robin: each
	// pick adds every entry's weight to a running counter, selects the largest,
	// and subtracts the total weight from it. The result is a smooth, burst-free
	// interleaving proportional to weight. This is the default.
	StrategySmoothWeightedRR Strategy = iota
	// StrategyRoundRobin cycles entries in order, ignoring weights.
	StrategyRoundRobin
	// StrategyRandom picks an entry uniformly at random.
	StrategyRandom
	// StrategyWeightedRandom picks an entry with probability proportional to
	// its weight.
	StrategyWeightedRandom
)

type entry[T any] struct {
	value         T
	weight        int
	currentWeight int // SWRR mutable state
}

// Balancer selects items from a weighted set. Construct with New.
type Balancer[T any] struct {
	mu          sync.Mutex
	id          func(T) string
	entries     []*entry[T]
	totalWeight int
	rrIdx       int
	strategy    Strategy
}

// Option configures a Balancer.
type Option[T any] func(*Balancer[T])

// WithStrategy sets the selection strategy (default StrategySmoothWeightedRR).
func WithStrategy[T any](s Strategy) Option[T] { return func(b *Balancer[T]) { b.strategy = s } }

// New builds a balancer over the given entries. id must return a stable, unique
// string per value; it is used to dedup Add/Remove. Pass nil to use fmt's
// default formatting (fine for comparable/strings, less so for structs/pointers
// that alias).
func New[T any](id func(T) string, entries []Entry[T], opts ...Option[T]) *Balancer[T] {
	b := &Balancer[T]{id: id, strategy: StrategySmoothWeightedRR}
	if b.id == nil {
		b.id = func(v T) string { return fmt.Sprintf("%v", v) }
	}
	for _, e := range entries {
		b.addLocked(e)
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Add inserts an entry; an entry with the same id is replaced (its weight is
// updated and SWRR state reset for that entry).
func (b *Balancer[T]) Add(e Entry[T]) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.addLocked(e)
}

func (b *Balancer[T]) addLocked(e Entry[T]) {
	w := e.Weight
	if w <= 0 {
		w = 1
	}
	// Replace existing entry with the same id, resetting SWRR state.
	for _, ex := range b.entries {
		if b.id(ex.value) == b.id(e.Value) {
			b.totalWeight -= ex.weight
			ex.weight = w
			ex.currentWeight = 0
			b.totalWeight += w
			return
		}
	}
	b.entries = append(b.entries, &entry[T]{value: e.Value, weight: w})
	b.totalWeight += w
}

// Remove drops the entry whose id matches value. No-op if absent.
func (b *Balancer[T]) Remove(value T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	target := b.id(value)
	out := b.entries[:0]
	for _, ex := range b.entries {
		if b.id(ex.value) == target {
			b.totalWeight -= ex.weight
			continue
		}
		out = append(out, ex)
	}
	b.entries = out
}

// Len returns the entry count.
func (b *Balancer[T]) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.entries)
}

// Next returns the selected value, or ok=false when the balancer is empty.
func (b *Balancer[T]) Next() (T, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var zero T
	if len(b.entries) == 0 {
		return zero, false
	}
	switch b.strategy {
	case StrategyRoundRobin:
		return b.nextRoundRobin()
	case StrategyRandom:
		return b.nextRandom()
	case StrategyWeightedRandom:
		return b.nextWeightedRandom()
	default: // StrategySmoothWeightedRR
		return b.nextSWRR()
	}
}

func (b *Balancer[T]) nextRoundRobin() (T, bool) {
	if b.rrIdx >= len(b.entries) {
		b.rrIdx = 0
	}
	v := b.entries[b.rrIdx].value
	b.rrIdx++
	return v, true
}

func (b *Balancer[T]) nextRandom() (T, bool) {
	return b.entries[rand.IntN(len(b.entries))].value, true
}

func (b *Balancer[T]) nextWeightedRandom() (T, bool) {
	r := rand.IntN(b.totalWeight)
	cum := 0
	for _, e := range b.entries {
		cum += e.weight
		if r < cum {
			return e.value, true
		}
	}
	return b.entries[len(b.entries)-1].value, true
}

// nextSWRR is the nginx smooth weighted round-robin.
func (b *Balancer[T]) nextSWRR() (T, bool) {
	var best *entry[T]
	for _, e := range b.entries {
		e.currentWeight += e.weight
		if best == nil || e.currentWeight > best.currentWeight {
			best = e
		}
	}
	best.currentWeight -= b.totalWeight
	return best.value, true
}

// All returns a copy of the current entries (value + weight), unspecified order.
func (b *Balancer[T]) All() []Entry[T] {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Entry[T], len(b.entries))
	for i, e := range b.entries {
		out[i] = Entry[T]{Value: e.value, Weight: e.weight}
	}
	return out
}
