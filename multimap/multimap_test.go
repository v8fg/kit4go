package multimap_test

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/multimap"
)

func TestAddAndGet(t *testing.T) {
	mm := multimap.New[string, string]()
	mm.Add("tag", "a")
	mm.Add("tag", "b")
	mm.Add("tag", "c")
	mm.Add("env", "prod")

	require.Equal(t, []string{"a", "b", "c"}, mm.Get("tag"))
	require.Equal(t, []string{"prod"}, mm.Get("env"))
	require.Nil(t, mm.Get("missing"))

	require.True(t, mm.Has("tag"))
	require.False(t, mm.Has("missing"))
}

func TestAddAll(t *testing.T) {
	mm := multimap.New[int, int]()
	mm.Add(1, 10)
	mm.AddAll(1, []int{20, 30})
	require.Equal(t, []int{10, 20, 30}, mm.Get(1))

	// AddAll on empty bucket.
	mm.AddAll(2, []int{5})
	require.Equal(t, []int{5}, mm.Get(2))
}

func TestSet(t *testing.T) {
	mm := multimap.New[string, int]()
	mm.Add("k", 1)
	mm.Add("k", 2)

	// Replace bucket with a copy.
	mm.Set("k", []int{9, 9, 9})
	require.Equal(t, []int{9, 9, 9}, mm.Get("k"))

	// Mutating the source slice must not affect the map.
	src := []int{7}
	mm.Set("k", src)
	src[0] = 999
	require.Equal(t, []int{7}, mm.Get("k"))

	// Empty slice removes the key.
	mm.Set("k", nil)
	require.False(t, mm.Has("k"))
}

func TestCount(t *testing.T) {
	mm := multimap.New[string, int]()
	require.Equal(t, 0, mm.Count("x"))
	mm.Add("x", 1)
	mm.Add("x", 2)
	require.Equal(t, 2, mm.Count("x"))
}

func TestDelete(t *testing.T) {
	mm := multimap.New[string, int]()
	mm.Add("k", 1)
	mm.Add("k", 2)
	mm.Add("other", 3)

	mm.Delete("k")
	require.False(t, mm.Has("k"))
	require.Nil(t, mm.Get("k"))
	require.Equal(t, 1, mm.Len())
}

func TestDeleteValue(t *testing.T) {
	mm := multimap.New[string, int]()
	mm.Add("k", 1)
	mm.Add("k", 2)
	mm.Add("k", 3)
	mm.Add("k", 2)

	// Remove first occurrence of 2.
	require.True(t, multimap.DeleteValue(mm, "k", 2))
	require.Equal(t, []int{1, 3, 2}, mm.Get("k"))

	// Remove remaining 2.
	require.True(t, multimap.DeleteValue(mm, "k", 2))
	require.Equal(t, []int{1, 3}, mm.Get("k"))

	// Value not present.
	require.False(t, multimap.DeleteValue(mm, "k", 99))

	// Key not present.
	require.False(t, multimap.DeleteValue(mm, "missing", 1))

	// Removing the last value deletes the key entirely.
	require.True(t, multimap.DeleteValue(mm, "k", 1))
	require.True(t, multimap.DeleteValue(mm, "k", 3))
	require.False(t, mm.Has("k"))
}

func TestKeysAndLen(t *testing.T) {
	mm := multimap.New[string, int]()
	mm.Add("a", 1)
	mm.Add("b", 2)
	mm.Add("b", 3)
	mm.Add("c", 4)

	require.Equal(t, 3, mm.Len())

	keys := mm.Keys()
	sort.Strings(keys)
	require.Equal(t, []string{"a", "b", "c"}, keys)

	// Keys with empty buckets (via Set nil) are excluded.
	mm.Set("a", nil)
	require.Equal(t, 2, mm.Len())
}

func TestEach(t *testing.T) {
	mm := multimap.New[string, int]()
	mm.Add("a", 1)
	mm.Add("a", 2)
	mm.Add("b", 3)

	seen := map[string][]int{}
	mm.Each(func(k string, v int) bool {
		seen[k] = append(seen[k], v)
		return true
	})
	require.Equal(t, []int{1, 2}, seen["a"])
	require.Equal(t, []int{3}, seen["b"])

	// Early termination: stop after 2 pairs.
	count := 0
	mm.Each(func(string, int) bool {
		count++
		return count < 2
	})
	require.Equal(t, 2, count)
}

func TestClear(t *testing.T) {
	mm := multimap.New[string, int]()
	mm.Add("a", 1)
	mm.Add("b", 2)
	mm.Clear()
	require.Equal(t, 0, mm.Len())
	require.False(t, mm.Has("a"))
}

func TestGetAliasesStorage(t *testing.T) {
	// Documented behavior: Get returns the internal slice (no copy). An in-place
	// mutation of the returned slice is visible through Get again — proving they
	// share the backing array. This pins the contract so a future "safe copy"
	// change is deliberate, not accidental. Callers must copy before mutating.
	mm := multimap.New[string, int]()
	mm.Add("k", 1)
	mm.Add("k", 2)
	internal := mm.Get("k")
	require.Equal(t, []int{1, 2}, internal)

	internal[0] = 999 // in-place index write — no realloc, same backing array
	require.Equal(t, 999, mm.Get("k")[0])
}
