// Package idempotency provides an in-process idempotency cache: concurrent or
// near-term repeat calls with the same key are coalesced into a single execution
// of the underlying function, and the successful result is served from cache
// until it expires.
//
// Two guarantees combine: (1) singleflight — while one call is in flight, later
// callers for the same key wait for the leader's result instead of re-running
// the work; (2) result cache — after the leader succeeds, the result is returned
// to all callers within the TTL, so a retried request yields the original
// outcome (Stripe-style Idempotency-Key semantics, in-process).
//
// By default a failed call is NOT cached, so the next caller retries; set
// WithCacheErrors to also persist failures (turning it into a hard de-dup).
// Pure standard library. Ad-tech / finance / push / chain uses: coalescing
// concurrent bid requests for the same auction, payment/charge dedup, webhook
// delivery dedup, transaction dedup.
package idempotency

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// entry holds an in-flight or completed computation for one key. The leader
// runs fn and closes done; followers read val/err after done closes.
type entry[V any] struct {
	done      chan struct{}
	val       V
	err       error
	completed bool
	expiresAt time.Time
}

func (e *entry[V]) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && now.After(e.expiresAt)
}

// Cache coalesces calls by key and caches successful results for a TTL. The
// zero value is NOT usable; construct with New.
type Cache[V any] struct {
	mu          sync.Mutex
	entries     map[string]*entry[V]
	ttl         time.Duration
	maxEntries  int
	cacheErrors bool
	clock       func() time.Time

	recovered uint64 // count of leader fn panics recovered (observable; L5)
	// onPanic is an optional hook fired on a recovered leader panic. Stored as
	// an atomic.Pointer so SetOnPanic (caller goroutine) and the recover path
	// (leader goroutine) never race on the bare field; a nil hook costs only a
	// Load on the no-hook fast path. Mirrors workerpool/pipeline/signalbus.
	onPanic atomic.Pointer[func(any)]
}

// Option configures a Cache.
type Option[V any] func(*Cache[V])

// WithTTL sets how long a successful result is cached (default 1m). 0 = no
// expiry (cached until evicted).
func WithTTL[V any](d time.Duration) Option[V] { return func(c *Cache[V]) { c.ttl = d } }

// WithMaxEntries caps the number of cached results (default 4096). When full, a
// new entry evicts the oldest expired entry (or, if none, the oldest by
// expiry/insertion). 0 = unbounded.
func WithMaxEntries[V any](n int) Option[V] { return func(c *Cache[V]) { c.maxEntries = n } }

// WithCacheErrors also caches failed results (so a failed key is NOT retried).
// Default off: failures are removable so the next caller retries.
func WithCacheErrors[V any](on bool) Option[V] { return func(c *Cache[V]) { c.cacheErrors = on } }

