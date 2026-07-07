package lru

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// withClock injects a clock for deterministic TTL tests (test-only).
func withClock[K comparable, V any](f func() time.Time) Option[K, V] {
	return func(c *Cache[K, V]) { c.clock = f }
}

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }

func TestSetAndGet(t *testing.T) {
	c := New[string, int](WithMaxSize[string, int](2))
	c.Set("a", 1)
	c.Set("b", 2)
	v, ok := c.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, v)
	_, ok = c.Get("missing")
	require.False(t, ok)
}

func TestEvictionOrder(t *testing.T) {
	c := New[string, int](WithMaxSize[string, int](2))
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3) // evicts "a" (a was oldest)
	require.False(t, c.Contains("a"))
	require.True(t, c.Contains("b"))
	require.True(t, c.Contains("c"))
}

func TestGetPromotesRecency(t *testing.T) {
	c := New[string, int](WithMaxSize[string, int](2))
	c.Set("a", 1)
	c.Set("b", 2)
	c.Get("a")    // a is now most-recent
	c.Set("c", 3) // evicts b (now least-recent)
	require.True(t, c.Contains("a"))
	require.False(t, c.Contains("b"))
	require.True(t, c.Contains("c"))
}

func TestSetUpdatesExistingAndPromotes(t *testing.T) {
	c := New[string, int](WithMaxSize[string, int](2))
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("a", 100) // a updated + promoted; still 2 entries
	require.Equal(t, 2, c.Len())
	v, ok := c.Get("a")
	require.True(t, ok)
	require.Equal(t, 100, v)
	c.Set("c", 3) // evicts b
	require.False(t, c.Contains("b"))
}

func TestPeekDoesNotPromote(t *testing.T) {
	c := New[string, int](WithMaxSize[string, int](2))
	c.Set("a", 1)
	c.Set("b", 2)
	v, ok := c.Peek("a") // read without promoting
	require.True(t, ok)
	require.Equal(t, 1, v)
	c.Set("c", 3) // a is still least-recent -> evicted
	require.False(t, c.Contains("a"))
	require.True(t, c.Contains("c"))
}

func TestDelete(t *testing.T) {
	c := New[string, int]()
	c.Set("a", 1)
	require.True(t, c.Delete("a"))
	require.False(t, c.Delete("a"))
	require.False(t, c.Contains("a"))
}

func TestKeysOrder(t *testing.T) {
	c := New[string, int](WithMaxSize[string, int](3))
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)
	c.Get("a") // order now a, c, b
	require.Equal(t, []string{"a", "c", "b"}, c.Keys())
}

func TestPurge(t *testing.T) {
	var evicted []string
	c := New[string, int](
		WithMaxSize[string, int](4),
		WithOnEvicted[string, int](func(k string, _ int) { evicted = append(evicted, k) }),
	)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Purge()
	require.Equal(t, 0, c.Len())
	require.ElementsMatch(t, []string{"a", "b"}, evicted)
}

func TestOnEvicted_FiresOnEvict(t *testing.T) {
	var evicted []string
	c := New[string, int](
		WithMaxSize[string, int](2),
		WithOnEvicted[string, int](func(k string, _ int) { evicted = append(evicted, k) }),
	)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3) // evicts a
	require.Equal(t, []string{"a"}, evicted)
}

func TestResize(t *testing.T) {
	c := New[string, int](WithMaxSize[string, int](4))
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)
	c.Set("d", 4)
	evicted := c.Resize(2) // evicts c,d (least-recent are c,d after inserting a,b,c,d)
	require.Equal(t, 2, evicted)
	require.Equal(t, 2, c.Len())
	require.True(t, c.Contains("d")) // d most recent, c next; a,b evicted
	require.False(t, c.Contains("a"))
}

func TestResizeDisableEviction(t *testing.T) {
	c := New[string, int](WithMaxSize[string, int](2))
	c.Set("a", 1)
	c.Set("b", 2)
	c.Resize(0) // disable
	c.Set("c", 3)
	c.Set("d", 4)
	require.Equal(t, 4, c.Len()) // nothing evicted
}

