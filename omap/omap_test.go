package omap_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/omap"
)

func TestSetAndGet(t *testing.T) {
	m := omap.New[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("c", 3)

	v, ok := m.Get("b")
	require.True(t, ok)
	require.Equal(t, 2, v)

	_, ok = m.Get("missing")
	require.False(t, ok)
	require.True(t, m.Has("a"))
	require.False(t, m.Has("z"))
}

func TestInsertionOrder(t *testing.T) {
	m := omap.New[string, int]()
	m.Set("c", 3)
	m.Set("a", 1)
	m.Set("b", 2)
	require.Equal(t, []string{"c", "a", "b"}, m.Keys())
	require.Equal(t, []int{3, 1, 2}, m.Values())
}

func TestUpdateKeepsPosition(t *testing.T) {
	m := omap.New[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("c", 3)

	m.Set("a", 99) // update — position unchanged
	require.Equal(t, []string{"a", "b", "c"}, m.Keys())
	v, _ := m.Get("a")
	require.Equal(t, 99, v)
}

func TestDelete(t *testing.T) {
	m := omap.New[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("c", 3)

	require.True(t, m.Delete("b"))
	require.False(t, m.Has("b"))
	require.Equal(t, []string{"a", "c"}, m.Keys(), "order preserved after delete")
	require.Equal(t, 2, m.Len())

	// Delete absent key.
	require.False(t, m.Delete("zzz"))
}

func TestDeleteFirstAndLast(t *testing.T) {
	m := omap.New[int, int]()
	m.Set(1, 10)
	m.Set(2, 20)
	m.Set(3, 30)

	require.True(t, m.Delete(1))
	require.Equal(t, []int{2, 3}, m.Keys())

	require.True(t, m.Delete(3))
	require.Equal(t, []int{2}, m.Keys())
}

func TestLenAndEmpty(t *testing.T) {
	m := omap.New[string, int]()
	require.Equal(t, 0, m.Len())
	require.Empty(t, m.Keys())
	require.Empty(t, m.Values())

	m.Set("x", 1)
	require.Equal(t, 1, m.Len())
}

func TestEach(t *testing.T) {
	m := omap.New[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("c", 3)

	var keys []string
	var vals []int
	m.Each(func(k string, v int) bool {
		keys = append(keys, k)
		vals = append(vals, v)
		return true
	})
	require.Equal(t, []string{"a", "b", "c"}, keys)
	require.Equal(t, []int{1, 2, 3}, vals)

	// Early termination.
	seen := 0
	m.Each(func(string, int) bool {
		seen++
		return seen < 2
	})
	require.Equal(t, 2, seen)
}

func TestClear(t *testing.T) {
	m := omap.New[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Clear()
	require.Equal(t, 0, m.Len())
	require.False(t, m.Has("a"))
	require.Empty(t, m.Keys())

	// Reusable after clear.
	m.Set("z", 9)
	require.Equal(t, []string{"z"}, m.Keys())
}

func TestKeysValuesAreCopies(t *testing.T) {
	m := omap.New[int, int]()
	m.Set(1, 10)
	m.Set(2, 20)
	keys := m.Keys()
	keys[0] = 999
	require.Equal(t, []int{1, 2}, m.Keys(), "mutating returned slice must not affect the map")
}

func TestDeleteAllThenIterate(t *testing.T) {
	m := omap.New[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Delete("a")
	m.Delete("b")
	count := 0
	m.Each(func(string, int) bool { count++; return true })
	require.Equal(t, 0, count)
}