// New builds an idempotency cache.
func New[V any](opts ...Option[V]) *Cache[V] {
	c := &Cache[V]{
		entries:    make(map[string]*entry[V]),
		ttl:        time.Minute,
		maxEntries: 4096,
		clock:      time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetOnPanic installs a hook fired (non-blocking) when the leader fn panics.
// The panic is also counted in Recovered and delivered as the result error (to
// the leader and all followers), and the entry is treated as a normal failure
// (dropped unless WithCacheErrors is on). Safe to call concurrently with Do
// (the hook pointer is stored atomically, so there is no data race between a
// writer here and the reader in the recover path). Pass nil to clear a
// previously-installed hook.
func (c *Cache[V]) SetOnPanic(fn func(any)) {
	if fn == nil {
		c.onPanic.Store(nil)
		return
	}
	f := fn // copy to heap
	c.onPanic.Store(&f)
}

// Recovered returns the total number of leader fn panics recovered since the
// cache was created.
func (c *Cache[V]) Recovered() uint64 { return atomic.LoadUint64(&c.recovered) }

// Do runs fn at most once for concurrent/repeat callers of key (within the TTL
// for cached successes). Followers wait for the leader; if their ctx is
// cancelled while waiting they return ctx.Err() immediately (the leader still
// completes). Returns the leader's (value, error).
func (c *Cache[V]) Do(ctx context.Context, key string, fn func(ctx context.Context) (V, error)) (V, error) {
	now := c.clock()
	c.mu.Lock()
	if e, ok := c.entries[key]; ok {
		if e.completed && !e.expired(now) {
			c.mu.Unlock()
			return e.val, e.err // cached success (or cached error, if cacheErrors)
		}
		if !e.completed {
			// In flight: become a follower.
			c.mu.Unlock()
			return c.waitFor(ctx, e)
		}
		// Completed-but-expired: fall through to become the new leader.
	}
	// Become the leader.
	e := &entry[V]{done: make(chan struct{})}
	c.entries[key] = e
	c.evictLocked(now)
	c.mu.Unlock()

	// Run the work outside the lock. The leader is async relative to its
	// followers (they wait on e.done), so a panicking fn must be recovered —
	// matching the kit callback-recover convention (workerpool/pipeline/
	// signalbus/shutdown/debounce). Without recovery the panic escapes Do
	// BEFORE close(e.done): followers hang forever, the entry leaks, and a
	// fresh Do(key) permanently stalls on a done that never closes. The panic
	// is converted to an error and routed through finishLocked so close(e.done)
	// is ALWAYS reached.
	var val V
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				atomic.AddUint64(&c.recovered, 1)
				if hp := c.onPanic.Load(); hp != nil {
					(*hp)(r)
				}
				err = fmt.Errorf("idempotency: fn panic recovered: %v", r)
			}
		}()
		val, err = fn(ctx)
	}()

	c.mu.Lock()
	c.finishLocked(e, key, val, err)
	c.mu.Unlock()
	return val, err
}

// finishLocked records the leader's result, decides whether to cache or drop
// the entry, and closes e.done to wake all followers. Used by BOTH the normal
// and panic-recovery paths so close(e.done) is ALWAYS reached. Caller must hold
// c.mu.
func (c *Cache[V]) finishLocked(e *entry[V], key string, val V, err error) {
	e.val = val
	e.err = err
	e.completed = true
	keep := err == nil || c.cacheErrors
	if keep && c.ttl > 0 {
		e.expiresAt = c.clock().Add(c.ttl)
	}
	if !keep {
		delete(c.entries, key) // failures (when not caching) are removable -> retry
	}
	close(e.done) // wake all followers
}

// waitFor blocks until the leader completes (e.done closes) or ctx is cancelled.
func (c *Cache[V]) waitFor(ctx context.Context, e *entry[V]) (V, error) {
	select {
	case <-e.done:
		return e.val, e.err
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	}
}

// Len returns the number of tracked keys (in-flight + cached).
func (c *Cache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Forget drops key from the cache so the next Do re-runs fn.
func (c *Cache[V]) Forget(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Clear drops all keys.
func (c *Cache[V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*entry[V])
}

// evictLocked trims the cache to maxEntries, dropping expired entries first,
// then the oldest by expiry. Caller holds the lock.
func (c *Cache[V]) evictLocked(now time.Time) {
	if c.maxEntries <= 0 || len(c.entries) <= c.maxEntries {
		return
	}
	// First pass: drop expired, completed entries (not in-flight leaders).
	for k, e := range c.entries {
		if e.completed && e.expired(now) {
			delete(c.entries, k)
		}
	}
	// Still over? drop oldest-completed by expiry.
	for len(c.entries) > c.maxEntries {
		var oldestK string
		var oldestT time.Time
		found := false
		for k, e := range c.entries {
			if !e.completed {
				continue // never evict an in-flight leader
			}
			if !found || e.expiresAt.Before(oldestT) {
				oldestK, oldestT, found = k, e.expiresAt, true
			}
		}
		if !found {
			return // nothing completed left to evict
		}
		delete(c.entries, oldestK)
	}
}
