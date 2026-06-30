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
	"sync"
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

	// Run the work outside the lock.
	val, err := fn(ctx)

	c.mu.Lock()
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
	c.mu.Unlock()
	close(e.done) // wake all followers
	return val, err
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
