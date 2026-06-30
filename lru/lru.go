// Package lru is a fixed-size, generically typed LRU cache with optional
// per-cache TTL and eviction callbacks.
//
// It is safe for concurrent use. Get promotes the entry (it is a real access);
// Peek reads without promoting. Expiry is lazy — checked on access and swept
// on DeleteExpired — which avoids a background goroutine and keeps the cache
// self-contained. Ad-tech uses: in-process lookups for creatives, bidder
// config, or frequency-cap counters where a Redis round-trip is too costly.
package lru

import (
	"container/list"
	"sync"
	"time"
)

// entry holds a key/value pair and its absolute expiry (zero = no expiry).
type entry[K comparable, V any] struct {
	key       K
	value     V
	expiresAt time.Time // zero value means "no expiry"
}

func (e *entry[K, V]) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && now.After(e.expiresAt)
}

// Cache is a fixed-size LRU cache. The zero value is NOT usable; construct with
// New. A maxSize of 0 disables eviction (unbounded) — use that deliberately,
// e.g. for a small, known key set.
type Cache[K comparable, V any] struct {
	mu        sync.RWMutex
	ll        *list.List
	items     map[K]*list.Element
	maxSize   int
	ttl       time.Duration
	onEvicted func(key K, value V)
	clock     func() time.Time
}

// Option configures a Cache.
type Option[K comparable, V any] func(*Cache[K, V])

// WithMaxSize sets the maximum number of entries (default 1024). A value <= 0
// disables eviction.
func WithMaxSize[K comparable, V any](n int) Option[K, V] {
	return func(c *Cache[K, V]) { c.maxSize = n }
}

// WithTTL sets a default time-to-live applied to every Set (entries expire after
// this duration from insertion/refresh). Zero (the default) means no expiry.
func WithTTL[K comparable, V any](d time.Duration) Option[K, V] {
	return func(c *Cache[K, V]) { c.ttl = d }
}

// WithOnEvicted registers a callback invoked once for every entry that leaves
// the cache (eviction, explicit Delete, expiry, Purge, or Resize). It runs
// under the cache lock — keep it cheap (no re-entrant cache calls).
func WithOnEvicted[K comparable, V any](fn func(key K, value V)) Option[K, V] {
	return func(c *Cache[K, V]) { c.onEvicted = fn }
}

// New constructs an LRU cache. The default max size is 1024 and the default TTL
// is "no expiry".
func New[K comparable, V any](opts ...Option[K, V]) *Cache[K, V] {
	c := &Cache[K, V]{
		ll:      list.New(),
		items:   make(map[K]*list.Element),
		maxSize: 1024,
		clock:   time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Set inserts or refreshes key=value, applying the cache's default TTL. It
// promotes an existing entry to most-recently-used.
func (c *Cache[K, V]) Set(key K, value V) {
	c.set(key, value, c.ttl)
}

// SetWithTTL inserts or refreshes key=value with a per-entry TTL overriding the
// cache default. A ttl of 0 means "no expiry" for this entry.
func (c *Cache[K, V]) SetWithTTL(key K, value V, ttl time.Duration) {
	c.set(key, value, ttl)
}

func (c *Cache[K, V]) set(key K, value V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var exp time.Time
	if ttl > 0 {
		exp = c.clock().Add(ttl)
	}
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		e := el.Value.(*entry[K, V])
		e.value = value
		e.expiresAt = exp
		return
	}
	el := c.ll.PushFront(&entry[K, V]{key: key, value: value, expiresAt: exp})
	c.items[key] = el
	if c.maxSize > 0 && c.ll.Len() > c.maxSize {
		c.evict()
	}
}

// evict removes the least-recently-used entry. Caller must hold the write lock.
func (c *Cache[K, V]) evict() {
	el := c.ll.Back()
	if el == nil {
		return
	}
	c.removeElement(el)
}

// Get returns the value for key and reports whether it was present and not
// expired. A hit promotes the entry to most-recently-used; an expired entry is
// evicted and reported as a miss.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	e := el.Value.(*entry[K, V])
	if e.expired(c.clock()) {
		c.removeElement(el)
		var zero V
		return zero, false
	}
	c.ll.MoveToFront(el)
	return e.value, true
}

// Peek returns the value for key without promoting it. An expired entry is
// reported as a miss (and evicted).
func (c *Cache[K, V]) Peek(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	e := el.Value.(*entry[K, V])
	if e.expired(c.clock()) {
		c.removeElement(el)
		var zero V
		return zero, false
	}
	return e.value, true
}

// Contains reports whether key is present and not expired, without promotion.
func (c *Cache[K, V]) Contains(key K) bool {
	_, ok := c.Peek(key)
	return ok
}

// Delete removes key and reports whether it was present. The onEvicted callback
// (if any) fires for the removed entry.
func (c *Cache[K, V]) Delete(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return false
	}
	c.removeElement(el)
	return true
}

// DeleteExpired removes all expired entries and returns the count removed.
func (c *Cache[K, V]) DeleteExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	removed := 0
	for _, el := range c.items {
		if el.Value.(*entry[K, V]).expired(now) {
			c.removeElement(el)
			removed++
		}
	}
	return removed
}

// Len returns the number of entries (including any not-yet-swept expired ones;
// call DeleteExpired first for an exact live count).
func (c *Cache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ll.Len()
}

// Keys returns the keys in most- to least-recently-used order.
func (c *Cache[K, V]) Keys() []K {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]K, 0, c.ll.Len())
	for el := c.ll.Front(); el != nil; el = el.Next() {
		keys = append(keys, el.Value.(*entry[K, V]).key)
	}
	return keys
}

// Purge empties the cache, firing onEvicted for every entry.
func (c *Cache[K, V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, el := range c.items {
		c.removeElement(el)
	}
}

// Resize changes the max size and immediately evicts down to it, returning the
// number of entries evicted. A new size <= 0 disables eviction.
func (c *Cache[K, V]) Resize(n int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxSize = n
	if n <= 0 {
		return 0
	}
	evicted := 0
	for c.maxSize > 0 && c.ll.Len() > c.maxSize {
		c.evict()
		evicted++
	}
	return evicted
}

// removeElement removes a list element and its map entry, firing onEvicted.
// Caller must hold the write lock.
func (c *Cache[K, V]) removeElement(el *list.Element) {
	e := c.ll.Remove(el).(*entry[K, V])
	delete(c.items, e.key)
	if c.onEvicted != nil {
		c.onEvicted(e.key, e.value)
	}
}
