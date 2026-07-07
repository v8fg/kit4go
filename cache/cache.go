// Package cache provides a unified, pluggable cache interface (Store[V]) with a
// thread-safe in-memory backend backed by kit4go/lru.
//
// The interface is context-aware so a distributed backend (Redis, via a separate
// module) drops in with the same call sites; the in-memory backend ignores ctx
// (it is synchronous). The in-memory backend stores typed values directly
// (zero-copy); a future Redis backend would JSON-serialize.
//
// Ad-tech uses: hot-lookup caches (creative metadata, bidder config, user
// profile fragments) where a Redis round-trip is too costly. Start in-memory;
// switch to Redis behind the same Store interface when multi-pod needs shared.
package cache

import (
	"context"
	"errors"
	"time"

	"github.com/v8fg/kit4go/lru"
)

// ErrMiss is returned by Get when the key is not in the cache (or has expired).
var ErrMiss = errors.New("cache: key not found")

// Store is the pluggable cache interface. All methods are safe for concurrent
// use. Get for an absent/expired key returns ErrMiss.
type Store[V any] interface {
	Get(ctx context.Context, key string) (V, error)
	Set(ctx context.Context, key string, val V, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Has(ctx context.Context, key string) bool
}

// Compile-time interface assertions: guard that the concrete implementations
// stay in sync with the interface contract.
var _ Store[any] = (*memStore[any])(nil)

// memStore is the in-memory backend wrapping kit4go/lru.
type memStore[V any] struct {
	c          *lru.Cache[string, V]
	defaultTTL time.Duration
}

// MemoryOption configures the in-memory backend.
type MemoryOption[V any] func(*memStore[V])

// WithMaxSize sets the max entries before LRU eviction (default 1024).
func WithMaxSize[V any](n int) MemoryOption[V] {
	return func(m *memStore[V]) { m.c = lru.New[string, V](lru.WithMaxSize[string, V](n)) }
}

// WithDefaultTTL sets a TTL applied to every Set called with ttl=0 (default 0 =
// no expiry). Explicit ttl > 0 on Set always wins.
func WithDefaultTTL[V any](d time.Duration) MemoryOption[V] {
	return func(m *memStore[V]) { m.defaultTTL = d }
}

// NewMemory builds an in-memory Store backed by lru.
func NewMemory[V any](opts ...MemoryOption[V]) Store[V] {
	m := &memStore[V]{c: lru.New[string, V]()}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *memStore[V]) Get(_ context.Context, key string) (V, error) {
	v, ok := m.c.Get(key)
	if !ok {
		var zero V
		return zero, ErrMiss
	}
	return v, nil
}

func (m *memStore[V]) Set(_ context.Context, key string, val V, ttl time.Duration) error {
	if ttl == 0 && m.defaultTTL > 0 {
		ttl = m.defaultTTL
	}
	if ttl > 0 {
		m.c.SetWithTTL(key, val, ttl)
	} else {
		m.c.Set(key, val)
	}
	return nil
}

func (m *memStore[V]) Delete(_ context.Context, key string) error {
	m.c.Delete(key)
	return nil
}

func (m *memStore[V]) Has(_ context.Context, key string) bool {
	return m.c.Contains(key)
}