func TestDefaultMaxSize1024(t *testing.T) {
	c := New[int, int]()
	for i := range 1100 {
		c.Set(i, i)
	}
	require.Equal(t, 1024, c.Len())
}

// --- TTL tests with an injected clock (no sleeping) ---

func TestTTLExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	c := New[string, int](
		WithTTL[string, int](10*time.Second),
		withClock[string, int](clk.now),
	)
	c.Set("a", 1)
	v, ok := c.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, v)

	clk.t = clk.t.Add(9 * time.Second)
	require.True(t, c.Contains("a")) // not yet

	clk.t = clk.t.Add(2 * time.Second) // now 11s later -> expired
	_, ok = c.Get("a")
	require.False(t, ok)
	require.Equal(t, 0, c.Len())
}

func TestSetWithTTLPerEntry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(2000, 0)}
	c := New[string, int](withClock[string, int](clk.now))
	c.SetWithTTL("short", 1, 5*time.Second)
	c.SetWithTTL("long", 2, 50*time.Second)
	clk.t = clk.t.Add(10 * time.Second)
	_, okShort := c.Get("short")
	vLong, okLong := c.Get("long")
	require.False(t, okShort)
	require.True(t, okLong)
	require.Equal(t, 2, vLong)
}

func TestSetRefreshResetsTTL(t *testing.T) {
	clk := &fakeClock{t: time.Unix(3000, 0)}
	c := New[string, int](
		WithTTL[string, int](10*time.Second),
		withClock[string, int](clk.now),
	)
	c.Set("a", 1)
	clk.t = clk.t.Add(8 * time.Second)
	c.Set("a", 2)                      // refresh -> expiry pushed out another 10s from now
	clk.t = clk.t.Add(9 * time.Second) // 17s since original insert, 9s since refresh
	v, ok := c.Get("a")
	require.True(t, ok)
	require.Equal(t, 2, v)
}

func TestDeleteExpired(t *testing.T) {
	clk := &fakeClock{t: time.Unix(4000, 0)}
	c := New[string, int](
		WithTTL[string, int](5*time.Second),
		withClock[string, int](clk.now),
	)
	c.Set("a", 1)
	c.Set("b", 2)
	require.Equal(t, 2, c.Len())
	clk.t = clk.t.Add(10 * time.Second)
	require.Equal(t, 2, c.Len()) // lazy: still present until sweep/access
	n := c.DeleteExpired()
	require.Equal(t, 2, n)
	require.Equal(t, 0, c.Len())
}

func TestZeroTTLNoExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(5000, 0)}
	c := New[string, int](withClock[string, int](clk.now))
	c.SetWithTTL("forever", 1, 0) // 0 = no expiry
	clk.t = clk.t.Add(100 * 365 * 24 * time.Hour)
	v, ok := c.Get("forever")
	require.True(t, ok)
	require.Equal(t, 1, v)
}

func TestConcurrency(t *testing.T) {
	c := New[int, int](WithMaxSize[int, int](256))
	var wg sync.WaitGroup
	var errors atomic.Int64
	const goroutines = 32
	wg.Add(goroutines * 2)
	// writers
	for i := range goroutines {
		i := i
		go func() {
			defer wg.Done()
			for j := range 500 {
				c.Set(i*1000+j, j)
			}
		}()
	}
	// readers
	for range goroutines {
		go func() {
			defer wg.Done()
			for j := range 500 {
				c.Get(j)
				c.Peek(j % 10)
				c.Len()
				c.Keys()
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(0), errors.Load())
	// Must not exceed max size.
	require.LessOrEqual(t, c.Len(), 256)
}

func TestEmptyValueRoundTrip(t *testing.T) {
	c := New[string, string]()
	c.Set("k", "")
	v, ok := c.Get("k")
	require.True(t, ok) // present even though value is the zero string
	require.Equal(t, "", v)
}
