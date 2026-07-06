package topk

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPanicOnZeroK(t *testing.T) {
	require.Panics(t, func() { New(0) })
}

func TestBasicTopK(t *testing.T) {
	tr := New(3)
	tr.TouchN("a", 10)
	tr.TouchN("b", 30)
	tr.TouchN("c", 20)
	tr.TouchN("d", 5)
	top := tr.Top()
	require.Len(t, top, 3)
	require.Equal(t, "b", top[0].Key) // highest
	require.Equal(t, int64(30), top[0].Count)
	require.Equal(t, "c", top[1].Key)
	require.Equal(t, "a", top[2].Key)
}

func TestEviction(t *testing.T) {
	tr := New(2)
	tr.TouchN("a", 1)
	tr.TouchN("b", 2)
	tr.TouchN("c", 3) // evicts "a" (min count)
	top := tr.Top()
	require.Len(t, top, 2)
	keys := map[string]bool{top[0].Key: true, top[1].Key: true}
	require.True(t, keys["b"])
	require.True(t, keys["c"])
	require.False(t, keys["a"])
}

func TestIncremental(t *testing.T) {
	tr := New(3)
	tr.Touch("a")
	tr.Touch("a")
	tr.Touch("a")
	tr.Touch("b")
	tr.Touch("b")
	tr.Touch("c")
	require.Equal(t, int64(3), tr.Count("a"))
	require.Equal(t, int64(2), tr.Count("b"))
}

func TestIncrementUpdatesHeap(t *testing.T) {
	tr := New(3)
	tr.TouchN("a", 1)
	tr.TouchN("b", 2)
	tr.TouchN("c", 3)
	// "a" is in heap with count 1; increment it to surpass "b".
	tr.TouchN("a", 5) // total 6
	top := tr.Top()
	require.Equal(t, "a", top[0].Key) // now highest
	require.Equal(t, int64(6), top[0].Count)
}

func TestFillThenExceed(t *testing.T) {
	tr := New(3)
	for i := 0; i < 100; i++ {
		tr.TouchN("k", 1)
	}
	tr.TouchN("new", 200) // new key with high count
	top := tr.Top()
	require.Contains(t, []string{top[0].Key}, "new")
}

func TestLen(t *testing.T) {
	tr := New(5)
	tr.Touch("a")
	require.Equal(t, 1, tr.Len())
	tr.Touch("b")
	tr.Touch("c")
	require.Equal(t, 3, tr.Len())
}

func TestK(t *testing.T) {
	tr := New(7)
	require.Equal(t, 7, tr.K())
}

func TestReset(t *testing.T) {
	tr := New(3)
	tr.TouchN("a", 10)
	tr.TouchN("b", 20)
	require.Equal(t, 2, tr.Len())
	tr.Reset()
	require.Equal(t, 0, tr.Len())
	require.Equal(t, int64(0), tr.Count("a"))
	require.Empty(t, tr.Top())
}

func TestCountUnseen(t *testing.T) {
	tr := New(3)
	require.Equal(t, int64(0), tr.Count("nope"))
}

func TestTouchNZero(t *testing.T) {
	tr := New(3)
	tr.TouchN("a", 0)
	tr.TouchN("b", -1)
	require.Equal(t, 0, tr.Len())
}

func TestConcurrency(t *testing.T) {
	tr := New(10)
	var wg sync.WaitGroup
	const g = 16
	wg.Add(g)
	for i := 0; i < g; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				tr.Touch("shared")
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1600), tr.Count("shared"))
	require.Contains(t, func() []string {
		top := tr.Top()
		keys := make([]string, len(top))
		for i, e := range top {
			keys[i] = e.Key
		}
		return keys
	}(), "shared")
}

// TestCountsMapBounded is a regression test for an unbounded-growth bug where
// Tracker.counts kept an entry for every distinct key ever seen, even after the
// key was evicted from the size-K minHeap. On a high-cardinality stream (auction
// IDs, user IDs) this leaked O(distinct keys) memory despite the package's
// O(K) claim. Counts must now stay bounded at ~K regardless of stream cardinality.
func TestCountsMapBounded(t *testing.T) {
	const k = 10
	const n = 10000
	tr := New(k)
	for i := 0; i < n; i++ {
		tr.Touch(fmt.Sprintf("key-%d", i))
	}

	// Heap must hold exactly K items.
	require.Equal(t, k, tr.Len())

	// counts map must be bounded — at most K (in-heap) plus tiny slack. The old
	// buggy code left len(counts) == 10000 here.
	tr.mu.Lock()
	countsLen := len(tr.counts)
	tr.mu.Unlock()
	require.LessOrEqual(t, countsLen, k, "counts map grew to %d, expected <= %d", countsLen, k)
	require.Less(t, countsLen, n, "counts map should be bounded well below stream cardinality")

	// Top() result is unaffected: still K entries. Keys that never entered the
	// heap (the vast majority here) are not tracked and report Count 0.
	top := tr.Top()
	require.Len(t, top, k)
	require.Equal(t, int64(0), tr.Count("key-9999"), "never-admitted key should not be tracked")
}

// TestEvictedKeyCountDropped targets the eviction branch directly: a key that
// was admitted to the top-K set and later displaced must have its count entry
// removed from the count map (the original P0 leak).
func TestEvictedKeyCountDropped(t *testing.T) {
	tr := New(2)
	tr.TouchN("a", 5) // admitted
	tr.TouchN("b", 3) // admitted; heap full {b=3, a=5}
	require.Equal(t, int64(5), tr.Count("a"))

	tr.TouchN("c", 10) // displaces "b" (min); evicted key must be dropped
	top := tr.Top()
	require.Len(t, top, 2)
	require.Equal(t, "c", top[0].Key)

	tr.mu.Lock()
	_, aStillThere := tr.counts["a"]
	_, bStillThere := tr.counts["b"]
	tr.mu.Unlock()
	require.True(t, aStillThere, "in-heap key a should still be tracked")
	require.False(t, bStillThere, "evicted key b should be dropped from counts")
	require.Equal(t, int64(0), tr.Count("b"), "evicted key should report count 0")
}
