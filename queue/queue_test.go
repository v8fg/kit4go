package queue_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/queue"
)

func TestNew(t *testing.T) {
	q := queue.New(1, 2, 3)
	require.Equal(t, 3, q.Len())
	v, _ := q.Front()
	require.Equal(t, 1, v) // front = first in
}

func TestEnqueueDequeue(t *testing.T) {
	q := queue.New[int]()
	q.Enqueue(10)
	q.Enqueue(20)
	v, ok := q.Dequeue()
	require.True(t, ok)
	require.Equal(t, 10, v) // FIFO
	v, _ = q.Dequeue()
	require.Equal(t, 20, v)
	require.True(t, q.IsEmpty())
}

func TestDequeueEmpty(t *testing.T) {
	q := queue.New[string]()
	_, ok := q.Dequeue()
	require.False(t, ok)
}

func TestFrontBack(t *testing.T) {
	q := queue.New(1, 2, 3)
	f, _ := q.Front()
	b, _ := q.Back()
	require.Equal(t, 1, f)
	require.Equal(t, 3, b)
	// Don't remove.
	require.Equal(t, 3, q.Len())
}

func TestFrontBackEmpty(t *testing.T) {
	q := queue.New[int]()
	_, fok := q.Front()
	_, bok := q.Back()
	require.False(t, fok)
	require.False(t, bok)
}

func TestIsEmptyClear(t *testing.T) {
	q := queue.New(1)
	require.False(t, q.IsEmpty())
	q.Clear()
	require.True(t, q.IsEmpty())
	q.Enqueue(99)
	require.Equal(t, 1, q.Len())
}

func TestToSlice(t *testing.T) {
	q := queue.New(1, 2, 3)
	require.Equal(t, []int{1, 2, 3}, q.ToSlice())
	// ToSlice is a copy.
	s := q.ToSlice()
	s[0] = 99
	require.Equal(t, 1, q.ToSlice()[0])
}

func TestCompaction(t *testing.T) {
	q := queue.New[int]()
	for i := range 100 {
		q.Enqueue(i)
	}
	for range 60 {
		q.Dequeue()
	}
	require.Equal(t, 40, q.Len())
	// Front should be 60 after 60 dequeues.
	v, _ := q.Front()
	require.Equal(t, 60, v)
}

func TestZeroValue(t *testing.T) {
	var q queue.Queue[int]
	require.True(t, q.IsEmpty())
	q.Enqueue(1)
	v, ok := q.Dequeue()
	require.True(t, ok)
	require.Equal(t, 1, v)
}

func TestFIFOOrder(t *testing.T) {
	q := queue.New[int]()
	for i := range 5 {
		q.Enqueue(i)
	}
	var got []int
	for !q.IsEmpty() {
		v, _ := q.Dequeue()
		got = append(got, v)
	}
	require.Equal(t, []int{0, 1, 2, 3, 4}, got)
}

// --- benchmarks ---

func BenchmarkQueueEnqueue(b *testing.B) {
	q := queue.New[int]()
	b.ResetTimer()
	for b.Loop() {
		q.Enqueue(1)
	}
}

func BenchmarkQueueDequeue(b *testing.B) {
	q := queue.New[int]()
	for range 100000 {
		q.Enqueue(1)
	}
	b.ResetTimer()
	for b.Loop() {
		q.Dequeue()
	}
}
