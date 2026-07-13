package optional_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/optional"
)

func TestSomeNone(t *testing.T) {
	s := optional.Some(42)
	require.True(t, s.IsSome())
	require.False(t, s.IsNone())

	n := optional.None[int]()
	require.False(t, n.IsSome())
	require.True(t, n.IsNone())
}

func TestGet(t *testing.T) {
	s := optional.Some("hello")
	v, ok := s.Get()
	require.True(t, ok)
	require.Equal(t, "hello", v)

	n := optional.None[string]()
	_, ok2 := n.Get()
	require.False(t, ok2)
}

func TestUnwrap(t *testing.T) {
	require.Equal(t, 42, optional.Some(42).Unwrap())
	require.Panics(t, func() { optional.None[int]().Unwrap() })
}

func TestUnwrapOr(t *testing.T) {
	require.Equal(t, 42, optional.Some(42).UnwrapOr(99))
	require.Equal(t, 99, optional.None[int]().UnwrapOr(99))
}

func TestUnwrapOrElse(t *testing.T) {
	require.Equal(t, 42, optional.Some(42).UnwrapOrElse(func() int { return 99 }))
	require.Equal(t, 7, optional.None[int]().UnwrapOrElse(func() int { return 7 }))
}

func TestUnwrapOrZero(t *testing.T) {
	require.Equal(t, 42, optional.Some(42).UnwrapOrZero())
	require.Equal(t, 0, optional.None[int]().UnwrapOrZero())
	require.Equal(t, "", optional.None[string]().UnwrapOrZero())
}

func TestFromPtr(t *testing.T) {
	v := 10
	require.True(t, optional.FromPtr(&v).IsSome())
	require.Equal(t, 10, optional.FromPtr(&v).Unwrap())

	require.True(t, optional.FromPtr[int](nil).IsNone())
}

func TestToPtr(t *testing.T) {
	p := optional.Some(5).ToPtr()
	require.NotNil(t, p)
	require.Equal(t, 5, *p)

	require.Nil(t, optional.None[int]().ToPtr())
}

func TestMap(t *testing.T) {
	doubled := optional.Map(optional.Some(21), func(x int) int { return x * 2 })
	require.True(t, doubled.IsSome())
	require.Equal(t, 42, doubled.Unwrap())

	none := optional.Map(optional.None[int](), func(x int) int { return x * 2 })
	require.True(t, none.IsNone())
}

func TestMapOr(t *testing.T) {
	require.Equal(t, 42, optional.MapOr(optional.Some(21), 0, func(x int) int { return x * 2 }))
	require.Equal(t, 0, optional.MapOr(optional.None[int](), 0, func(x int) int { return x * 2 }))
}

func TestAndThen(t *testing.T) {
	parseHex := optional.AndThen(optional.Some("ff"), func(s string) optional.Option[int] {
		if s == "ff" {
			return optional.Some(255)
		}
		return optional.None[int]()
	})
	require.True(t, parseHex.IsSome())
	require.Equal(t, 255, parseHex.Unwrap())

	none := optional.AndThen(optional.None[string](), func(s string) optional.Option[int] {
		return optional.Some(0)
	})
	require.True(t, none.IsNone())
}

func TestEqual(t *testing.T) {
	eqInt := func(a, b int) bool { return a == b }
	require.True(t, optional.Equal(optional.Some(1), optional.Some(1), eqInt))
	require.False(t, optional.Equal(optional.Some(1), optional.Some(2), eqInt))
	require.True(t, optional.Equal(optional.None[int](), optional.None[int](), eqInt))
	require.False(t, optional.Equal(optional.Some(1), optional.None[int](), eqInt))
}

func TestZeroValue(t *testing.T) {
	var o optional.Option[int]
	require.True(t, o.IsNone())
	require.False(t, o.IsSome())
}
