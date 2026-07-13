package bitset_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/bitset"
)

func TestSetTest(t *testing.T) {
	bs := bitset.New(128)
	bs.Set(5)
	bs.Set(100)
	require.True(t, bs.Test(5))
	require.True(t, bs.Test(100))
	require.False(t, bs.Test(6))
	require.False(t, bs.Test(200)) // out of range
}

func TestClear(t *testing.T) {
	bs := bitset.New(64)
	bs.Set(10)
	require.True(t, bs.Test(10))
	bs.Clear(10)
	require.False(t, bs.Test(10))
}

func TestLen(t *testing.T) {
	bs := bitset.New(128)
	bs.Set(1)
	bs.Set(64)
	bs.Set(127)
	require.Equal(t, 3, bs.Len())
}

func TestIsEmpty(t *testing.T) {
	bs := bitset.New(64)
	require.True(t, bs.IsEmpty())
	bs.Set(0)
	require.False(t, bs.IsEmpty())
}

func TestClearAll(t *testing.T) {
	bs := bitset.New(64)
	bs.Set(1)
	bs.Set(2)
	bs.ClearAll()
	require.True(t, bs.IsEmpty())
}

func TestGrow(t *testing.T) {
	bs := bitset.New(64)
	bs.Set(1000) // grows
	require.True(t, bs.Test(1000))
	require.Equal(t, 1, bs.Len())
}

func TestUnion(t *testing.T) {
	a := bitset.New(128)
	a.Set(1)
	a.Set(3)
	b := bitset.New(128)
	b.Set(2)
	b.Set(3)
	a.Union(b)
	require.True(t, a.Test(1))
	require.True(t, a.Test(2))
	require.True(t, a.Test(3))
	require.Equal(t, 3, a.Len())
}

func TestIntersect(t *testing.T) {
	a := bitset.New(128)
	a.Set(1)
	a.Set(3)
	b := bitset.New(128)
	b.Set(2)
	b.Set(3)
	a.Intersect(b)
	require.False(t, a.Test(1))
	require.False(t, a.Test(2))
	require.True(t, a.Test(3))
	require.Equal(t, 1, a.Len())
}

func TestToSlice(t *testing.T) {
	bs := bitset.New(128)
	bs.Set(0)
	bs.Set(63)
	bs.Set(64)
	bs.Set(127)
	require.Equal(t, []int{0, 63, 64, 127}, bs.ToSlice())
}

func TestNegativePanic(t *testing.T) {
	bs := bitset.New(64)
	require.Panics(t, func() { bs.Set(-1) })
}

func TestNewDefault(t *testing.T) {
	bs := bitset.New(0) // defaults to 64
	bs.Set(5)
	require.True(t, bs.Test(5))
}

func TestClearOutOfRange(t *testing.T) {
	bs := bitset.New(64)
	bs.Set(5)
	bs.Clear(100) // out of range → no-op
	bs.Clear(-1)  // negative → no-op (doesn't panic, unlike Set)
	require.True(t, bs.Test(5))
}

func TestUnionNil(t *testing.T) {
	bs := bitset.New(64)
	bs.Set(1)
	bs.Union(nil) // no-op
	require.True(t, bs.Test(1))
}

func TestUnionGrow(t *testing.T) {
	a := bitset.New(64)
	b := bitset.New(256)
	b.Set(200)
	a.Union(b)
	require.True(t, a.Test(200))
}

func TestIntersectNil(t *testing.T) {
	bs := bitset.New(64)
	bs.Set(1)
	bs.Set(2)
	bs.Intersect(nil) // clears all
	require.True(t, bs.IsEmpty())
}

func TestIntersectShorterOther(t *testing.T) {
	a := bitset.New(128)
	a.Set(1)
	a.Set(64)
	b := bitset.New(64) // shorter than a
	b.Set(1)
	a.Intersect(b)
	require.True(t, a.Test(1))
	require.False(t, a.Test(64)) // cleared (b doesn't have word 1)
}

func TestGrowSameWord(t *testing.T) {
	bs := bitset.New(128)
	bs.Set(50) // same word, no grow needed
	require.True(t, bs.Test(50))
}

func TestGrowExtendN(t *testing.T) {
	bs := bitset.New(64) // n=64, 1 word
	// Set a bit within the existing word but beyond current n — grow() should
	// just extend n without allocating a new word.
	bs.Set(63) // within word 0, but n was 64 so 63 < n → no grow needed
	require.True(t, bs.Test(63))
	// Now set 64 — needs word 1, triggers grow with new allocation.
	bs.Set(64)
	require.True(t, bs.Test(64))
}

func TestGrowExtendNOnly(t *testing.T) {
	// New(100): creates 2 words (covers 0-127), n=100.
	// Set(120): word already exists (120/64=1 < 2), but 120 >= n(100) → extend n.
	bs := bitset.New(100)
	bs.Set(120)
	require.True(t, bs.Test(120))
}
