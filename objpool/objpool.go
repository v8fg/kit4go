// Package objpool is a generic object pool: a sync.Pool wrapper with a reset
// hook and live stats. Pure standard library.
//
// Distinct from kit4go's workerpool, which is a goroutine pool (N workers
// draining a job queue). objpool recycles objects across goroutines: Get
// fetches (calling the constructor on a miss), Put returns; an optional reset
// hook re-initializes an object before it is handed back on Get.
//
// Ad-tech uses: pooling *bytes.Buffer for log/JSON rendering, reusing
// protobuf message structs, recycling scratch slices and maps in hot
// serialization paths, and anywhere allocation churn dominates the GC.
package objpool

import (
	"sync"
	"sync/atomic"
)

// Pool is a generic, concurrent-safe object pool wrapping sync.Pool. Get
// returns a pooled object (calling the constructor on a miss) and applies the
// reset hook if one is set; Put returns an object for reuse. Stats snapshots
// are available via Stats.
type Pool[T any] struct {
	pool  sync.Pool
	reset func(T) // optional; nil-safe — applied on Get when set

	// Counters exposed via Stats. inUse is tracked internally as a signed
	// int64 because under concurrency Gets - Puts can briefly go negative;
	// the snapshot clamps to 0 before exposing as uint64.
	gets   uint64
	puts   uint64
	misses uint64
	inUse  int64
}

// Option configures the Pool.
type Option[T any] func(*Pool[T])

// WithReset installs a hook called on every Get, after fetching the object and
// before returning it. Use it to zero fields, truncate buffers, or otherwise
// re-initialize a recycled object so callers always receive a clean value. The
// hook is nil-safe: omitting WithReset skips the call entirely.
func WithReset[T any](fn func(T)) Option[T] {
	return func(p *Pool[T]) {
		p.reset = fn
	}
}

// New builds a Pool. The new function is called on every cache miss (and may
// also be called by the runtime for sync.Pool warm-up). Panics if new is nil —
// a pool without a constructor cannot satisfy Get.
func New[T any](new func() T, opts ...Option[T]) *Pool[T] {
	if new == nil {
		panic("objpool: new func must not be nil")
	}
	p := &Pool[T]{}
	// Wrap new so every constructor invocation (miss or warm-up) is counted as
	// a miss. This is the only reliable signal: sync.Pool.Get does not tell the
	// caller whether the value came from the cache or from pool.New.
	p.pool.New = func() any {
		atomic.AddUint64(&p.misses, 1)
		return new()
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Get returns an object, calling the constructor on a miss. If a reset hook is
// set it is applied to the object before it is returned. Get, Misses, and InUse
// stats are updated atomically.
func (p *Pool[T]) Get() T {
	// sync.Pool stores any; the constructor always returns a non-nil T, so the
	// type assertion never panics in practice. We avoid the comma-ok form to
	// keep the hot path branch-free.
	x := p.pool.Get().(T)
	if p.reset != nil {
		p.reset(x)
	}
	atomic.AddUint64(&p.gets, 1)
	atomic.AddInt64(&p.inUse, 1)
	return x
}

// Put returns an object to the pool for reuse. Puts and InUse stats are updated
// atomically.
func (p *Pool[T]) Put(x T) {
	p.pool.Put(x)
	atomic.AddUint64(&p.puts, 1)
	atomic.AddInt64(&p.inUse, -1)
}

// Stats returns an atomic snapshot of pool counters. InUse = Gets - Puts,
// clamped to 0 (under concurrency the signed internal gauge can dip slightly
// negative before the matching Put lands; the snapshot never reports a negative
// uint64).
func (p *Pool[T]) Stats() Stats {
	inUse := atomic.LoadInt64(&p.inUse)
	if inUse < 0 {
		inUse = 0
	}
	return Stats{
		Gets:   atomic.LoadUint64(&p.gets),
		Puts:   atomic.LoadUint64(&p.puts),
		Misses: atomic.LoadUint64(&p.misses),
		InUse:  uint64(inUse),
	}
}

// Stats is an atomic snapshot of Pool activity.
type Stats struct {
	Gets   uint64 // total Get calls
	Puts   uint64 // total Put calls
	Misses uint64 // times Get fell back to the constructor
	InUse  uint64 // Gets - Puts, clamped to >= 0
}
