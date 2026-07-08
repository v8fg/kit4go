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

// TestMaxKeysSoftCap covers the R13 F3 fix: maxKeys is a SOFT cap. A live
// (in-window) key is NEVER evicted to make room for a new one — that would
// silently reset its cap and cause over-delivery. When the live-key count
// reaches the cap, a fresh entity's first Allow is denied instead. Idle keys
// are still reclaimed to make room.
func TestMaxKeysSoftCap(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(time.Hour, 5, WithMaxKeys(2), WithClock(clk.now))
	require.True(t, c.Allow("a"))
	clk.t = clk.t.Add(time.Second)
	require.True(t, c.Allow("b"))
	require.Equal(t, 2, c.Len())

	// Both keys live -> a third distinct key must be DENIED, not admitted by
	// evicting "a" (the pre-F3 behaviour, which reset the victim's cap).
	clk.t = clk.t.Add(time.Second)
	require.False(t, c.Allow("c"), "soft cap full: fresh entity denied, no live key dropped")
	require.Equal(t, 2, c.Len(), "no live key evicted")
	require.True(t, c.Allow("b"), "already-tracked key still updates under the cap")
	require.Equal(t, 2, c.Count("b"))

	// Idle reclamation still frees room: age "a" out, then a new key is admitted.
	clk.t = clk.t.Add(2 * time.Hour) // "a" and "b" both aged out
	require.True(t, c.Allow("d"), "idle keys reclaimed, room for a fresh entity")
	require.Equal(t, 1, c.Len(), "only the new live key remains")
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
// confirms the soft cap keeps the map at exactly DefaultMaxKeys entries: the
// overflow key is DENIED (all slots full of live keys) rather than admitted by
// evicting a live one.
func TestDefaultMaxKeysEvictsOverCeiling(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(time.Hour, 1, WithClock(clk.now)) // no WithMaxKeys -> default
	require.Equal(t, DefaultMaxKeys, c.maxKeys)

	// Fill to the ceiling; each Allow is a fresh live key, and the soft cap
	// admits them one per slot.
	for i := range DefaultMaxKeys {
		c.Allow(stringKey(i))
	}
	require.Equal(t, DefaultMaxKeys, c.Len())

	// One more distinct, in-window key is DENIED (soft cap: no live key to
	// reclaim), so the map does not grow past the default ceiling.
	clk.t = clk.t.Add(time.Second)
	require.False(t, c.Allow("overflow"), "soft cap full -> overflow entity denied")
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
		for i := range n {
			c.Allow(stringKey(i))
		}
		require.Equal(t, n, c.Len(), "unbounded map must not prune over the cap")
	})
	t.Run("negative normalised to unbounded", func(t *testing.T) {
		clk := &fakeClock{t: time.Unix(0, 0)}
		c := New(time.Hour, 1, WithMaxKeys(-1), WithClock(clk.now))
		const n = 50
		for i := range n {
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
	for i := range goroutines {
		i := i
		go func() {
			defer wg.Done()
			key := "k"
			if i%2 == 0 {
				key = "k2"
			}
			for range perG {
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

// TestCountReadOnly pins the invariant fixed in R13 F2: Count is read-only with
// respect to the key set. A never-seen key returns 0 WITHOUT creating a map
// entry, and a key whose window has drained to empty is deleted (reclaimed)
// rather than left as a nil/empty-slice stub. This is what keeps the read path
// from bypassing maxKeys: an attacker probing Count with distinct untrusted
// keys must not grow the map.
func TestCountReadOnly(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(time.Second, 1, WithMaxKeys(1), WithClock(clk.now))

	// "a" gets one event, then ages out; Count() must NOT leave an empty-slice
	// stub behind — it reclaims the entry.
	c.Allow("a")
	require.Equal(t, 1, c.Len())
	clk.t = clk.t.Add(2 * time.Second)
	require.Equal(t, 0, c.Count("a"))

	// The drained key must have been removed from the map, not stored as an
	// empty slice. (Pre-F2 the map still held "a" -> []time.Time{} here.)
	c.mu.Lock()
	_, ok := c.keys["a"]
	c.mu.Unlock()
	require.False(t, ok, "Count must delete a drained key, not leave an empty slice")

	// A never-seen key must not be created by Count.
	require.Equal(t, 0, c.Count("ghost"))
	c.mu.Lock()
	_, ok = c.keys["ghost"]
	c.mu.Unlock()
	require.False(t, ok, "Count must not create an entry for an absent key")

	// A new in-window key records cleanly (the reclaimed slot is reused).
	require.True(t, c.Allow("b"))
	require.Equal(t, 1, c.Len())
	require.Equal(t, 1, c.Count("b"))

	// maxKeys is now a SOFT cap: filling to the cap with live keys and then
	// asking for one more distinct key must DENY rather than evict a live key.
	// (c2 exercises the multi-live-key path that the old second loop used to
	// serve by dropping the oldest-start key.)
	c2 := New(time.Hour, 5, WithMaxKeys(2), WithClock(clk.now))
	c2.Allow("a")
	clk.t = clk.t.Add(time.Second)
	c2.Allow("b")
	clk.t = clk.t.Add(time.Second)
	require.False(t, c2.Allow("c"), "soft cap: 2 live keys fill the cap, c must be DENIED")
	require.Equal(t, 2, c2.Len(), "no live key evicted to make room")
	// "a" and "b" keep their caps: their next Allow still counts toward the
	// original cap (no silent reset, no over-delivery).
	require.True(t, c2.Allow("a"))
	require.Equal(t, 2, c2.Count("a"))
}

// TestCountDoesNotGrowMapR13F2 is the F2 regression: the read-only Count
// accessor must not create map entries for never-seen keys, otherwise the
// maxKeys cap is bypassed on the read path. Pre-F2, Count over 50 distinct
// never-seen keys with maxKeys=3 grew the map to 50. It MUST stay at 0.
func TestCountDoesNotGrowMapR13F2(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(time.Hour, 3, WithMaxKeys(3), WithClock(clk.now))
	for i := range 50 {
		require.Equal(t, 0, c.Count(stringKey(i)), "never-seen key reports 0")
	}
	require.Equal(t, 0, c.Len(), "Count must not create entries for absent keys")

	// Even after the map has live entries, probing more never-seen keys must
	// not grow the map beyond what Allow recorded.
	require.True(t, c.Allow("live"))
	require.Equal(t, 1, c.Len())
	for i := range 50 {
		require.Equal(t, 0, c.Count(stringKey(i+100)))
	}
	require.Equal(t, 1, c.Len(), "absent-key Count probes added no entries")
}

// TestSoftCapNoOverDeliveryR13F3 is the F3 regression: when the live-key count
// reaches maxKeys, a fresh entity's first Allow is DENIED rather than evicting a
// live entity. Evicting a live entity would silently reset its cap and cause
// over-delivery (the core "at most maxEvents per entity per window" invariant).
// Pre-F3, u1 at 2/3 impressions was evicted to admit another key, then u1 got 3
// more -> 5/hour instead of 3. The scenario below asserts no over-delivery.
func TestSoftCapNoOverDeliveryR13F3(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	// Default cap of 3 per hour. maxKeys=3 so three live entities saturate it.
	c := New(time.Hour, 3, WithMaxKeys(3), WithClock(clk.now))

	// u1 takes 2 of its 3 allowed impressions.
	require.True(t, c.Allow("u1"))
	require.True(t, c.Allow("u1"))
	require.Equal(t, 2, c.Count("u1"))

	// Two more distinct live keys fill the map to the soft cap.
	require.True(t, c.Allow("u2"))
	require.True(t, c.Allow("u3"))
	require.Equal(t, 3, c.Len())

	// A fourth distinct key is DENIED: the map is full of live audiences and
	// none may be dropped. (Pre-F3: u1 was dropped here, resetting its cap.)
	require.False(t, c.Allow("u4"), "soft cap full -> fresh entity denied, no live key evicted")
	require.Equal(t, 3, c.Len(), "all three live keys retained")

	// u1's cap is intact: it has exactly 1 remaining impression in the window.
	// (Pre-F3: u1's count was reset to 0, so this Allow + 2 more = 5/hour.)
	require.True(t, c.Allow("u1"), "u1 still has 1 remaining within its original cap")
	require.False(t, c.Allow("u1"), "u1 now at cap=3")
	require.False(t, c.Allow("u1"))
	require.Equal(t, 3, c.Count("u1"), "no over-delivery: exactly 3 in window, not 5")
}
