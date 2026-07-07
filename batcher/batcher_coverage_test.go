package batcher

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFlushAfterClose covers Flush's two <-b.done branches: after Close, Flush
// is a no-op (returns immediately without sending to flushCh or waiting on ack).
func TestFlushAfterClose(t *testing.T) {
	b := New[int](10, 0, func([]int) {})
	require.NoError(t, b.Close())
	// Must return promptly (does not block on the unbuffered flushCh send).
	done := make(chan struct{})
	go func() {
		b.Flush()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Flush after Close blocked")
	}
}

// TestAddRacesClose covers Add's <-b.done branch inside the buffered-send
// select (the path where Close fires while Add is blocked sending). We make Add
// block by filling the buffer, then Close while a blocked Add is in flight.
func TestAddRacesClose(t *testing.T) {
	var flushed atomic.Int64
	// Buffer cap == maxSize so the first maxSize Adds fill it; the next Add
	// blocks on b.in <- item.
	b := New[int](2, 0, func(batch []int) { flushed.Add(int64(len(batch))) }, WithBufferSize[int](2))
	b.Add(1)
	b.Add(2) // buffer full now

	addDone := make(chan bool, 1)
	go func() {
		// This Add blocks (buffer full, no consumer read yet because run loops
		// on b.in but picks items one at a time; we race Close to interrupt it).
		addDone <- b.Add(3)
	}()
	// Give the blocked Add time to park on the send, then Close.
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, b.Close())

	select {
	case ok := <-addDone:
		// Add returned false (Close's done closed before the buffered send
		// resolved), OR true (the send won the race and the item was flushed by
		// Close's final drain). Either is contract-valid; both branches get
		// exercised across runs.
		_ = ok
	case <-time.After(time.Second):
		t.Fatal("blocked Add did not return after Close")
	}
}

// TestCloseDrainsFullMaxSizeBatch covers the len(buf) >= b.maxSize flush branch
// inside Close's final-drain loop. We stuff > maxSize items into the buffer
// (bypassing the run-loop read by closing done first) so Close's drain fills a
// complete maxSize batch and flushes it.
func TestCloseDrainsFullMaxSizeBatch(t *testing.T) {
	var mu sync.Mutex
	var batches [][]int
	// A flusher that is slow enough to let the producer fill the buffer beyond
	// maxSize before the run loop drains it.
	b := New[int](3, 0, func(batch []int) {
		mu.Lock()
		batches = append(batches, append([]int(nil), batch...))
		mu.Unlock()
	}, WithBufferSize[int](16))

	// Add several maxSize-sized groups faster than the collector can read; then
	// Close. The final-drain loop will accumulate up to maxSize items before
	// flushing (the len(buf) >= b.maxSize branch).
	for i := 1; i <= 9; i++ {
		require.True(t, b.Add(i))
	}
	require.NoError(t, b.Close())

	mu.Lock()
	defer mu.Unlock()
	total := 0
	for _, batch := range batches {
		require.LessOrEqual(t, len(batch), 3, "no batch may exceed maxSize")
		total += len(batch)
	}
	require.Equal(t, 9, total, "every Added item must be flushed exactly once")
}

// TestCloseEmptyNoFlush covers the no-items final-drain branch of Close (the
// default case of the drain select with len(buf) == 0).
func TestCloseEmptyNoFlush(t *testing.T) {
	var flushes atomic.Int64
	b := New[int](4, 0, func([]int) { flushes.Add(1) })
	require.NoError(t, b.Close())
	// Nothing was added; the final drain finds nothing and must not call flush.
	require.Equal(t, int64(0), flushes.Load())
}

// TestRunDrainOnDone covers the run-loop <-b.done drain path: items accepted
// before Close are drained by the collector (not Close's final drain). This is
// the case item := <-b.in branch inside the done-select of run.
func TestRunDrainOnDone(t *testing.T) {
	var mu sync.Mutex
	var got []int
	b := New[int](100, 0, func(batch []int) {
		mu.Lock()
		got = append(got, batch...)
		mu.Unlock()
	}, WithBufferSize[int](8))
	// Fill the buffer; the collector reads some, others remain buffered when
	// Close fires. The run-loop's done-branch must drain the rest.
	for i := range 8 {
		require.True(t, b.Add(i))
	}
	// Give the collector a moment to read a few items, then Close.
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, b.Close())
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 8)
}

// TestFlushDrainsInputChannel covers the run-loop's flushCh branch that first
// drains any items still queued in b.in before flushing (the drain loop).
func TestFlushDrainsInputChannel(t *testing.T) {
	var mu sync.Mutex
	var got []int
	b := New[int](100, 0, func(batch []int) {
		mu.Lock()
		got = append(got, batch...)
		mu.Unlock()
	}, WithBufferSize[int](16))
	// Add several items without yielding to the collector, then Flush. The
	// flushCh handler drains b.in before flushing.
	for i := range 5 {
		require.True(t, b.Add(i))
	}
	b.Flush()
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 5)
}

// TestNewWithBufferSize_SmallerThanMax covers WithBufferSize where the buffer
// is smaller than maxSize (exercises the option path and backpressure).
func TestNewWithBufferSize_SmallerThanMax(t *testing.T) {
	var flushed atomic.Int64
	b := New[int](5, 0, func(batch []int) { flushed.Add(int64(len(batch))) }, WithBufferSize[int](2))
	defer func() { _ = b.Close() }()
	// maxSize=5; buffer=2 -> a full batch needs 5 Adds, the collector reads and
	// buffers internally until it hits 5.
	for i := range 5 {
		require.True(t, b.Add(i))
	}
	b.Flush()
	require.Equal(t, int64(5), flushed.Load())
}

// TestErrClosedIdentity re-checks the sentinel (covers the var read).
func TestErrClosedIdentity(t *testing.T) {
	require.True(t, errors.Is(ErrClosed, ErrClosed))
	require.Equal(t, "batcher: closed", ErrClosed.Error())
}

// TestIntervalTickerFires covers the run-loop <-tickerC branch by adding items
// and waiting for a time-triggered flush.
func TestIntervalTickerFires(t *testing.T) {
	var mu sync.Mutex
	var got []int
	b := New[int](100, 15*time.Millisecond, func(batch []int) {
		mu.Lock()
		got = append(got, batch...)
		mu.Unlock()
	})
	defer func() { _ = b.Close() }()
	b.Add(1)
	b.Add(2)
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 2
	}, 500*time.Millisecond, 5*time.Millisecond)
}
