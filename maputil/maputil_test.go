package maputil_test

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/maputil"
)

func TestMerge(t *testing.T) {
	a := map[int]string{1: "a", 2: "b"}
	b := map[int]string{2: "B", 3: "c"}

	// Later maps win on conflict.
	merged := maputil.Merge(a, b)
	require.Equal(t, "a", merged[1])
	require.Equal(t, "B", merged[2]) // b overrides a
	require.Equal(t, "c", merged[3])
	require.Len(t, merged, 3)

	// No mutation of inputs.
	require.Equal(t, "b", a[2])

	// Single / none.
	require.Equal(t, a, maputil.Merge(a))
	require.Empty(t, maputil.Merge[int, string]())
}

func TestInvert(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	inv := maputil.Invert(m)
	require.Equal(t, "a", inv[1])
	require.Equal(t, "b", inv[2])
	require.Equal(t, "c", inv[3])
	require.Len(t, inv, 3)

	// Last-wins on duplicate values.
	dup := map[string]int{"x": 1, "y": 1}
	inv2 := maputil.Invert(dup)
	require.Len(t, inv2, 1)
	require.True(t, inv2[1] == "x" || inv2[1] == "y")

	// Input not mutated.
	require.Len(t, m, 3)
}

func TestFromSlice(t *testing.T) {
	type kv struct {
		K int
		V string
	}
	m := maputil.FromSlice([]kv{{1, "a"}, {2, "b"}}, func(e kv) (int, string) {
		return e.K, e.V
	})
	require.Equal(t, "a", m[1])
	require.Equal(t, "b", m[2])
	require.Len(t, m, 2)

	// Empty.
	require.Empty(t, maputil.FromSlice([]kv{}, func(e kv) (int, string) { return e.K, e.V }))
}

func TestToSlice(t *testing.T) {
	m := map[int]string{1: "a", 2: "b", 3: "c"}
	pairs := maputil.ToSlice(m)
	require.Len(t, pairs, 3)

	// Map iteration order is random — sort for a stable assertion.
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Key < pairs[j].Key })
	require.Equal(t, []maputil.KV[int, string]{{1, "a"}, {2, "b"}, {3, "c"}}, pairs)

	// Empty map → empty slice (not nil, since len=0 → make returns non-nil).
	require.Empty(t, maputil.ToSlice(map[int]string{}))
}

func TestEqual(t *testing.T) {
	require.True(t, maputil.Equal(map[int]int{1: 2, 3: 4}, map[int]int{1: 2, 3: 4}))
	require.False(t, maputil.Equal(map[int]int{1: 2}, map[int]int{1: 3}))
	require.False(t, maputil.Equal(map[int]int{1: 2}, map[int]int{1: 2, 3: 4}))
	require.True(t, maputil.Equal(map[int]int{}, map[int]int{}))
}

func TestCopy(t *testing.T) {
	dst := map[int]string{}
	src := map[int]string{1: "a", 2: "b"}
	maputil.Copy(dst, src)
	require.Equal(t, "a", dst[1])
	require.Equal(t, "b", dst[2])
	require.Len(t, dst, 2)
}
