package debounce

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDebounceFiresAfterQuiet(t *testing.T) {
	var fired atomic.Int64
	// Wide quiet window (200ms) so the negative check is robust even if the
	// interspersed sleeps overshoot under load — the timer cannot fire before
	// 200ms, so "not yet" holds while calls keep resetting it.
	d := New(200*time.Millisecond, func() { fired.Add(1) })
	defer d.Close()
	d.Call()
	time.Sleep(10 * time.Millisecond)
	d.Call() // reset
	time.Sleep(10 * time.Millisecond)
	d.Call()                                 // reset again
	require.Equal(t, int64(0), fired.Load()) // still coalescing, window not elapsed
	// fn fires exactly once after calls stop. Poll instead of a fixed sleep:
	// the timer + AfterFunc goroutine schedule is non-deterministic under load (E5).
	require.Eventually(t, func() bool { return fired.Load() == 1 },
		500*time.Millisecond, 5*time.Millisecond)
}

func TestDebounceCancel(t *testing.T) {
	var fired atomic.Int64
	d := New(20*time.Millisecond, func() { fired.Add(1) })
	d.Call()
	d.Cancel()
	time.Sleep(40 * time.Millisecond)
	require.Equal(t, int64(0), fired.Load())
	require.False(t, d.Pending())
}

func TestDebounceFlush(t *testing.T) {
	var fired atomic.Int64
	d := New(1*time.Second, func() { fired.Add(1) })
	d.Call()
	require.True(t, d.Pending())
	d.Flush()
	// Flush spawns a goroutine; poll for its completion instead of a fixed
	// sleep so the assertion survives goroutine-scheduling latency under load (E5).
	require.Eventually(t, func() bool { return fired.Load() == 1 },
		500*time.Millisecond, 5*time.Millisecond)
	require.False(t, d.Pending())
}

func TestDebounceCallWith(t *testing.T) {
	var got any
	var d *Debounce
	done := make(chan struct{})
	d = New(20*time.Millisecond, func() {
		got = d.LastArg()
		close(done)
	})
	d.CallWith("hello")
	<-done
	require.Equal(t, "hello", got)
}

func TestDebouncePending(t *testing.T) {
	d := New(1*time.Second, func() {})
	require.False(t, d.Pending())
	d.Call()
	require.True(t, d.Pending())
	d.Cancel()
	require.False(t, d.Pending())
}

func TestDebounceCloseStopsCalls(t *testing.T) {
	var fired atomic.Int64
	d := New(10*time.Millisecond, func() { fired.Add(1) })
	d.Close()
	d.Call()
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, int64(0), fired.Load())
}

func TestDebouncePanicOnNilFn(t *testing.T) {
	require.Panics(t, func() { New(10*time.Millisecond, nil) })
}

func TestThrottleFirstCallFires(t *testing.T) {
	var fired atomic.Int64
	th := NewThrottle(50*time.Millisecond, func() { fired.Add(1) })
	defer th.Close()
	require.True(t, th.Call())
	// Call spawns a safeFire goroutine; poll for it rather than a fixed sleep (E5).
	require.Eventually(t, func() bool { return fired.Load() == 1 },
		500*time.Millisecond, 5*time.Millisecond)
}

func TestThrottleDropsWithinInterval(t *testing.T) {
	var fired atomic.Int64
	th := NewThrottle(50*time.Millisecond, func() { fired.Add(1) })
	defer th.Close()
	th.Call() // fires
	for range 10 {
		require.False(t, th.Call()) // throttled
	}
	require.Eventually(t, func() bool { return fired.Load() == 1 },
		500*time.Millisecond, 5*time.Millisecond)
}

func TestThrottleFiresAfterInterval(t *testing.T) {
	var fired atomic.Int64
	fc := newFakeClock()
	th := NewThrottle(30*time.Millisecond, func() { fired.Add(1) })
	defer th.Close()
	th.now = fc.Now // inject fake clock: deterministic window assertion (E5)

	require.True(t, th.Call())
	// Advance the fake clock past the interval — no wall-clock Sleep. The
	// throttle reads now via the seam, so the next Call must fire regardless of
	// CPU contention.
	fc.advance(40 * time.Millisecond)
	require.True(t, th.Call()) // interval elapsed
	require.Eventually(t, func() bool { return fired.Load() == 2 },
		500*time.Millisecond, 5*time.Millisecond)
}

func TestThrottleCallBlocking(t *testing.T) {
	var fired atomic.Int64
	fc := newFakeClock()
	th := NewThrottle(30*time.Millisecond, func() { fired.Add(1) })
	defer th.Close()
	th.now = fc.Now // inject fake clock (E5)

	require.True(t, th.CallBlocking())
	require.False(t, th.CallBlocking())
	require.Equal(t, int64(1), fired.Load())
}

func TestThrottleCalls(t *testing.T) {
	fc := newFakeClock()
	th := NewThrottle(20*time.Millisecond, func() {})
	defer th.Close()
	th.now = fc.Now // inject fake clock (E5)

	th.Call()
	th.Call()
	require.Equal(t, int64(1), th.Calls())
}

func TestThrottleClose(t *testing.T) {
	th := NewThrottle(10*time.Millisecond, func() {})
	th.Close()
	require.False(t, th.Call())
}

func TestThrottlePanicOnNilFn(t *testing.T) {
	require.Panics(t, func() { NewThrottle(10*time.Millisecond, nil) })
}
