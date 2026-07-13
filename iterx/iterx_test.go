package iterx_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/iterx"
)

func TestMap(t *testing.T) {
	doubled := iterx.Collect(iterx.Map(slices.Values([]int{1, 2, 3}), func(v int) int {
		return v * 2
	}))
	require.Equal(t, []int{2, 4, 6}, doubled)
}

func TestFilter(t *testing.T) {
	evens := iterx.Collect(iterx.Filter(slices.Values([]int{1, 2, 3, 4, 5, 6}), func(v int) bool {
		return v%2 == 0
	}))
	require.Equal(t, []int{2, 4, 6}, evens)
}

func TestTake(t *testing.T) {
	require.Nil(t, iterx.Collect(iterx.Take(slices.Values([]int{1, 2, 3}), 0)))
	require.Nil(t, iterx.Collect(iterx.Take(slices.Values([]int{1, 2, 3}), -1)))

	require.Equal(t, []int{1, 2}, iterx.Collect(iterx.Take(slices.Values([]int{1, 2, 3, 4}), 2)))

	// n larger than source.
	require.Equal(t, []int{1, 2, 3}, iterx.Collect(iterx.Take(slices.Values([]int{1, 2, 3}), 10)))
}

func TestDrop(t *testing.T) {
	// n <= 0 yields all.
	require.Equal(t, []int{1, 2, 3}, iterx.Collect(iterx.Drop(slices.Values([]int{1, 2, 3}), 0)))

	require.Equal(t, []int{3, 4}, iterx.Collect(iterx.Drop(slices.Values([]int{1, 2, 3, 4}), 2)))

	// n larger than source yields nothing.
	require.Nil(t, iterx.Collect(iterx.Drop(slices.Values([]int{1, 2}), 5)))
}

func TestCollectEmpty(t *testing.T) {
	require.Nil(t, iterx.Collect(slices.Values([]int{})))
}

func TestReduce(t *testing.T) {
	sum := iterx.Reduce(slices.Values([]int{1, 2, 3, 4}), 0, func(acc, v int) int {
		return acc + v
	})
	require.Equal(t, 10, sum)

	// Non-zero initial.
	joined := iterx.Reduce(slices.Values([]string{"a", "b", "c"}), "z", func(acc, v string) string {
		return acc + v
	})
	require.Equal(t, "zabc", joined)

	// Empty → initial.
	require.Equal(t, 42, iterx.Reduce(slices.Values([]int{}), 42, func(a, b int) int { return a + b }))
}

func TestChain(t *testing.T) {
	chained := iterx.Collect(iterx.Chain(
		slices.Values([]int{1, 2}),
		slices.Values([]int{}),
		slices.Values([]int{3, 4, 5}),
	))
	require.Equal(t, []int{1, 2, 3, 4, 5}, chained)

	// No seqs.
	require.Nil(t, iterx.Collect(iterx.Chain[int]()))
}

func TestZip(t *testing.T) {
	pairs := iterx.Collect(iterx.Zip(
		slices.Values([]int{1, 2, 3}),
		slices.Values([]string{"a", "b", "c"}),
	))
	require.Len(t, pairs, 3)
	require.Equal(t, 1, pairs[0].First)
	require.Equal(t, "a", pairs[0].Second)
	require.Equal(t, 3, pairs[2].First)
	require.Equal(t, "c", pairs[2].Second)

	// Stops at the shorter.
	short := iterx.Collect(iterx.Zip(
		slices.Values([]int{1, 2, 3, 4, 5}),
		slices.Values([]string{"a", "b"}),
	))
	require.Len(t, short, 2)

	// Empty left.
	require.Nil(t, iterx.Collect(iterx.Zip(
		slices.Values([]int{}),
		slices.Values([]string{"a"}),
	)))
}

func TestRange(t *testing.T) {
	require.Equal(t, []int{0, 1, 2, 3, 4}, iterx.Collect(iterx.Range(0, 5, 1)))
	require.Equal(t, []int{10, 12, 14, 16}, iterx.Collect(iterx.Range(10, 18, 2)))

	// Negative step.
	require.Equal(t, []int{3, 2, 1}, iterx.Collect(iterx.Range(3, 0, -1)))

	// step == 0 → nothing.
	require.Nil(t, iterx.Collect(iterx.Range(0, 5, 0)))

	// Empty range (start == end).
	require.Nil(t, iterx.Collect(iterx.Range(5, 5, 1)))

	// Inverted direction (step positive but start >= end) → nothing.
	require.Nil(t, iterx.Collect(iterx.Range(5, 0, 1)))
}

