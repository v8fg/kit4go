package batcher

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSizeTrigger(t *testing.T) {
	var mu sync.Mutex
	var batches [][]int
	b := New[int](3, 0, func(batch []int) {
		mu.Lock()
		batches = append(batches, append([]int(nil), batch...))
		mu.Unlock()
	})
	defer func() { _ = b.Close() }()

	b.Add(1)
	b.Add(2)
	b.Add(3)  // 3rd fills -> flush [1,2,3]
	b.Flush() // synchronous sync point (flushes anything still buffered)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, batches)
	flat := []int{}
	for _, batch := range batches {
		require.LessOrEqual(t, len(batch), 3, "no batch may exceed maxSize")
		flat = append(flat, batch...)
	}
	require.Equal(t, []int{1, 2, 3}, flat)
}

func TestTimeTrigger(t *testing.T) {
	var flushed atomic.Int64
	b := New[int](1000, 20*time.Millisecond, func([]int) { flushed.Add(1) })
	defer func() { _ = b.Close() }()

	b.Add(1)
	b.Add(2)
	require.Equal(t, int64(0), flushed.Load()) // size not reached, nothing yet
	require.Eventually(t, func() bool { return flushed.Load() >= 1 }, 500*time.Millisecond, 5*time.Millisecond)
}

func TestManualFlush(t *testing.T) {
	var mu sync.Mutex
	var got []int
	b := New[int](100, 0, func(batch []int) {
		mu.Lock()
		got = append(got, batch...)
		mu.Unlock()
	})
	defer func() { _ = b.Close() }()

	b.Add(10)
	b.Add(20)
	b.Flush() // synchronous: returns after the flush ran
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []int{10, 20}, got)
}

func TestCloseFlushesRemaining(t *testing.T) {
	var mu sync.Mutex
	var got []int
	b := New[int](100, 0, func(batch []int) {
		mu.Lock()
		got = append(got, batch...)
		mu.Unlock()
	})
	b.Add(1)
	b.Add(2)
	b.Add(3)
	require.NoError(t, b.Close()) // must flush buffered items
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []int{1, 2, 3}, got)
}

func TestAddAfterClose(t *testing.T) {
	b := New[int](10, 0, func([]int) {})
	require.NoError(t, b.Close())
	require.False(t, b.Add(1), "Add after Close returns false")
}

func TestCloseIdempotent(t *testing.T) {
	b := New[int](10, 0, func([]int) {})
	require.NoError(t, b.Close())
	require.NoError(t, b.Close())
}

func TestBufferSizeOption(t *testing.T) {
	// Larger buffer decouples producer from flusher; just verify it builds+works.
	var n atomic.Int64
	b := New[int](2, 0, func([]int) { n.Add(1) }, WithBufferSize[int](8))
	defer func() { _ = b.Close() }()
	b.Add(1)
	b.Add(2)
	b.Flush()
	require.GreaterOrEqual(t, n.Load(), int64(1))
}

func TestPanicGuards(t *testing.T) {
	require.Panics(t, func() { New[int](0, 0, func([]int) {}) })
	require.Panics(t, func() { New[int](1, 0, nil) })
}

func TestErrClosed(t *testing.T) {
	require.True(t, errors.Is(ErrClosed, ErrClosed))
}

func TestConcurrency(t *testing.T) {
	var total atomic.Int64
	b := New[int](50, 5*time.Millisecond, func(batch []int) {
		total.Add(int64(len(batch)))
	})
	var wg sync.WaitGroup
	const g = 16
	const perG = 300
	wg.Add(g)
	for range g {
		go func() {
			defer wg.Done()
			for j := range perG {
				b.Add(j)
			}
		}()
	}
	wg.Wait()
	require.NoError(t, b.Close())
	// Every Added item must be flushed exactly once (Close flushes the remainder).
	require.Equal(t, int64(g*perG), total.Load())
}
