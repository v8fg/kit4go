// Package singleflight deduplicates concurrent in-flight function calls. When
// multiple goroutines call [Group.Do] with the same key while a call is already
// running, only the first executes the function; the rest wait and share its
// result. Pure standard library.
//
// Unlike memoization, results are NOT cached after the call completes — a
// subsequent Do re-runs the function. Singleflight only coalesces calls that
// overlap in time. Use it to suppress duplicate work under burst load (thundering
// herd: N requests for the same key race to compute it — only one runs).
package singleflight

import (
	"fmt"
	"sync"
)

// Result holds the outcome of a Do call.
type Result[V any] struct {
	Value V
	Err   error
	// Shared reports whether this caller received a result computed by another
	// in-flight caller (true) or ran the function itself (false).
	Shared bool
}

// call represents one in-flight function invocation for a key.
type call[V any] struct {
	wg  sync.WaitGroup
	val V
	err error
}

// Group is a deduplication group keyed by K. The zero value is NOT ready to use;
// construct with New.
type Group[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]*call[V]
}

// New creates an empty Group.
func New[K comparable, V any]() *Group[K, V] {
	return &Group[K, V]{m: make(map[K]*call[V])}
}

// Do executes fn once for key, deduplicating concurrent calls. If a call for key
// is already in flight, the caller waits and shares that call's result instead of
// running fn. Results are not cached — once the in-flight call completes, the next
// Do for the same key runs fn again.
//
// Shared is true when this caller received another caller's result.
//
// Panic safety: if fn panics, the panic is recovered, recorded as Result.Err
// (shared with all in-flight waiters), and the in-flight entry is released — a
// panicking fn never deadlocks waiters or starves the key (unlike a naive
// singleflight that skips wg.Done on panic). The panic does NOT propagate.
func (g *Group[K, V]) Do(key K, fn func() (V, error)) (result Result[V]) {
	g.mu.Lock()
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait() // share the in-flight result
		return Result[V]{Value: c.val, Err: c.err, Shared: true}
	}
	c := &call[V]{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	// Defer guarantees waiters are released and the entry is cleaned up on EVERY
	// exit path — including a fn panic, which would otherwise skip wg.Done (dead-
	// locking waiters) and delete (permanently starving the key). The panic is
	// converted to c.err so the leader and all waiters share it as Result.Err.
	defer func() {
		if r := recover(); r != nil {
			c.err = fmt.Errorf("singleflight: fn panicked for key %v: %v", key, r)
		}
		c.wg.Done() // release any waiters
		g.mu.Lock()
		if g.m[key] == c {
			delete(g.m, key) // no caching
		}
		g.mu.Unlock()
		result = Result[V]{Value: c.val, Err: c.err, Shared: false}
	}()
	c.val, c.err = fn() // sole executor
	return
}
