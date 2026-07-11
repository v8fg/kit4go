// Package backpressure provides a non-blocking load-shed gate: when in-flight
// work exceeds a threshold, new work is rejected immediately (TryAcquire returns
// false) rather than queueing. Distinct from semaphore (which blocks): this
// signals "stop sending" instead of "wait."
//
// Ad-tech uses: bid server self-protection (reject new bid requests when the
// pipeline is full, rather than queueing and timing out late), API gateway load
// shedding, worker admission control.
package backpressure

import "sync/atomic"

// Gate is a non-blocking admission gate. Track in-flight work; reject when full.
type Gate struct {
	max      atomic.Int32 // atomic: SetMax (hot-reload) races with TryAcquire/IsOverloaded/Max readers
	current  atomic.Int32
	rejected atomic.Uint64
}

// New builds a Gate that allows up to maxConcurrent in-flight items.
func New(maxConcurrent int32) *Gate {
	if maxConcurrent < 0 {
		maxConcurrent = 0
	}
	g := &Gate{}
	g.max.Store(maxConcurrent)
	return g
}

// TryAcquire attempts to register one in-flight item. Returns true if accepted,
// false if the gate is full (the caller should shed/reject the work). Each
// successful TryAcquire MUST be paired with exactly one Release.
func (g *Gate) TryAcquire() bool {
	for {
		cur := g.current.Load()
		if cur >= g.max.Load() {
			g.rejected.Add(1)
			return false
		}
		if g.current.CompareAndSwap(cur, cur+1) {
			return true
		}
	}
}

// Release marks one in-flight item as done. Returns false (a no-op) if called
// when nothing is in flight — safe for concurrent code where a Release might
// race with the item completing. Returns true on a successful decrement.
func (g *Gate) Release() bool {
	for {
		cur := g.current.Load()
		if cur <= 0 {
			return false
		}
		if g.current.CompareAndSwap(cur, cur-1) {
			return true
		}
	}
}

// Current returns the number of in-flight items.
func (g *Gate) Current() int32 { return g.current.Load() }

// Rejected returns the total count of rejected (shed) attempts (L5 observable).
func (g *Gate) Rejected() uint64 { return g.rejected.Load() }

// IsOverloaded reports whether the gate is at capacity.
func (g *Gate) IsOverloaded() bool { return g.current.Load() >= g.max.Load() }

// SetMax adjusts the capacity at runtime (hot-reload). Does not reject already-
// in-flight items even if the new max is lower than current.
func (g *Gate) SetMax(max int32) {
	if max < 0 {
		max = 0
	}
	g.max.Store(max)
}

// Max returns the configured capacity.
func (g *Gate) Max() int32 { return g.max.Load() }
