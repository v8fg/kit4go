package bimap_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/bimap"
)

func TestNew(t *testing.T) {
	bm := bimap.New[int, string]()
	require.Equal(t, 0, bm.Len())
}

func TestInsertGet(t *testing.T) {
	bm := bimap.New[int, string]()
	require.NoError(t, bm.Insert(1, "one"))
	require.NoError(t, bm.Insert(2, "two"))

	v, ok := bm.Get(1)
	require.True(t, ok)
	require.Equal(t, "one", v)

	k, ok := bm.GetKey("two")
	require.True(t, ok)
	require.Equal(t, 2, k)

	_, ok = bm.Get(99)
	require.False(t, ok)

	_, ok = bm.GetKey("missing")
	require.False(t, ok)
}

func TestInsertDuplicateKey(t *testing.T) {
	bm := bimap.New[int, string]()
	bm.Insert(1, "one")
	err := bm.Insert(1, "different")
	require.ErrorIs(t, err, bimap.ErrDuplicateKey)
	// Original is untouched.
	v, _ := bm.Get(1)
	require.Equal(t, "one", v)
}

func TestInsertDuplicateValue(t *testing.T) {
	bm := bimap.New[int, string]()
	bm.Insert(1, "one")
	err := bm.Insert(2, "one")
	require.ErrorIs(t, err, bimap.ErrDuplicateValue)
	require.False(t, bm.HasKey(2))
}

func TestMustInsert(t *testing.T) {
	bm := bimap.New[int, string]()
	bm.MustInsert(1, "a")
	require.Panics(t, func() { bm.MustInsert(1, "b") })
}

func TestFromMap(t *testing.T) {
	bm, err := bimap.FromMap(map[int]string{1: "a", 2: "b"})
	require.NoError(t, err)
	require.Equal(t, 2, bm.Len())

	// Duplicate value → error.
	_, err = bimap.FromMap(map[int]string{1: "a", 2: "a"})
	require.ErrorIs(t, err, bimap.ErrDuplicateValue)
}

func TestHasKeyHasValue(t *testing.T) {
	bm := bimap.New[int, string]()
	bm.Insert(1, "one")
	require.True(t, bm.HasKey(1))
	require.False(t, bm.HasKey(2))
	require.True(t, bm.HasValue("one"))
	require.False(t, bm.HasValue("two"))
}

func TestDelete(t *testing.T) {
	bm := bimap.New[int, string]()
	bm.Insert(1, "one")
	require.True(t, bm.Delete(1))
	require.False(t, bm.HasKey(1))
	require.False(t, bm.HasValue("one"))
	require.False(t, bm.Delete(99)) // absent
}

func TestDeleteValue(t *testing.T) {
	bm := bimap.New[int, string]()
	bm.Insert(1, "one")
	require.True(t, bm.DeleteValue("one"))
	require.False(t, bm.HasKey(1))
	require.False(t, bm.DeleteValue("absent"))
}

func TestKeysValues(t *testing.T) {
	bm := bimap.New[int, string]()
	bm.Insert(1, "a")
	bm.Insert(2, "b")
	keys := bm.Keys()
	require.ElementsMatch(t, []int{1, 2}, keys)
	vals := bm.Values()
	require.ElementsMatch(t, []string{"a", "b"}, vals)
}

func TestClear(t *testing.T) {
	bm := bimap.New[int, string]()
	bm.Insert(1, "a")
	bm.Clear()
	require.Equal(t, 0, bm.Len())
	bm.Insert(1, "a") // reusable after clear
	require.Equal(t, 1, bm.Len())
}
