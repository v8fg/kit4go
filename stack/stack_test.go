package stack_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/stack"
)

func TestNew(t *testing.T) {
	s := stack.New(1, 2, 3) // 3 on top
	require.Equal(t, 3, s.Len())
	v, ok := s.Peek()
	require.True(t, ok)
	require.Equal(t, 3, v)
}

func TestPushPop(t *testing.T) {
	s := stack.New[int]()
	s.Push(10)
	s.Push(20)
	v, ok := s.Pop()
	require.True(t, ok)
	require.Equal(t, 20, v) // LIFO
	require.Equal(t, 1, s.Len())
}

func TestPopEmpty(t *testing.T) {
	s := stack.New[string]()
	_, ok := s.Pop()
	require.False(t, ok)
}

func TestPeekEmpty(t *testing.T) {
	s := stack.New[int]()
	_, ok := s.Peek()
	require.False(t, ok)
}

func TestPeekNoRemove(t *testing.T) {
	s := stack.New(1, 2)
	v, _ := s.Peek()
	require.Equal(t, 2, v)
	require.Equal(t, 2, s.Len()) // peek doesn't remove
}

func TestIsEmpty(t *testing.T) {
	s := stack.New[int]()
	require.True(t, s.IsEmpty())
	s.Push(1)
	require.False(t, s.IsEmpty())
}

func TestClear(t *testing.T) {
	s := stack.New(1, 2, 3)
	s.Clear()
	require.True(t, s.IsEmpty())
	s.Push(99)
	require.Equal(t, 1, s.Len())
}

func TestToSlice(t *testing.T) {
	s := stack.New(1, 2, 3)
	require.Equal(t, []int{1, 2, 3}, s.ToSlice()) // bottom → top
	// ToSlice returns a copy.
	slc := s.ToSlice()
	slc[0] = 99
	require.Equal(t, 1, s.ToSlice()[0]) // original unchanged
}

func TestZeroValue(t *testing.T) {
	var s stack.Stack[int]
	require.True(t, s.IsEmpty())
	s.Push(1)
	v, ok := s.Pop()
	require.True(t, ok)
	require.Equal(t, 1, v)
}

// --- benchmarks ---

func BenchmarkStackPush(b *testing.B) {
	s := stack.New[int]()
	b.ResetTimer()
	for b.Loop() {
		s.Push(1)
	}
}

func BenchmarkStackPop(b *testing.B) {
	s := stack.New[int]()
	for range 100000 {
		s.Push(1)
	}
	b.ResetTimer()
	for b.Loop() {
		s.Pop()
	}
}

func TestWithCapacity(t *testing.T) {
	s := stack.WithCapacity[int](100)
	require.True(t, s.IsEmpty())
	s.Push(1)
	s.Push(2)
	require.Equal(t, 2, s.Len())
}
