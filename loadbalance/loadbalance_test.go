package loadbalance

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func strID(s string) string { return s }

func TestEmpty(t *testing.T) {
	b := New[string](strID, nil)
	_, ok := b.Next()
	require.False(t, ok)
	require.Equal(t, 0, b.Len())
}

func TestRoundRobin(t *testing.T) {
	b := New(strID, []Entry[string]{{"a", 0}, {"b", 0}, {"c", 0}}, WithStrategy[string](StrategyRoundRobin))
	got := make([]string, 0, 7)
	for i := 0; i < 7; i++ {
		v, ok := b.Next()
		require.True(t, ok)
		got = append(got, v)
	}
	require.Equal(t, []string{"a", "b", "c", "a", "b", "c", "a"}, got)
}

// SWRR with weights 5:1:1 must interleave proportionally and never burst the
// heavy node. The canonical nginx example: a,b,c weights 5,1,1 over 7 picks ->
// a,a,b,a,c,a,a (a smooth, not "aaaaa...").
func TestSWRR_CanonicalDistribution(t *testing.T) {
	b := New(strID, []Entry[string]{{"a", 5}, {"b", 1}, {"c", 1}})
	var seq []string
	for i := 0; i < 7; i++ {
		v, _ := b.Next()
		seq = append(seq, v)
	}
	// Exactly proportional over a full cycle (totalWeight=7).
	counts := map[string]int{}
	for _, s := range seq {
		counts[s]++
	}
	require.Equal(t, 5, counts["a"])
	require.Equal(t, 1, counts["b"])
	require.Equal(t, 1, counts["c"])

	// Smoothness: no run of "a" longer than 2 in the first cycle.
	maxRun := 0
	run := 0
	for _, s := range seq {
		if s == "a" {
			run++
			if run > maxRun {
				maxRun = run
			}
		} else {
			run = 0
		}
	}
	require.LessOrEqual(t, maxRun, 2, "SWRR bursted: %v", seq)
}

func TestSWRR_LongRunProportional(t *testing.T) {
	b := New(strID, []Entry[string]{{"x", 3}, {"y", 1}})
	counts := map[string]int{}
	for i := 0; i < 8000; i++ {
		v, _ := b.Next()
		counts[v]++
	}
	// 3:1 ratio -> ~6000:2000, within 3%.
	require.InDelta(t, 6000, counts["x"], 300)
	require.InDelta(t, 2000, counts["y"], 300)
}

func TestRandomDistribution(t *testing.T) {
	b := New(strID, []Entry[string]{{"a", 1}, {"b", 1}, {"c", 1}, {"d", 1}},
		WithStrategy[string](StrategyRandom))
	counts := map[string]int{}
	const N = 40000
	for i := 0; i < N; i++ {
		v, _ := b.Next()
		counts[v]++
	}
	expect := N / 4
	for _, k := range []string{"a", "b", "c", "d"} {
		require.InDelta(t, expect, counts[k], float64(expect)*0.1)
	}
}

func TestWeightedRandomDistribution(t *testing.T) {
	b := New(strID, []Entry[string]{{"a", 7}, {"b", 3}},
		WithStrategy[string](StrategyWeightedRandom))
	counts := map[string]int{}
	const N = 40000
	for i := 0; i < N; i++ {
		v, _ := b.Next()
		counts[v]++
	}
	require.InDelta(t, 28000, counts["a"], 600) // 70%
	require.InDelta(t, 12000, counts["b"], 600) // 30%
}

func TestAddReplaceResetWeight(t *testing.T) {
	b := New(strID, []Entry[string]{{"a", 1}, {"b", 1}})
	require.Equal(t, 2, b.Len())
	b.Add(Entry[string]{"c", 1})
	require.Equal(t, 3, b.Len())
	// Replacing "a" with a new weight resets its weight and SWRR state.
	b.Add(Entry[string]{"a", 9})
	found := false
	for _, e := range b.All() {
		if e.Value == "a" {
			require.Equal(t, 9, e.Weight)
			found = true
		}
	}
	require.True(t, found)
}

