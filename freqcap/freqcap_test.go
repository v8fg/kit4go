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

// TestDefaultMaxKeysApplied guards the D5 fix: omitting WithMaxKeys (or passing
// 0) must cap tracked keys at DefaultMaxKeys instead of leaving the map
// unbounded.
func TestDefaultMaxKeysApplied(t *testing.T) {
	// Sanity: the documented default is a sane, finite ceiling.
	require.Equal(t, 10000, DefaultMaxKeys)

	t.Run("omitted option applies default", func(t *testing.T) {
		c := New(time.Hour, 1)
		require.Equal(t, DefaultMaxKeys, c.maxKeys)
	})
	t.Run("explicit zero applies default", func(t *testing.T) {
		c := New(time.Hour, 1, WithMaxKeys(0))
		require.Equal(t, DefaultMaxKeys, c.maxKeys)
	})
	t.Run("explicit positive cap is preserved", func(t *testing.T) {
		c := New(time.Hour, 1, WithMaxKeys(42))
		require.Equal(t, 42, c.maxKeys)
	})
	t.Run("negative means unbounded", func(t *testing.T) {
		c := New(time.Hour, 1, WithMaxKeys(-1))
		require.Equal(t, -1, c.maxKeys)
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

// TestUnboundedNoEviction confirms the negative-sentinel escape hatch disables
// the key cap entirely (legacy 0=unbounded behaviour, now opt-in).
func TestUnboundedNoEviction(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(time.Hour, 1, WithMaxKeys(-1), WithClock(clk.now))
	const n = 50
	for i := 0; i < n; i++ {
		c.Allow(stringKey(i))
	}
	require.Equal(t, n, c.Len(), "unbounded map must not prune over the cap")
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
