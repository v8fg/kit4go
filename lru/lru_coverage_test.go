package lru

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestEvict_BackNil covers the el == nil branch of evict. evict reads
// ll.Back(); when the list is empty, Back() is nil and evict must return without
// panicking.
//
// This branch is unreachable through the public API: every public caller of
// evict (set at lru.go:125, Resize at lru.go:273) guards with a
// "Len() > maxSize" condition that guarantees the list is non-empty before
// evict runs, so ll.Back() is never nil in production. It is defensive against
// a future caller that forgets the guard. Because the test is white-box
// (package lru), we invoke evict directly on an empty cache to exercise the
// nil-Back() early return.
func TestEvict_BackNil(t *testing.T) {
	c := New[string, int](WithMaxSize[string, int](4))
	// Empty cache: ll.Back() is nil, so evict must take the early-return path
	// and neither panic nor mutate state.
	var evicted []evictedKV[string, int]
	c.evict(&evicted)
	require.Empty(t, evicted)
	require.Equal(t, 0, c.Len())
}

// TestPeek_ExpiredEntryCovers covers the expired-entry branch of Peek (the entry
// is present but expired -> evicted and reported as a miss).
func TestPeek_ExpiredEntry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	c := New[string, int](
		WithTTL[string, int](5*time.Second),
		withClock[string, int](clk.now),
	)
	c.Set("k", 7)
	clk.t = clk.t.Add(10 * time.Second) // expire it
	v, ok := c.Peek("k")
	require.False(t, ok)
	require.Equal(t, 0, v)
	require.False(t, c.Contains("k"))
	require.Equal(t, 0, c.Len())
}

// TestPeek_ExpiredEntryFiresCallback covers the removeElement+fireEvicted path
// of Peek when an entry expires.
func TestPeek_ExpiredEntryFiresCallback(t *testing.T) {
	clk := &fakeClock{t: time.Unix(2000, 0)}
	var evicted []string
	c := New[string, int](
		WithTTL[string, int](5*time.Second),
		WithOnEvicted[string, int](func(k string, v int) { evicted = append(evicted, k) }),
		withClock[string, int](clk.now),
	)
	c.Set("a", 1)
	clk.t = clk.t.Add(6 * time.Second)
	_, ok := c.Peek("a")
	require.False(t, ok)
	require.Equal(t, []string{"a"}, evicted)
}

// TestContains_ExpiredEntry covers the expired-entry path of Contains via Peek.
func TestContains_ExpiredEntry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(3000, 0)}
	c := New[string, int](
		WithTTL[string, int](2*time.Second),
		withClock[string, int](clk.now),
	)
	c.Set("a", 1)
	clk.t = clk.t.Add(3 * time.Second)
	require.False(t, c.Contains("a"))
}

// TestDeleteExpired_NoExpired covers DeleteExpired when no entries are expired
// (the loop runs but nothing is removed).
func TestDeleteExpired_NoExpired(t *testing.T) {
	clk := &fakeClock{t: time.Unix(4000, 0)}
	c := New[string, int](withClock[string, int](clk.now))
	c.Set("a", 1)
	c.Set("b", 2)
	removed := c.DeleteExpired()
	require.Equal(t, 0, removed)
	require.Equal(t, 2, c.Len())
}

// TestGet_ExpiredFiresCallback covers the expired-entry branch of Get when
// onEvicted is registered.
func TestGet_ExpiredFiresCallback(t *testing.T) {
	clk := &fakeClock{t: time.Unix(5000, 0)}
	var evicted []string
	c := New[string, int](
		WithTTL[string, int](2*time.Second),
		WithOnEvicted[string, int](func(k string, v int) { evicted = append(evicted, k) }),
		withClock[string, int](clk.now),
	)
	c.Set("a", 1)
	clk.t = clk.t.Add(3 * time.Second)
	_, ok := c.Get("a")
	require.False(t, ok)
	require.Equal(t, []string{"a"}, evicted)
}

// TestResize_ToZero covers the n <= 0 branch of Resize (disables eviction).
func TestResize_ToZero(t *testing.T) {
	c := New[string, int](WithMaxSize[string, int](2))
	c.Set("a", 1)
	c.Set("b", 2)
	evicted := c.Resize(0)
	require.Equal(t, 0, evicted)
	c.Set("c", 3)
	c.Set("d", 4)
	require.Equal(t, 4, c.Len()) // unbounded now
}

// TestPurge_Empty covers Purge on an empty cache (no-op, no panic).
func TestPurge_Empty(t *testing.T) {
	c := New[string, int]()
	c.Purge()
	require.Equal(t, 0, c.Len())
}

// TestDelete_Missing covers Delete on a key that is absent (returns false, no
// panic, no callback).
func TestDelete_Missing(t *testing.T) {
	var evicted []string
	c := New[string, int](WithOnEvicted[string, int](func(k string, v int) { evicted = append(evicted, k) }))
	require.False(t, c.Delete("ghost"))
	require.Empty(t, evicted)
}

// TestOnEvicted_ResizeFires covers onEvicted dispatch from Resize.
func TestOnEvicted_ResizeFires(t *testing.T) {
	var evicted []string
	c := New[string, int](
		WithMaxSize[string, int](4),
		WithOnEvicted[string, int](func(k string, v int) { evicted = append(evicted, k) }),
	)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)
	n := c.Resize(1) // evicts 2 entries
	require.Equal(t, 2, n)
	require.Len(t, evicted, 2)
}

// TestKeys_Empty covers Keys on an empty cache.
func TestKeys_Empty(t *testing.T) {
	c := New[string, int]()
	require.Empty(t, c.Keys())
}

// TestGet_Missing covers the not-present branch of Get.
func TestGet_Missing(t *testing.T) {
	c := New[string, int]()
	v, ok := c.Get("missing")
	require.False(t, ok)
	require.Equal(t, 0, v)
}

// TestPeek_Missing covers the not-present branch of Peek.
func TestPeek_Missing(t *testing.T) {
	c := New[string, int]()
	v, ok := c.Peek("missing")
	require.False(t, ok)
	require.Equal(t, 0, v)
}

// TestResize_ConcurrentAndDeterministic sanity-checks Resize+Len under no
// contention beyond the package's existing concurrency model (extra branch
// coverage of the eviction loop body).
func TestResize_DownToOne(t *testing.T) {
	var mu sync.Mutex
	var evicted []string
	c := New[string, int](
		WithMaxSize[string, int](4),
		WithOnEvicted[string, int](func(k string, v int) {
			mu.Lock()
			evicted = append(evicted, k)
			mu.Unlock()
		}),
	)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)
	c.Set("d", 4)
	n := c.Resize(1)
	require.Equal(t, 3, n)
	require.Equal(t, 1, c.Len())
	require.Len(t, evicted, 3)
}
