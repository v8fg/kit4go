package freqcap

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPanicGuards(t *testing.T) {
	require.Panics(t, func() { New(time.Second, 0) })
	require.Panics(t, func() { New(0, 3) })
}

func TestAllowUnderCap(t *testing.T) {
	c := New(time.Hour, 3)
	require.True(t, c.Allow("u1"))
	require.True(t, c.Allow("u1"))
	require.True(t, c.Allow("u1"))
	require.False(t, c.Allow("u1")) // 4th in the hour -> rejected
}

func TestKeysAreIndependent(t *testing.T) {
	c := New(time.Hour, 1)
	require.True(t, c.Allow("a"))
	require.True(t, c.Allow("b")) // different key, own cap
	require.False(t, c.Allow("a"))
	require.False(t, c.Allow("b"))
}

func TestWindowExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(10*time.Second, 2, WithClock(clk.now))

	require.True(t, c.Allow("u"))
	clk.t = clk.t.Add(5 * time.Second)
	require.True(t, c.Allow("u"))
	require.False(t, c.Allow("u")) // 2 in window
	require.Equal(t, 2, c.Count("u"))

	clk.t = clk.t.Add(6 * time.Second) // first event now 11s old -> expired
	require.True(t, c.Allow("u"))      // 1 still in window + 1 new = 2
	require.False(t, c.Allow("u"))
}

func TestCountAndReset(t *testing.T) {
	c := New(time.Minute, 5)
	c.Allow("u")
	c.Allow("u")
	require.Equal(t, 2, c.Count("u"))
	c.Reset("u")
	require.Equal(t, 0, c.Count("u"))
	require.True(t, c.Allow("u")) // fresh after reset
}

func TestIdleKeysPruned(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(time.Second, 5, WithClock(clk.now))
	c.Allow("a")
	c.Allow("b")
	require.Equal(t, 2, c.Len())
	clk.t = clk.t.Add(10 * time.Second) // both idle
	c.Allow("c")                        // triggers idle pruning
	require.Equal(t, 1, c.Len())
}

func TestMaxKeysEviction(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(time.Hour, 5, WithMaxKeys(2), WithClock(clk.now))
	c.Allow("a")
	clk.t = clk.t.Add(time.Second)
	c.Allow("b")
	clk.t = clk.t.Add(time.Second)
	c.Allow("c") // over cap (3 > 2): oldest-start key "a" evicted
	require.Equal(t, 2, c.Len())
	require.True(t, c.Allow("b")) // b still tracked
	require.True(t, c.Allow("c"))
}

// TestDefaultMaxKeysApplied guards the D5 fix: omitting WithMaxKeys must cap
// tracked keys at DefaultMaxKeys instead of leaving the map unbounded. An
// explicit WithMaxKeys(0) is the documented opt-out (unbounded), matching
// package hotkey's zero-value convention.
func TestDefaultMaxKeysApplied(t *testing.T) {
	// Sanity: the documented default is a sane, finite ceiling.
	require.Equal(t, 10000, DefaultMaxKeys)

	t.Run("omitted option applies default", func(t *testing.T) {
		c := New(time.Hour, 1)
		require.Equal(t, DefaultMaxKeys, c.maxKeys)
	})
	t.Run("explicit zero means unbounded", func(t *testing.T) {
		c := New(time.Hour, 1, WithMaxKeys(0))
		require.Equal(t, 0, c.maxKeys, "WithMaxKeys(0) must opt out of the default cap")
	})
	t.Run("explicit positive cap is preserved", func(t *testing.T) {
		c := New(time.Hour, 1, WithMaxKeys(42))
		require.Equal(t, 42, c.maxKeys)
	})
	t.Run("negative normalised to unbounded", func(t *testing.T) {
		c := New(time.Hour, 1, WithMaxKeys(-1))
		require.Equal(t, 0, c.maxKeys, "negative values normalise to the 0 unbounded sentinel")
	})
}

// TestDefaultMaxKeysEvictsOverCeiling drives the default cap to overflow and
// confirms eviction keeps the map at exactly DefaultMaxKeys entries.
func TestDefaultMaxKeysEvictsOverCeiling(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(time.Hour, 1, WithClock(clk.now)) // no WithMaxKeys -> default
	require.Equal(t, DefaultMaxKeys, c.maxKeys)

	// Fill to the ceiling; each Allow triggers a prune of idle keys, so the
	// map size never exceeds the cap while events are all in-window.
	for i := 0; i < DefaultMaxKeys; i++ {
		c.Allow(stringKey(i))
	}
	require.Equal(t, DefaultMaxKeys, c.Len())

	// One more distinct, in-window key must evict the oldest-start key rather
	// than grow the map past the default ceiling.
	clk.t = clk.t.Add(time.Second)
	c.Allow("overflow")
	require.Equal(t, DefaultMaxKeys, c.Len(), "default ceiling must hold")
}

