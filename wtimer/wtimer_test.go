package wtimer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeClock is a deterministic, advanceable clock seam for timing-correctness
// assertions. Tests install it on a Timer.now so the run loop's due-check and
// reschedule read this clock instead of wall time; advancing it past a timer's
// when makes the wheel see d<=0 and fire deterministically — no wall-clock
// Sleep on the correctness boundary.
//
// The clock is mutex-guarded because the run-loop goroutine reads now()
// concurrently with the test advancing it.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(0, 0)} }

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// useClock swaps a Timer's clock to fake and repoints when relative to the
// fake clock so the wheel evaluates it under the injected time. The swap
// happens under the wheel's mutex (the run loop reads these fields under the
// same lock) to stay race-clean. Returns the fake clock for advancement.
func useClock(w *Wheel, t *Timer, delay time.Duration) *fakeClock {
	c := newFakeClock()
	w.mu.Lock()
	t.now = c.now
	t.when = c.now().Add(delay)
	w.mu.Unlock()
	return c
}

// fireSync is a short, bounded wait for the run-loop goroutine to react to a
// wakeup after the (deterministic) clock state is set. It only synchronizes
// the goroutine — it never defines the correctness boundary.
func fireSync() { time.Sleep(30 * time.Millisecond) }

func TestAddOneShot(t *testing.T) {
	w := New()
	defer w.Close()
	var fired atomic.Int64
	timer, err := w.Add(20*time.Millisecond, func() { fired.Add(1) })
	require.NoError(t, err)
	// Deterministic: advance the injected clock past when and wake the wheel.
	clock := useClock(w, timer, 20*time.Millisecond)
	clock.advance(21 * time.Millisecond)
	w.wake()
	fireSync()
	require.Equal(t, int64(1), fired.Load())
}

func TestAddRecurring(t *testing.T) {
	w := New()
	defer w.Close()
	var fired atomic.Int64
	timer, err := w.AddRecurring(20*time.Millisecond, func() { fired.Add(1) })
	require.NoError(t, err)
	clock := useClock(w, timer, 20*time.Millisecond)
	// Advance past several intervals; the recurring reschedule reads the same
	// injected clock, so each tick is deterministic.
	for i := 0; i < 4; i++ {
		clock.advance(20 * time.Millisecond)
		w.wake()
		fireSync()
	}
	require.GreaterOrEqual(t, fired.Load(), int64(3))

	// Cancel-boundary correctness: advancing further must not produce more
	// fires once cancelled.
	timer.Cancel()
	fired.Store(0)
	clock.advance(60 * time.Millisecond)
	w.wake()
	fireSync()
	require.Equal(t, int64(0), fired.Load(), "cancelled recurring should not fire")
}

func TestCancelOneShot(t *testing.T) {
	w := New()
	defer w.Close()
	var fired atomic.Int64
	timer, err := w.Add(50*time.Millisecond, func() { fired.Add(1) })
	require.NoError(t, err)
	clock := useClock(w, timer, 50*time.Millisecond)
	timer.Cancel()
	// Advance past the due time — a cancelled timer must not fire.
	clock.advance(100 * time.Millisecond)
	w.wake()
	fireSync()
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
