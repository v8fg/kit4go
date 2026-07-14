// Package memoize provides thread-safe generic memoization of pure functions.
// The first call with a given key computes the result and caches it; subsequent
// calls with the same key return the cached value without recomputing.
//
// Pure standard library.
//
// fn MUST be pure — same input always yields the same output — otherwise the
// cache serves stale results. Memoization trades memory for compute: best suited
// to expensive, referentially-transparent functions (parses, derivations,
// lookups over immutable data).
package memoize

import "sync"

// Memoize returns a memoized version of fn. Thread-safe via sync.Map.
//
// Under contention, two goroutines hitting the same uncached key may both
// compute fn: the result is identical for a pure fn, so this is safe but
// wasteful. For expensive functions where single-computation matters, wrap fn
// with a per-key lock (e.g. golang.org/x/sync/singleflight) before memoizing.
func Memoize[K comparable, V any](fn func(K) V) func(K) V {
	var cache sync.Map
	return func(k K) V {
		if v, ok := cache.Load(k); ok {
			return v.(V)
		}
		v := fn(k)
		cache.Store(k, v)
		return v
	}
}

// MemoizeErr is like Memoize but for functions that return an error. Only
// successful results are cached; an error is returned to the caller and NOT
// cached, so a transient failure retries on the next call rather than being
// remembered as a permanent result.
func MemoizeErr[K comparable, V any](fn func(K) (V, error)) func(K) (V, error) {
	var cache sync.Map
	return func(k K) (V, error) {
		if v, ok := cache.Load(k); ok {
			return v.(V), nil
		}
		val, err := fn(k)
		if err == nil {
			cache.Store(k, val)
		}
		return val, err
	}
}
