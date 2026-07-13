package set_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/set"
)

func TestNew(t *testing.T) {
	s := set.New(1, 2, 3, 2) // duplicate collapsed
	require.Equal(t, 3, s.Len())
	require.True(t, s.Contains(1))
	require.False(t, s.Contains(4))
}

func TestFrom(t *testing.T) {
	s := set.From([]string{"a", "b", "a"})
	require.Equal(t, 2, s.Len())
}

func TestAddRemove(t *testing.T) {
	s := set.New[int]()
	s.Add(1, 2)
	require.Equal(t, 2, s.Len())
	s.Add(1) // idempotent
	require.Equal(t, 2, s.Len())
	s.Remove(1)
	require.False(t, s.Contains(1))
	s.Remove(99) // absent no-op
	require.Equal(t, 1, s.Len())
}

func TestContainsAll(t *testing.T) {
	s := set.New(1, 2, 3)
	require.True(t, s.ContainsAll(1, 2))
	require.False(t, s.ContainsAll(1, 4))
	require.True(t, s.ContainsAll()) // vacuous
}

func TestContainsAny(t *testing.T) {
	s := set.New(1, 2, 3)
	require.True(t, s.ContainsAny(4, 2))
	require.False(t, s.ContainsAny(4, 5))
	require.False(t, s.ContainsAny()) // vacuous
}

func TestIsEmptyClear(t *testing.T) {
	s := set.New[int]()
	require.True(t, s.IsEmpty())
	s.Add(1)
	require.False(t, s.IsEmpty())
	s.Clear()
	require.True(t, s.IsEmpty())
}

func TestPop(t *testing.T) {
	s := set.New(1, 2)
	v, ok := s.Pop()
	require.True(t, ok)
	require.Contains(t, []int{1, 2}, v)
	require.Equal(t, 1, s.Len())
	_, ok = s.Pop()
	require.True(t, ok)
	_, ok = s.Pop()
	require.False(t, ok)
}

func TestEach(t *testing.T) {
	s := set.New(1, 2, 3)
	seen := map[int]bool{}
	s.Each(func(v int) { seen[v] = true })
	require.Len(t, seen, 3)
}

func TestFilter(t *testing.T) {
	s := set.New(1, 2, 3, 4)
	even := s.Filter(func(v int) bool { return v%2 == 0 })
	require.Equal(t, 2, even.Len())
	require.True(t, even.ContainsAll(2, 4))
	// Original unchanged.
	require.Equal(t, 4, s.Len())
}

func TestToSlice(t *testing.T) {
	s := set.New("a", "b")
	slice := s.ToSlice()
	require.Len(t, slice, 2)
}

func TestClone(t *testing.T) {
	s := set.New(1, 2, 3)
	c := s.Clone()
	s.Remove(1)
	require.Equal(t, 2, s.Len())
	require.Equal(t, 3, c.Len()) // clone independent
	require.True(t, c.Contains(1))
}

func TestUnion(t *testing.T) {
	a := set.New(1, 2)
	b := set.New(2, 3)
	u := set.Union(a, b)
	require.True(t, u.ContainsAll(1, 2, 3))
	require.Equal(t, 3, u.Len())
	// Variadic + nil.
	u2 := set.Union[int](a, nil, b, nil)
	require.Equal(t, 3, u2.Len())
}

func TestIntersect(t *testing.T) {
	a := set.New(1, 2, 3)
	b := set.New(2, 3, 4)
	i := set.Intersect(a, b)
	require.Equal(t, 2, i.Len())
	require.True(t, i.ContainsAll(2, 3))
	// Disjoint → empty.
	require.Equal(t, 0, set.Intersect(set.New(1), set.New(2)).Len())
	// Nil-safe.
	require.True(t, set.Intersect[int](nil, set.New(1)).IsEmpty())
}

func TestDifference(t *testing.T) {
	a := set.New(1, 2, 3)
	b := set.New(2, 4)
	d := set.Difference(a, b)
	require.True(t, d.ContainsAll(1, 3))
	require.False(t, d.Contains(2))
	require.Equal(t, 2, d.Len())
	// Nil b → all of a.
	require.Equal(t, 2, set.Difference(set.New(5, 6), nil).Len())
}

func TestSymmetricDifference(t *testing.T) {
	a := set.New(1, 2, 3)
	b := set.New(2, 3, 4)
	sd := set.SymmetricDifference(a, b)
	require.True(t, sd.ContainsAll(1, 4))
	require.False(t, sd.Contains(2))
	require.Equal(t, 2, sd.Len())
}

func TestIsSubset(t *testing.T) {
	require.True(t, set.IsSubset(set.New(1, 2), set.New(1, 2, 3)))
	require.False(t, set.IsSubset(set.New(1, 4), set.New(1, 2, 3)))
	require.True(t, set.IsSubset(set.New[int](), set.New(1))) // empty is subset
	require.True(t, set.IsSubset[int](nil, set.New(1)))       // nil is subset
}

func TestIsSuperset(t *testing.T) {
	require.True(t, set.IsSuperset(set.New(1, 2, 3), set.New(1, 2)))
	require.False(t, set.IsSuperset(set.New(1, 2), set.New(1, 2, 3)))
}

