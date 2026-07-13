package ringbuf_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/ringbuf"
)

func TestPushOverwrite(t *testing.T) {
	c := ringbuf.New[int](3)
	c.Push(1)
	c.Push(2)
	c.Push(3)
	c.Push(4) // overwrites 1
	require.Equal(t, 3, c.Len())
	require.Equal(t, []int{2, 3, 4}, c.ToSlice())
}

func TestPop(t *testing.T) {
	c := ringbuf.New[int](3)
	c.Push(10)
	c.Push(20)
	v, err := c.Pop()
	require.NoError(t, err)
	require.Equal(t, 10, v)
	require.Equal(t, 1, c.Len())
}

func TestPopEmpty(t *testing.T) {
	c := ringbuf.New[int](3)
	_, err := c.Pop()
	require.ErrorIs(t, err, ringbuf.ErrEmpty)
}

func TestAt(t *testing.T) {
	c := ringbuf.New[int](3)
	c.Push(10)
	c.Push(20)
	c.Push(30)
	v, _ := c.At(0)
	require.Equal(t, 10, v)
	v, _ = c.At(2)
	require.Equal(t, 30, v)
	_, err := c.At(3)
	require.ErrorIs(t, err, ringbuf.ErrEmpty)
}

func TestLatest(t *testing.T) {
	c := ringbuf.New[int](3)
	c.Push(1)
	c.Push(2)
	v, _ := c.Latest()
	require.Equal(t, 2, v)
}

func TestLatestEmpty(t *testing.T) {
	c := ringbuf.New[int](3)
	_, err := c.Latest()
	require.ErrorIs(t, err, ringbuf.ErrEmpty)
}

func TestIsFullIsEmpty(t *testing.T) {
	c := ringbuf.New[int](2)
	require.True(t, c.IsEmpty())
	c.Push(1)
	require.False(t, c.IsFull())
	c.Push(2)
	require.True(t, c.IsFull())
}

func TestClear(t *testing.T) {
	c := ringbuf.New[int](3)
	c.Push(1)
	c.Clear()
	require.True(t, c.IsEmpty())
	c.Push(99)
	require.Equal(t, 1, c.Len())
}

func TestToSlice(t *testing.T) {
	c := ringbuf.New[int](3)
	c.Push(1)
	c.Push(2)
	c.Push(3)
	c.Push(4) // overwrites 1
	require.Equal(t, []int{2, 3, 4}, c.ToSlice())
}

func TestWrapAround(t *testing.T) {
	c := ringbuf.New[int](3)
	for i := range 10 {
		c.Push(i)
	}
	require.Equal(t, 3, c.Len())
	require.Equal(t, []int{7, 8, 9}, c.ToSlice())
}

func TestCap(t *testing.T) {
	c := ringbuf.New[int](5)
	require.Equal(t, 5, c.Cap())
}

func TestZeroValue(t *testing.T) {
	require.Panics(t, func() { ringbuf.New[int](0) })
}
