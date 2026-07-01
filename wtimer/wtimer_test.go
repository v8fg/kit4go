package wtimer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAddOneShot(t *testing.T) {
	w := New()
	defer w.Close()
	var fired atomic.Int64
	w.Add(20*time.Millisecond, func() { fired.Add(1) })
	time.Sleep(60 * time.Millisecond)
	require.Equal(t, int64(1), fired.Load())
}

func TestAddRecurring(t *testing.T) {
	w := New()
	defer w.Close()
	var fired atomic.Int64
	timer, _ := w.AddRecurring(20*time.Millisecond, func() { fired.Add(1) })
	time.Sleep(100 * time.Millisecond)
	require.GreaterOrEqual(t, fired.Load(), int64(3))
	timer.Cancel()
	fired.Store(0)
	time.Sleep(60 * time.Millisecond)
	require.Equal(t, int64(0), fired.Load(), "cancelled recurring should not fire")
}

func TestCancelOneShot(t *testing.T) {
	w := New()
	defer w.Close()
	var fired atomic.Int64
	timer, _ := w.Add(50*time.Millisecond, func() { fired.Add(1) })
	timer.Cancel()
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, int64(0), fired.Load())
}

func TestNilCallback(t *testing.T) {
	w := New()
	defer w.Close()
	_, err := w.Add(10*time.Millisecond, nil)
	require.Error(t, err)
}

func TestAddAfterClose(t *testing.T) {
	w := New()
	w.Close()
	_, err := w.Add(10*time.Millisecond, func() {})
	require.ErrorIs(t, err, ErrClosed)
}

func TestCloseIdempotent(t *testing.T) {
	w := New()
	w.Close()
	w.Close()
}

func TestLen(t *testing.T) {
	w := New()
	defer w.Close()
	w.Add(1*time.Second, func() {})
	w.Add(1*time.Second, func() {})
	require.Equal(t, 2, w.Len())
}

func TestManyTimers(t *testing.T) {
	w := New()
	defer w.Close()
	var fired atomic.Int64
	const n = 100
	for i := 0; i < n; i++ {
		w.Add(time.Duration(i+1)*time.Millisecond, func() { fired.Add(1) })
	}
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, int64(n), fired.Load())
}

func TestFiresInOrder(t *testing.T) {
	w := New()
	defer w.Close()
	var mu sync.Mutex
	var order []int
	w.Add(50*time.Millisecond, func() { mu.Lock(); order = append(order, 3); mu.Unlock() })
	w.Add(10*time.Millisecond, func() { mu.Lock(); order = append(order, 1); mu.Unlock() })
	w.Add(30*time.Millisecond, func() { mu.Lock(); order = append(order, 2); mu.Unlock() })
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	require.Equal(t, []int{1, 2, 3}, order)
	mu.Unlock()
}

func TestCancelledCleanedFromHeap(t *testing.T) {
	w := New()
	defer w.Close()
	w.Add(1*time.Second, func() {})
	timer, _ := w.Add(1*time.Second, func() {})
	require.Equal(t, 2, w.Len())
	timer.Cancel()
	// The wheel cleans cancelled timers on the next tick.
	w.wake()
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, 1, w.Len())
}
