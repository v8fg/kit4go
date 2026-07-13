package sliceutil_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/sliceutil"
)

func TestChunk(t *testing.T) {
	require.Nil(t, sliceutil.Chunk([]int{1, 2, 3}, 0))
	require.Nil(t, sliceutil.Chunk([]int{1, 2, 3}, -1))
	require.Nil(t, sliceutil.Chunk([]int{}, 2))

	got := sliceutil.Chunk([]int{1, 2, 3, 4, 5}, 2)
	require.Equal(t, [][]int{{1, 2}, {3, 4}, {5}}, got)

	got = sliceutil.Chunk([]int{1, 2, 3, 4}, 2)
	require.Equal(t, [][]int{{1, 2}, {3, 4}}, got)

	// Exact multiple.
	got = sliceutil.Chunk([]int{1, 2, 3, 4, 5, 6}, 3)
	require.Equal(t, [][]int{{1, 2, 3}, {4, 5, 6}}, got)

	// size larger than slice.
	got = sliceutil.Chunk([]int{1, 2}, 5)
	require.Equal(t, [][]int{{1, 2}}, got)
}

func TestChunkSubsliceAliasing(t *testing.T) {
	// Each chunk must be a subslice with independent cap so appending to one
	// cannot corrupt another.
	src := []int{1, 2, 3, 4}
	chunks := sliceutil.Chunk(src, 2)
	require.Equal(t, 2, len(chunks))
	require.Equal(t, 0, cap(chunks[0])-len(chunks[0])) // cap == len (no room to grow)
}

func TestFlatten(t *testing.T) {
	require.Nil(t, sliceutil.Flatten([][]int(nil)))
	require.Nil(t, sliceutil.Flatten([][]int{}))

	got := sliceutil.Flatten([][]int{{1, 2}, {3}, {}, {4, 5, 6}})
	require.Equal(t, []int{1, 2, 3, 4, 5, 6}, got)
}

func TestDeduplicate(t *testing.T) {
	require.Nil(t, sliceutil.Deduplicate([]int{}))
	require.Equal(t, []int{1}, sliceutil.Deduplicate([]int{1}))

	got := sliceutil.Deduplicate([]int{1, 2, 1, 3, 2, 4, 1})
	require.Equal(t, []int{1, 2, 3, 4}, got)

	// Does not mutate input.
	src := []int{3, 3, 1}
	_ = sliceutil.Deduplicate(src)
	require.Equal(t, []int{3, 3, 1}, src)
}

func TestPartition(t *testing.T) {
	even, odd := sliceutil.Partition([]int{1, 2, 3, 4, 5, 6}, func(v int) bool {
		return v%2 == 0
	})
	require.Equal(t, []int{2, 4, 6}, even)
	require.Equal(t, []int{1, 3, 5}, odd)

	// All true / all false.
	all, none := sliceutil.Partition([]int{1, 2}, func(int) bool { return true })
	require.Equal(t, []int{1, 2}, all)
	require.Empty(t, none)

	// Empty input.
	a, b := sliceutil.Partition([]int{}, func(int) bool { return true })
	require.Empty(t, a)
	require.Empty(t, b)
}

func TestGroupBy(t *testing.T) {
	got := sliceutil.GroupBy([]int{1, 2, 3, 4, 5, 6}, func(v int) bool {
		return v%2 == 0
	})
	require.Equal(t, []int{2, 4, 6}, got[true])
	require.Equal(t, []int{1, 3, 5}, got[false])

	// Group by string key.
	words := sliceutil.GroupBy([]string{"go", "rust", "gem", "red"}, func(s string) byte {
		return s[0]
	})
	require.Equal(t, []string{"go", "gem"}, words['g'])
	require.Equal(t, []string{"rust", "red"}, words['r'])
}

func TestWindow(t *testing.T) {
	require.Nil(t, sliceutil.Window([]int{1, 2}, 0))
	require.Nil(t, sliceutil.Window([]int{1, 2}, -1))
	require.Nil(t, sliceutil.Window([]int{1, 2}, 3)) // n > len

	got := sliceutil.Window([]int{1, 2, 3, 4}, 2)
	require.Equal(t, [][]int{{1, 2}, {2, 3}, {3, 4}}, got)

	got = sliceutil.Window([]int{1, 2, 3}, 3)
	require.Equal(t, [][]int{{1, 2, 3}}, got)
}

func TestFill(t *testing.T) {
	s := make([]int, 5)
	sliceutil.Fill(s, 7)
	require.Equal(t, []int{7, 7, 7, 7, 7}, s)

	// Empty.
	sliceutil.Fill([]int{}, 1)

	// Pointer types get the same pointer.
	ptrs := make([]*int, 2)
	v := 42
	sliceutil.Fill(ptrs, &v)
	require.Same(t, &v, ptrs[0])
	require.Same(t, &v, ptrs[1])
}

func TestRepeat(t *testing.T) {
	require.Nil(t, sliceutil.Repeat(1, 0))
	require.Nil(t, sliceutil.Repeat(1, -3))

	require.Equal(t, []int{7, 7, 7}, sliceutil.Repeat(7, 3))
}

func TestReverse(t *testing.T) {
	require.Nil(t, sliceutil.Reverse[int](nil))

	got := sliceutil.Reverse([]int{1, 2, 3})
	require.Equal(t, []int{3, 2, 1}, got)

	// Does not mutate input.
	src := []int{1, 2, 3}
	_ = sliceutil.Reverse(src)
	require.Equal(t, []int{1, 2, 3}, src)
}

func TestAssociate(t *testing.T) {
	type user struct {
		ID   int
		Name string
	}
	users := []user{{1, "a"}, {2, "b"}, {3, "c"}}
	m := sliceutil.Associate(users, func(u user) (int, string) {
		return u.ID, u.Name
	})
	require.Equal(t, "a", m[1])
	require.Equal(t, "c", m[3])
	require.Len(t, m, 3)

	// Last-wins on key conflict.
	m2 := sliceutil.Associate([]int{1, 2, 1}, func(v int) (int, int) {
		return v, v * 10
	})
	require.Equal(t, 10, m2[1])
	require.Len(t, m2, 2)
}

func TestIndex(t *testing.T) {
	require.Equal(t, -1, sliceutil.Index([]int{1, 2, 3}, 9))
	require.Equal(t, 1, sliceutil.Index([]int{1, 2, 3}, 2))
	// First occurrence.
	require.Equal(t, 0, sliceutil.Index([]int{1, 1, 1}, 1))
}

func TestContains(t *testing.T) {
	require.False(t, sliceutil.Contains([]int{1, 2, 3}, 9))
	require.True(t, sliceutil.Contains([]int{1, 2, 3}, 2))
	require.False(t, sliceutil.Contains([]int{}, 1))
}