func TestRemove(t *testing.T) {
	b := New(strID, []Entry[string]{{"a", 1}, {"b", 2}, {"c", 3}})
	b.Remove("b")
	require.Equal(t, 2, b.Len())
	// Remaining still selectable; b never appears.
	for i := 0; i < 50; i++ {
		v, ok := b.Next()
		require.True(t, ok)
		require.NotEqual(t, "b", v)
	}
}

func TestRemoveMissing(t *testing.T) {
	b := New(strID, []Entry[string]{{"a", 1}})
	b.Remove("zzz")
	require.Equal(t, 1, b.Len())
}

func TestZeroWeightTreatedAsOne(t *testing.T) {
	b := New(strID, []Entry[string]{{"a", 0}, {"b", -1}})
	w := 0
	for _, e := range b.All() {
		require.Equal(t, 1, e.Weight)
		w += e.Weight
	}
	require.Equal(t, 2, w)
}

func TestConcurrency(t *testing.T) {
	b := New(strID, []Entry[string]{{"a", 2}, {"b", 1}, {"c", 1}})
	var wg sync.WaitGroup
	const g = 16
	wg.Add(g)
	for i := 0; i < g; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				_, ok := b.Next()
				_ = ok
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 3, b.Len())
}

func TestAllIsCopy(t *testing.T) {
	b := New(strID, []Entry[string]{{"a", 1}, {"b", 2}})
	all := b.All()
	all[0].Weight = 999 // mutate the copy
	for _, e := range b.All() {
		require.Less(t, e.Weight, 999)
	}
}

// TestNewNilID covers the nil-id fallback in New, where id defaults to
// fmt.Sprintf("%v", v). Without this the fallback closure is never executed.
func TestNewNilID(t *testing.T) {
	// int values: fmt %v gives "1","2". Dedup must work through the default id.
	b := New[int](nil, []Entry[int]{{1, 1}, {2, 1}})
	require.Equal(t, 2, b.Len())
	// Replacing via the default id (same value 1) must reset, not append.
	b.Add(Entry[int]{1, 5})
	require.Equal(t, 2, b.Len())
	// Remove via default id works.
	b.Remove(2)
	require.Equal(t, 1, b.Len())
	// Confirm the default id function is the fmt formatter.
	require.Equal(t, "1", b.id(1))
}

// TestWeightedRandomDefaultStrategy covers nextWeightedRandom through the
// default-fallback case path in Next (strategy != SWRR branch already covered;
// this exercises the weighted-random loop returning from inside the loop body
// for the last entry as well, by drawing many samples).
func TestWeightedRandomLastEntryReachable(t *testing.T) {
	b := New(strID, []Entry[string]{{"a", 1}, {"b", 1}, {"c", 1}},
		WithStrategy[string](StrategyWeightedRandom))
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		v, ok := b.Next()
		require.True(t, ok)
		seen[v] = true
	}
	// Over 1000 uniform draws all three entries are hit (the in-loop return
	// fires for the final entry whenever r lands in its weight bucket).
	for _, k := range []string{"a", "b", "c"} {
		require.True(t, seen[k], "entry %q never selected", k)
	}
}

// TestWeightedRandomFallbackDefensive documents the trailing
// `return b.entries[len(b.entries)-1].value, true` in nextWeightedRandom.
//
// It is mathematically UNREACHABLE through the public API:
//   - r = rand.IntN(totalWeight) ∈ [0, totalWeight)
//   - the loop sums cum to exactly totalWeight (addLocked normalizes w<=0 to 1)
//   - so the last iteration always satisfies r < cum and returns inside the loop
//
// The statement is a pure defensive guard against future internal-state
// corruption. We deliberately do NOT cover it by desyncing b.totalWeight from
// the entry-weight sum, since that would test corrupted private state rather
// than any real code path. Skipped per the "unreachable defensive" carve-out.
func TestWeightedRandomFallbackDefensive(t *testing.T) {
	t.Skip("line 173 is a defensive guard, unreachable via public API; see comment")
}

func ExampleNew() {
	// Send ~75% of traffic to the higher-capacity upstream.
	b := New(strID, []Entry[string]{
		{Value: "ssp-fast:443", Weight: 3},
		{Value: "ssp-backup:443", Weight: 1},
	})
	upstream, ok := b.Next()
	fmt.Println(ok, upstream != "")
	// Output: true true
}