func TestIsDisjoint(t *testing.T) {
	require.True(t, set.IsDisjoint(set.New(1, 2), set.New(3, 4)))
	require.False(t, set.IsDisjoint(set.New(1, 2), set.New(2, 3)))
	require.True(t, set.IsDisjoint(set.New[int](), set.New(1))) // empty is disjoint
	// Swap branch: b smaller than a → iterate b as the small set.
	require.True(t, set.IsDisjoint(set.New(1, 2, 3, 4), set.New(5, 6)))
}

func TestEqual(t *testing.T) {
	require.True(t, set.Equal(set.New(1, 2), set.New(2, 1)))
	require.False(t, set.Equal(set.New(1, 2), set.New(1, 2, 3)))
	require.True(t, set.Equal(set.New[int](), set.New[int]()))
	require.False(t, set.Equal[int](nil, set.New(1)))
	require.True(t, set.Equal[int](nil, nil))
	require.False(t, set.Equal(set.New(1), nil))              // a non-nil, b nil
	require.False(t, set.Equal(set.New(1, 2), set.New(1, 3))) // same len, diff elements
}

// --- nil-safe coverage (100% gap) ---

func TestDifferenceNilA(t *testing.T) {
	require.True(t, set.Difference[int](nil, set.New(1)).IsEmpty())
}

func TestIsSubsetNilSup(t *testing.T) {
	require.False(t, set.IsSubset(set.New(1), nil))    // non-empty sub, nil sup → false
	require.True(t, set.IsSubset(set.New[int](), nil)) // empty sub, nil sup → true
}

func TestIsDisjointNil(t *testing.T) {
	require.True(t, set.IsDisjoint[int](nil, set.New(1)))
	require.True(t, set.IsDisjoint(set.New(1), nil))
}

// --- in-place mutation ---

func TestAddAll(t *testing.T) {
	a := set.New(1, 2)
	b := set.New(2, 3, 4)
	a.AddAll(b)
	require.True(t, a.ContainsAll(1, 2, 3, 4))
	require.Equal(t, 4, a.Len())
	// nil safe.
	a.AddAll(nil)
	require.Equal(t, 4, a.Len())
}

func TestRetainAll(t *testing.T) {
	a := set.New(1, 2, 3, 4)
	b := set.New(2, 4, 6)
	removed := a.RetainAll(b)
	require.Equal(t, 2, removed) // 1 and 3 removed
	require.True(t, a.ContainsAll(2, 4))
	require.False(t, a.Contains(1))
	// nil other → all removed.
	a2 := set.New(1, 2)
	removed2 := a2.RetainAll(nil)
	require.Equal(t, 2, removed2)
	require.True(t, a2.IsEmpty())
}

func TestRemoveAll(t *testing.T) {
	a := set.New(1, 2, 3, 4)
	b := set.New(2, 4)
	removed := a.RemoveAll(b)
	require.Equal(t, 2, removed)
	require.True(t, a.ContainsAll(1, 3))
	require.False(t, a.Contains(2))
	// nil other → nothing removed.
	a2 := set.New(1, 2)
	require.Equal(t, 0, a2.RemoveAll(nil))
	require.Equal(t, 2, a2.Len())
}

func TestWithCapacity(t *testing.T) {
	s := set.WithCapacity[int](100)
	require.Equal(t, 0, s.Len())
	s.Add(1, 2, 3)
	require.Equal(t, 3, s.Len())
}

// --- benchmarks ---

func BenchmarkSetAdd(b *testing.B) {
	s := set.New[int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			s.Add(i)
			i++
		}
	})
}

func BenchmarkSetContains(b *testing.B) {
	s := set.New[int]()
	for i := range 1000 {
		s.Add(i)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Contains(500)
		}
	})
}

func BenchmarkSetUnion(b *testing.B) {
	a := set.New[int]()
	c := set.New[int]()
	for i := range 500 {
		a.Add(i)
		c.Add(i + 250)
	}
	b.ResetTimer()
	for b.Loop() {
		set.Union(a, c)
	}
}

func BenchmarkSetIntersect(b *testing.B) {
	a := set.New[int]()
	c := set.New[int]()
	for i := range 500 {
		a.Add(i)
		c.Add(i + 250)
	}
	b.ResetTimer()
	for b.Loop() {
		set.Intersect(a, c)
	}
}

func BenchmarkSetDifference(b *testing.B) {
	a := set.New[int]()
	c := set.New[int]()
	for i := range 500 {
		a.Add(i)
		c.Add(i + 250)
	}
	b.ResetTimer()
	for b.Loop() {
		set.Difference(a, c)
	}
}

func BenchmarkSetAddAll(b *testing.B) {
	a := set.New[int]()
	c := set.New[int]()
	for i := range 500 {
		a.Add(i)
		c.Add(i + 250)
	}
	b.ResetTimer()
	for b.Loop() {
		dst := set.WithCapacity[int](1000)
		dst.AddAll(a)
		dst.AddAll(c)
	}
}

func BenchmarkSetRetainAll(b *testing.B) {
	a := set.New[int]()
	c := set.New[int]()
	for i := range 500 {
		a.Add(i)
		c.Add(i + 250)
	}
	b.ResetTimer()
	for b.Loop() {
		clone := a.Clone()
		clone.RetainAll(c)
	}
}

func BenchmarkSetRemoveAll(b *testing.B) {
	a := set.New[int]()
	c := set.New[int]()
	for i := range 500 {
		a.Add(i)
		c.Add(i + 250)
	}
	b.ResetTimer()
	for b.Loop() {
		clone := a.Clone()
		clone.RemoveAll(c)
	}
}