func TestSeq2KeysValues(t *testing.T) {
	src := []string{"a", "b", "c"}
	keys := iterx.Collect(iterx.Seq2Keys(slices.All(src)))
	require.Equal(t, []int{0, 1, 2}, keys)

	vals := iterx.Collect(iterx.Seq2Values(slices.All(src)))
	require.Equal(t, []string{"a", "b", "c"}, vals)
}

// TestLazinessAndEarlyTermination proves the combinators are lazy: Take(n) must
// not pull more than n elements from its source. A side-effecting Map counts
// invocations; only the taken count should be realized, not the full 100.
func TestLazinessAndEarlyTermination(t *testing.T) {
	calls := 0
	mapped := iterx.Map(iterx.Range(0, 100, 1), func(i int) int {
		calls++
		return i
	})
	taken := iterx.Take(mapped, 5)
	out := iterx.Collect(taken)

	require.Equal(t, []int{0, 1, 2, 3, 4}, out)
	require.Equal(t, 5, calls, "lazy: only 5 elements should be pulled, not 100")
}

// TestFilterTakeChains composes Filter → Take → Collect and checks the pipeline
// short-circuits: with Take(2) over Filter(evens) of [1..10], only the first
// two evens plus the elements scanned up to them are realized.
func TestFilterTakeChains(t *testing.T) {
	src := slices.Values([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	out := iterx.Collect(iterx.Take(iterx.Filter(src, func(v int) bool { return v%2 == 0 }), 2))
	require.Equal(t, []int{2, 4}, out)
}

// TestEarlyTermination exercises the `yield returns false → return` branch of
// every lazy combinator: a `break` in the range loop must stop the upstream
// immediately rather than draining it. This is the contract that keeps lazy
// pipelines from doing wasted work (and leaking resources in Zip's iter.Pull).
func TestEarlyTermination(t *testing.T) {
	// Map: break after 3.
	var got []int
	for v := range iterx.Map(iterx.Range(0, 1000, 1), func(i int) int { return i }) {
		got = append(got, v)
		if len(got) == 3 {
			break
		}
	}
	require.Equal(t, []int{0, 1, 2}, got)

	// Filter: break after 2 evens.
	got = nil
	for v := range iterx.Filter(iterx.Range(0, 1000, 1), func(i int) bool { return i%2 == 0 }) {
		got = append(got, v)
		if len(got) == 2 {
			break
		}
	}
	require.Equal(t, []int{0, 2}, got)

	// Drop: break after 1.
	got = nil
	for v := range iterx.Drop(iterx.Range(0, 1000, 1), 5) {
		got = append(got, v)
		break
	}
	require.Equal(t, []int{5}, got)

	// Take: break before reaching n (yield returns false path).
	got = nil
	for v := range iterx.Take(iterx.Range(0, 1000, 1), 100) {
		got = append(got, v)
		if len(got) == 2 {
			break
		}
	}
	require.Equal(t, []int{0, 1}, got)

	// Chain: break early in the first seq.
	got = nil
	for v := range iterx.Chain(iterx.Range(0, 1000, 1), iterx.Range(0, 1000, 1)) {
		got = append(got, v)
		if len(got) == 2 {
			break
		}
	}
	require.Equal(t, []int{0, 1}, got)

	// Range negative step: break early.
	got = nil
	for v := range iterx.Range(1000, 0, -1) {
		got = append(got, v)
		if len(got) == 2 {
			break
		}
	}
	require.Equal(t, []int{1000, 999}, got)

	// Zip: break early (also exercises iter.Pull stop cleanup).
	var pairs []int
	for p := range iterx.Zip(iterx.Range(0, 1000, 1), iterx.Range(0, 1000, 1)) {
		pairs = append(pairs, p.First)
		if len(pairs) == 2 {
			break
		}
	}
	require.Equal(t, []int{0, 1}, pairs)

	// Seq2Keys / Seq2Values: break early.
	keys := 0
	for range iterx.Seq2Keys(slices.All([]string{"a", "b", "c", "d"})) {
		keys++
		if keys == 2 {
			break
		}
	}
	require.Equal(t, 2, keys)

	vals := 0
	for range iterx.Seq2Values(slices.All([]string{"a", "b", "c", "d"})) {
		vals++
		if vals == 2 {
			break
		}
	}
	require.Equal(t, 2, vals)
}
