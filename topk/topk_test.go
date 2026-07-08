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
	for range 100 {
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
	for range g {
		go func() {
			defer wg.Done()
			for range 100 {
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
// O(K) claim. Counts must stay bounded at O(K) — the in-heap keys plus a bounded
// candidate set of at most candidateFactor*K entries — regardless of stream
// cardinality. The old buggy code left len(counts) == 10000 here.
func TestCountsMapBounded(t *testing.T) {
	const k = 10
	const n = 10000
	tr := New(k)
	for i := range n {
		tr.Touch(fmt.Sprintf("key-%d", i))
	}

	// Heap must hold exactly K items.
	require.Equal(t, k, tr.Len())

	// counts map must be bounded at O(K): in-heap keys (K) plus candidates
	// (<= (candidateFactor-1)*K), i.e. <= candidateFactor*K.
	tr.mu.Lock()
	countsLen := len(tr.counts)
	tr.mu.Unlock()
	maxAllowed := candidateFactor * k
	require.LessOrEqual(t, countsLen, maxAllowed, "counts map grew to %d, expected <= %d", countsLen, maxAllowed)
	require.Less(t, countsLen, n, "counts map should be bounded well below stream cardinality")

	// Top() result is unaffected: still K entries.
	top := tr.Top()
	require.Len(t, top, k)
}

// TestDemotedKeyBecomesCandidate targets the eviction branch directly: a key
// that was admitted to the top-K set and later displaced is NOT immediately
// dropped — it becomes a candidate so it can re-accumulate if touched again.
// (An earlier, stricter O(K) variant dropped it at once, but that made any key
// arriving incrementally via Touch unable to ever accumulate — see
// TestLateHeavyHitterViaTouch.) The candidate is only dropped once the cap is
// exceeded; TestCandidateEvictionAtCap covers that.
func TestDemotedKeyBecomesCandidate(t *testing.T) {
	tr := New(2)
	tr.TouchN("a", 5) // admitted
	tr.TouchN("b", 3) // admitted; heap full {b=3, a=5}
	require.Equal(t, int64(5), tr.Count("a"))

	tr.TouchN("c", 10) // displaces "b" (min); "b" becomes a candidate
	top := tr.Top()
	require.Len(t, top, 2)
	require.Equal(t, "c", top[0].Key)

	tr.mu.Lock()
	_, aStillThere := tr.counts["a"]
	_, bStillThere := tr.counts["b"]
	tr.mu.Unlock()
	require.True(t, aStillThere, "in-heap key a should still be tracked")
	require.True(t, bStillThere, "demoted key b should remain a candidate")
	require.Equal(t, int64(3), tr.Count("b"), "demoted key keeps its accumulated count")

	// A later touch on the demoted key builds on the retained count and can
	// re-admit it (here it beats the new min "a"=5 once bumped past it).
	tr.TouchN("b", 10) // candidate count 3 + 10 = 13 > min(a=5) → re-admitted
	top = tr.Top()
	require.Equal(t, "b", top[0].Key)
	require.Equal(t, int64(13), top[0].Count)
}

// TestCandidateEvictionAtCap verifies that when the candidate set fills up, the
// smallest-count candidate is dropped (memory stays O(K)) while in-heap keys are
// always preserved.
func TestCandidateEvictionAtCap(t *testing.T) {
	const k = 2
	tr := New(k)
	// Fill the heap with high counts.
	tr.TouchN("h1", 100)
	tr.TouchN("h2", 200)

	// Add candidates until the cap (candidateFactor*k == 8) is exceeded. The
	// first candidate to be evicted is the smallest-count non-heap key.
	for i := range candidateFactor * k {
		tr.TouchN(fmt.Sprintf("c%d", i), int64(i+1))
	}

	tr.mu.Lock()
	countsLen := len(tr.counts)
	_, h1There := tr.counts["h1"]
	_, h2There := tr.counts["h2"]
	tr.mu.Unlock()
	require.LessOrEqual(t, countsLen, candidateFactor*k, "counts must stay <= cap")
	require.True(t, h1There, "in-heap key h1 must never be evicted")
	require.True(t, h2There, "in-heap key h2 must never be evicted")
}

// TestLateHeavyHitterViaTouch is the regression test for R12 starvation: a key
// with a high true count that arrives incrementally via Touch (one event at a
// time, the natural streaming API) must surface in the top-K. The earlier
// strict-O(K) fix discarded a rejected key's count on every Touch, so
// newCount = 0+1 = 1 forever and the key could never accumulate — a true count
// of 100 reported Count==0 and was absent from the leaderboard. With the
// bounded-candidate fix the key accumulates and displaces an incumbent.
//
// Table covers the documented edge inputs: positive (heavy > all incumbents),
// tied-to-min (must surface by beating, not equaling), and a negative control
// (a key that never beats the min stays out).
func TestLateHeavyHitterViaTouch(t *testing.T) {
	tests := []struct {
		name       string
		incumbents []struct {
			key   string
			count int64
		}
		heavyKey     string
		touches      int // number of Touch(key) calls
		wantInTop    bool
		wantCount    int64
		wantNegative bool // negative control: key must NOT be in top, count 0 or low
	}{
		{
			name: "heavy beats all incumbents",
			incumbents: []struct {
				key   string
				count int64
			}{{"a", 1}, {"b", 1}, {"c", 1}},
			heavyKey:  "realheavy",
			touches:   100,
			wantInTop: true,
			wantCount: 100,
		},
		{
			name: "heavy exactly equals min then exceeds (mixed)",
			incumbents: []struct {
				key   string
				count int64
			}{{"a", 1}, {"b", 1}, {"c", 1}},
			heavyKey:  "tiebreaker",
			touches:   2, // count 2 > min 1
			wantInTop: true,
			wantCount: 2,
		},
		{
			name: "negative control: key never beats min stays out",
			incumbents: []struct {
				key   string
				count int64
			}{{"a", 1000}, {"b", 1000}, {"c", 1000}},
			heavyKey:     "lightweight",
			touches:      1, // count 1 < min 1000
			wantInTop:    false,
			wantNegative: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := New(len(tc.incumbents))
			for _, inc := range tc.incumbents {
				tr.TouchN(inc.key, inc.count)
			}
			for range tc.touches {
				tr.Touch(tc.heavyKey)
			}

			top := tr.Top()
			keys := make(map[string]int64, len(top))
			for _, e := range top {
				keys[e.Key] = e.Count
			}

			if tc.wantNegative {
				_, inTop := keys[tc.heavyKey]
				require.False(t, inTop, "%s must not be in top-K (never beat min)", tc.heavyKey)
				return
			}

			count, inTop := keys[tc.heavyKey]
			require.True(t, inTop, "%s (touched %d times) must surface in top-K", tc.heavyKey, tc.touches)
			require.Equal(t, tc.wantCount, count, "reported count for %s", tc.heavyKey)
			require.Equal(t, tc.wantCount, tr.Count(tc.heavyKey), "Count() must match Top()")
		})
	}
}
