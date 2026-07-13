package interval_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/interval"
)

func TestNew(t *testing.T) {
	i, err := interval.New(1, 5)
	require.NoError(t, err)
	require.Equal(t, 1, i.Start)
	require.Equal(t, 5, i.End)

	_, err = interval.New(5, 1)
	require.ErrorIs(t, err, interval.ErrInverted)

	_, err = interval.New(5, 5) // empty
	require.ErrorIs(t, err, interval.ErrInverted)
}

func TestMustNew(t *testing.T) {
	require.Panics(t, func() { interval.MustNew(5, 1) })
	i := interval.MustNew(1, 5)
	require.Equal(t, 1, i.Start)
}

func TestContains(t *testing.T) {
	i := interval.MustNew(1, 5)     // [1, 5)
	require.True(t, i.Contains(1))  // inclusive start
	require.True(t, i.Contains(4))  // inside
	require.False(t, i.Contains(5)) // exclusive end
	require.False(t, i.Contains(0)) // before
}

func TestContainsInclusive(t *testing.T) {
	i := interval.MustNew(1, 5)
	require.True(t, i.ContainsInclusive(5)) // inclusive end
	require.True(t, i.ContainsInclusive(1))
	require.False(t, i.ContainsInclusive(6))
}

func TestOverlaps(t *testing.T) {
	i := interval.MustNew(1, 5)
	require.True(t, i.Overlaps(interval.MustNew(3, 8)))
	require.True(t, i.Overlaps(interval.MustNew(0, 2)))
	require.False(t, i.Overlaps(interval.MustNew(5, 8))) // adjacent, half-open → no overlap
	require.False(t, i.Overlaps(interval.MustNew(6, 9)))
}

func TestIsBeforeIsAfter(t *testing.T) {
	i := interval.MustNew(1, 5)
	require.True(t, i.IsBefore(interval.MustNew(5, 8)))
	require.False(t, i.IsBefore(interval.MustNew(3, 8)))
	require.True(t, i.IsAfter(interval.MustNew(0, 1)))
	require.False(t, i.IsAfter(interval.MustNew(3, 8)))
}

func TestUnion(t *testing.T) {
	i := interval.MustNew(1, 5)
	u, ok := i.Union(interval.MustNew(3, 8))
	require.True(t, ok)
	require.Equal(t, 1, u.Start)
	require.Equal(t, 8, u.End)

	// Adjacent → merged (touching).
	u2, ok2 := i.Union(interval.MustNew(5, 8))
	require.True(t, ok2)
	require.Equal(t, 8, u2.End)

	// Gap → no union.
	_, ok3 := i.Union(interval.MustNew(10, 15))
	require.False(t, ok3)
}

func TestIntersect(t *testing.T) {
	i := interval.MustNew(1, 10)
	in, ok := i.Intersect(interval.MustNew(5, 15))
	require.True(t, ok)
	require.Equal(t, 5, in.Start)
	require.Equal(t, 10, in.End)

	// No overlap.
	_, ok2 := i.Intersect(interval.MustNew(20, 30))
	require.False(t, ok2)

	// Contained.
	in3, ok3 := i.Intersect(interval.MustNew(3, 7))
	require.True(t, ok3)
	require.Equal(t, 3, in3.Start)
	require.Equal(t, 7, in3.End)
}

func TestMerge(t *testing.T) {
	intervals := []interval.Interval[int]{
		interval.MustNew(1, 4),
		interval.MustNew(3, 6),
		interval.MustNew(8, 10),
		interval.MustNew(9, 12),
		interval.MustNew(15, 20),
	}
	merged := interval.Merge(intervals)
	require.Len(t, merged, 3)
	require.Equal(t, interval.MustNew(1, 6), merged[0])
	require.Equal(t, interval.MustNew(8, 12), merged[1])
	require.Equal(t, interval.MustNew(15, 20), merged[2])
}

func TestMergeEmpty(t *testing.T) {
	require.Nil(t, interval.Merge[int](nil))
}

func TestMergeSingle(t *testing.T) {
	merged := interval.Merge([]interval.Interval[int]{interval.MustNew(1, 5)})
	require.Len(t, merged, 1)
}