// TestUnboundedNoEviction confirms the zero-sentinel escape hatch disables the
// key cap entirely (WithMaxKeys(0), matching package hotkey's zero-value
// convention). A negative value is normalised to the same unbounded behaviour.
func TestUnboundedNoEviction(t *testing.T) {
	t.Run("zero is unbounded", func(t *testing.T) {
		clk := &fakeClock{t: time.Unix(0, 0)}
		c := New(time.Hour, 1, WithMaxKeys(0), WithClock(clk.now))
		const n = 50
		for i := 0; i < n; i++ {
			c.Allow(stringKey(i))
		}
		require.Equal(t, n, c.Len(), "unbounded map must not prune over the cap")
	})
	t.Run("negative normalised to unbounded", func(t *testing.T) {
		clk := &fakeClock{t: time.Unix(0, 0)}
		c := New(time.Hour, 1, WithMaxKeys(-1), WithClock(clk.now))
		const n = 50
		for i := 0; i < n; i++ {
			c.Allow(stringKey(i))
		}
		require.Equal(t, n, c.Len(), "negative maxKeys is unbounded after normalisation")
	})
}

// stringKey returns a deterministic distinct key for i.
func stringKey(i int) string {
	return "k" + itoa(i)
}

// itoa is a stdlib-free non-negative int -> string for tests.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

func TestConcurrency(t *testing.T) {
	c := New(time.Second, 100)
	const goroutines = 32
	const perG = 200
	var wg sync.WaitGroup
	var allowed atomic.Int64
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			key := "k"
			if i%2 == 0 {
				key = "k2"
			}
			for j := 0; j < perG; j++ {
				if c.Allow(key) {
					allowed.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	// Across both keys, allowed must never exceed 2 * cap = 200.
	require.LessOrEqual(t, allowed.Load(), int64(200))
	require.Greater(t, allowed.Load(), int64(0))
}

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }

// TestEvictionSecondLoopCannotSeeEmptySlice pins the invariant that makes two
// branches of evictIdleLocked unreachable, so they are documented here rather
// than left as silent coverage gaps.
//
// The second eviction loop (drop oldest-start key once the idle prune is not
// enough) has two defensive branches:
//
//	if len(ts) == 0 { victim = k; break }   // freqcap.go:146-148
//	if victim == "" { return }              // freqcap.go:154-156
//
// Both are unreachable given the first loop: the first loop deletes every key
// whose trimmed slice is empty, and trimBefore only ever shrinks a slice, so
// every key that survives into the second loop has len(ts) >= 1. Therefore the
// len(ts) == 0 check can never fire, and because the second loop only runs when
// len(c.keys) > c.maxKeys >= 0 (i.e. at least one key is present) the inner
// range always sets `victim` on its first iteration (the `first` flag is true),
// so victim == "" can never fire either. This test asserts the surviving-keys
// property directly so a future change to the first loop does not silently make
// the second loop's branches reachable (and buggy) without flagging it here.
func TestEvictionSecondLoopCannotSeeEmptySlice(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(time.Second, 1, WithMaxKeys(1), WithClock(clk.now))

	// "a" gets one event, then ages out; Count() lazy-trims it to an empty
	// slice still held under the key.
	c.Allow("a")
	clk.t = clk.t.Add(2 * time.Second)
	c.Count("a") // stores []time.Time{} under "a", does not prune

	// Confirm the empty-slice state was in fact materialised by Count.
	c.mu.Lock()
	ts, ok := c.keys["a"]
	c.mu.Unlock()
	require.True(t, ok)
	require.Len(t, ts, 0)

	// A new in-window key forces evictIdleLocked. The first loop must delete
	// "a" (its trimmed slice is empty); the second loop then never runs since
	// len(c.keys)==1 <= maxKeys==1. The empty-slice branch in loop 2 is
	// therefore never reached, and "b" is recorded cleanly.
	require.True(t, c.Allow("b"))
	require.Equal(t, 1, c.Len())
	require.Equal(t, 1, c.Count("b"))

	// Drive the multi-key, all-non-empty path through loop 2 to confirm that
	// path deletes the oldest-start key without needing either defensive
	// branch.
	c2 := New(time.Hour, 5, WithMaxKeys(2), WithClock(clk.now))
	c2.Allow("a")
	clk.t = clk.t.Add(time.Second)
	c2.Allow("b")
	clk.t = clk.t.Add(time.Second)
	require.True(t, c2.Allow("c")) // 3 > 2 -> evict oldest-start ("a")
	require.Equal(t, 2, c2.Len())
	require.Equal(t, 0, c2.Count("a")) // evicted -> 0, not stored empty slice
}
