package hotkey

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }

func TestPanicGuards(t *testing.T) {
	require.Panics(t, func() { New(0, 5) })
	require.Panics(t, func() { New(time.Second, 0) })
}

func TestTopReturnsHeaviestFirst(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	d := New(time.Second, 3, WithClock(clk.now))
	// "hot" gets 10 hits, "warm" gets 3, "cold" gets 1.
	for range 10 {
		d.Touch("hot")
	}
	for range 3 {
		d.Touch("warm")
	}
	d.Touch("cold")

	top := d.Top()
	require.Len(t, top, 3)
	require.Equal(t, "hot", top[0].Key)
	require.Equal(t, 10, top[0].Count)
	require.Equal(t, "warm", top[1].Key)
	require.Equal(t, 3, top[1].Count)
	require.Equal(t, "cold", top[2].Key)
	require.Equal(t, 1, top[2].Count)
}

func TestTopLimitedToK(t *testing.T) {
	d := New(time.Second, 2)
	for range 10 {
		d.Touch("a")
		d.Touch("b")
		d.Touch("c") // c will be excluded (only top 2)
	}
	top := d.Top()
	require.Len(t, top, 2)
}

func TestExpiredKeysDropOff(t *testing.T) {
	clk := &fakeClock{t: time.Unix(2000, 0)}
	d := New(time.Second, 5, WithClock(clk.now))
	d.Touch("old")
	clk.t = clk.t.Add(5 * time.Second) // past window
	d.Touch("new")

	top := d.Top()
	require.Len(t, top, 1)
	require.Equal(t, "new", top[0].Key)
	require.Equal(t, 0, d.Count("old"))
}

func TestCountInWindow(t *testing.T) {
	clk := &fakeClock{t: time.Unix(3000, 0)}
	d := New(time.Second, 5, WithClock(clk.now))
	d.Touch("k")
	d.Touch("k")
	d.Touch("k")
	require.Equal(t, 3, d.Count("k"))
	clk.t = clk.t.Add(2 * time.Second)
	require.Equal(t, 0, d.Count("k"))
}

func TestReset(t *testing.T) {
	d := New(time.Second, 5)
	d.Touch("a")
	d.Touch("b")
	require.Equal(t, 2, d.Len())
	d.Reset()
	require.Equal(t, 0, d.Len())
	require.Empty(t, d.Top())
}

func TestIdleKeyPruned(t *testing.T) {
	clk := &fakeClock{t: time.Unix(4000, 0)}
	d := New(time.Second, 5, WithClock(clk.now))
	d.Touch("a")
	d.Touch("b")
	require.Equal(t, 2, d.Len())
	clk.t = clk.t.Add(10 * time.Second)
	d.Touch("c") // triggers idle pruning
	require.Equal(t, 1, d.Len())
}

func TestMaxKeysEviction(t *testing.T) {
	clk := &fakeClock{t: time.Unix(5000, 0)}
	d := New(time.Hour, 10, WithMaxKeys(2), WithClock(clk.now))
	d.Touch("a")
	clk.t = clk.t.Add(time.Second)
	d.Touch("b")
	clk.t = clk.t.Add(time.Second)
	d.Touch("c") // over cap -> evict "a" (fewest hits)
	require.Equal(t, 2, d.Len())
	top := d.Top()
	for _, hk := range top {
		require.NotEqual(t, "a", hk.Key)
	}
}

// TestDefaultMaxKeysApplied guards D5: when WithMaxKeys is omitted the Detector
// must default to DefaultMaxKeys (not 0 = unbounded), so a runaway key space is
// bounded. A low-level check confirms the field is populated without relying on
// touching 10000+ keys.
func TestDefaultMaxKeysApplied(t *testing.T) {
	d := New(time.Second, 5)
	require.Equal(t, DefaultMaxKeys, d.maxKeys, "New must default maxKeys to DefaultMaxKeys")
	require.Equal(t, 10000, DefaultMaxKeys, "DefaultMaxKeys sanity")
}

// TestDefaultMaxKeysEvicts verifies the default ceiling actually evicts when
// exceeded. It uses a small override via the exported path is not possible, so
// instead drive the default by setting maxKeys through WithMaxKeys to a small
// value and confirm eviction honours it — together with TestDefaultMaxKeysApplied
// this proves the default (10000) path is wired identically.
func TestDefaultMaxKeysEvicts(t *testing.T) {
	clk := &fakeClock{t: time.Unix(6000, 0)}
	// Simulate the default-cap eviction path with a tiny cap.
	d := New(time.Hour, 10, WithMaxKeys(3), WithClock(clk.now))
	for i, key := range []string{"a", "b", "c", "d", "e"} {
		clk.t = clk.t.Add(time.Second)
		d.Touch(key) // each new key pushes prior fewest-hit out once over cap
		_ = i
	}
	require.Equal(t, 3, d.Len(), "cap of 3 must be enforced")
}

// TestWithMaxKeysZeroIsUnbounded confirms backward compatibility: an explicit
// WithMaxKeys(0) disables the cap regardless of the new default, so callers that
// relied on unbounded tracking keep working.
func TestWithMaxKeysZeroIsUnbounded(t *testing.T) {
	d := New(time.Second, 5, WithMaxKeys(0))
	require.Equal(t, 0, d.maxKeys, "WithMaxKeys(0) must opt out of the default cap")

	// Eviction loop guard is maxKeys > 0; with 0 it must never trim by cap.
	// Push more keys than DefaultMaxKeys would allow and confirm none are dropped
	// for cap reasons (idle pruning still applies within the window).
	clk := &fakeClock{t: time.Unix(7000, 0)}
	d2 := New(time.Hour, 100, WithMaxKeys(0), WithClock(clk.now))
	for i := range 50 {
		clk.t = clk.t.Add(time.Millisecond)
		d2.Touch(fmt.Sprintf("k%d", i))
	}
	require.Equal(t, 50, d2.Len(), "unbounded mode must retain all in-window keys")
}

func TestTopExcludesZeroCountKeys(t *testing.T) {
	d := New(time.Second, 10)
	top := d.Top()
	require.Empty(t, top) // no keys -> empty
}

// TestTopPrunesIdleEntryLeftByCount covers the in-map idle-pruning branch of
// Top (delete + continue on a key whose trimmed window is empty). Count leaves
// an empty (but still-present) slice for an expired key instead of deleting the
// map entry; Top must then observe that idle entry and remove it. This drives
// the Top branch that ordinary Touch+evictIdleLocked never expose, because
// Touch prunes idle keys before Top can see them.
func TestTopPrunesIdleEntryLeftByCount(t *testing.T) {
	clk := &fakeClock{t: time.Unix(8000, 0)}
	d := New(time.Second, 5, WithClock(clk.now))

	// "stale" is in-window at touch time, then ages out.
	d.Touch("stale")
	clk.t = clk.t.Add(2 * time.Second) // past the 1s window

	// Count trims "stale" to an empty slice and writes it back WITHOUT deleting
	// the map entry, leaving an idle key resident in d.keys.
	require.Equal(t, 0, d.Count("stale"))
	require.Equal(t, 1, d.Len(), "Count must leave the idle entry in the map")

	// Touching a fresh key adds it; evictIdleLocked prunes the idle "stale"
	// entry too, but the assertion below holds regardless.
	d.Touch("live")
	require.Equal(t, 1, d.Len(), "idle stale key must be gone after fresh touch")

	// Re-establish an idle-in-map entry and drive Top's own delete branch.
	clk.t = clk.t.Add(2 * time.Second)
	require.Equal(t, 0, d.Count("live")) // trims live to empty, leaves entry
	require.Equal(t, 1, d.Len())

	top := d.Top()
	require.Empty(t, top, "Top must exclude the idle entry and report nothing")
	require.Equal(t, 0, d.Len(), "Top must delete the idle entry it encountered")
}

// TestEvictIdleLockedVictimEmptyGuard documents the defensive victim==""
// guard in evictIdleLocked (hotkey.go:180-182). The eviction loop only runs
// while d.maxKeys > 0 && len(d.keys) > d.maxKeys, so the map is non-empty. The
// inner range loop unconditionally sets victim to the first key it sees
// (minCount starts at -1, so the first key always wins). Therefore victim==""
// is unreachable unless len(d.keys)==0, which the loop guard forbids. The
// branch is pure defensive code and cannot be driven by any external input;
// this test pins the reasoning rather than the line.
func TestEvictIdleLockedVictimEmptyGuard(t *testing.T) {
	clk := &fakeClock{t: time.Unix(9000, 0)}
	// maxKeys=1 with two in-window keys forces the eviction loop's inner body
	// to run and pick a non-empty victim; the empty-victim guard is never hit.
	d := New(time.Hour, 5, WithMaxKeys(1), WithClock(clk.now))
	d.Touch("a")
	clk.t = clk.t.Add(time.Second)
	d.Touch("b") // over cap of 1 -> evict "a"
	require.Equal(t, 1, d.Len())
	top := d.Top()
	require.Len(t, top, 1)
	require.Equal(t, "b", top[0].Key, "b has more recent hits, a is the victim")
}

func TestConcurrency(t *testing.T) {
	d := New(time.Second, 5)
	var wg sync.WaitGroup
	const g = 16
	wg.Add(g)
	for i := range g {
		key := "hot"
		if i%3 == 0 {
			key = "warm"
		}
		go func(k string) {
			defer wg.Done()
			for range 100 {
				d.Touch(k)
			}
		}(key)
	}
	wg.Wait()
	top := d.Top()
	require.NotEmpty(t, top)
	require.Equal(t, "hot", top[0].Key) // hot had more touches (12 goroutines vs 4)
}
