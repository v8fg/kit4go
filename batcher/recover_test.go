package batcher

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFlushPanicRecovered_F2 is the R11-F2 regression test. Before the fix every
// flushFn invocation ran with NO recover: a panicking flush aborted the collector
// goroutine, so defer close(b.closed) never ran and Close deadlocked forever on
// <-b.closed (process abort + hang). This test would FAIL on the old code:
//   - without recover, the panic propagates and crashes the test binary (no
//     graceful Recovered()==1 assertion); under -race the process aborts.
//   - Close would deadlock (timeout) because b.closed is never closed.
//
// After the fix the collector survives, Recovered() counts the panic, the
// onPanic hook fires, and Close returns cleanly (no deadlock). Run with -race.
func TestFlushPanicRecovered_F2(t *testing.T) {
	var hookFired atomic.Bool
	var panicVal atomic.Value

	flush := func(batch []int) { panic("boom") }

	b := New[int](2, 0, flush)
	b.SetOnPanic(func(r any) {
		panicVal.Store(r)
		hookFired.Store(true)
	})

	// Two Adds reach maxSize and trigger the size-flush -> panic (site #1).
	require.True(t, b.Add(1))
	require.True(t, b.Add(2))

	// The collector must SURVIVE the panic: it keeps reading and the buffer is
	// flushed again on Close. Give the collector a moment to recover and loop.
	require.True(t, b.Add(3))
	require.True(t, b.Add(4)) // another size-triggered flush -> second panic

	// Close must return (not deadlock). Its final-drain flushes any straggler in
	// b.in, going through safeFlush too — proving the recover path on site #5.
	done := make(chan struct{})
	go func() {
		_ = b.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close deadlocked — collector did not survive flush panic (F2 regression)")
	}

	require.GreaterOrEqual(t, b.Recovered(), uint64(2), "Recovered() must count recovered flush panics")
	require.True(t, hookFired.Load(), "onPanic hook must fire on recovered flush panic")
	if v := panicVal.Load(); v != "boom" {
		t.Fatalf("onPanic received %v, want \"boom\"", v)
	}
}

// TestFlushPanicInCloseDrain_F2 covers the recover path specifically on Close's
// final straggler-drain (site #5), where a panic during the drain would abort
// Close and leak the mu write-lock. Before the fix this hung Close forever.
func TestFlushPanicInCloseDrain_F2(t *testing.T) {
	flush := func(batch []int) { panic("close-drain boom") }

	// maxSize=2, buffer=1 so items pile up and the collector is still draining
	// when Close runs its final-drain with a pending batch -> panic recovered.
	b := New[int](2, 0, flush, WithBufferSize[int](1))
	require.True(t, b.Add(1))
	require.True(t, b.Add(2)) // size-trigger -> collector flush -> panic recovered

	done := make(chan struct{})
	go func() {
		_ = b.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close deadlocked on a straggler-flush panic (F2 site #5 regression)")
	}

	require.GreaterOrEqual(t, b.Recovered(), uint64(1), "straggler-drain panic must be recovered")
}

// TestSetOnPanic_Concurrent runs the hook install path concurrently with flush
// panics to confirm SetOnPanic / Recovered are race-free under the race detector
// (atomic.Uint64 + atomic.Pointer[func(any)], no shared-mutable field access).
func TestSetOnPanic_Concurrent(t *testing.T) {
	flush := func(batch []int) { panic("race boom") }
	b := New[int](1, 0, flush, WithBufferSize[int](4))

	stop := make(chan struct{})
	go func() { // toggler
		on := false
		for {
			select {
			case <-stop:
				return
			default:
				if on {
					b.SetOnPanic(func(any) {})
				} else {
					b.SetOnPanic(nil)
				}
				on = !on
			}
		}
	}()

	for i := 0; i < 8; i++ {
		_ = b.Add(i) // each triggers a flush -> panic -> recover
	}
	close(stop)
	_ = b.Close()

	require.GreaterOrEqual(t, b.Recovered(), uint64(8), "all flush panics recovered under concurrent SetOnPanic")
}

// TestWithBufferSize_NegativeClamped_F6 is the R11-F6 regression test. Before
// the fix WithBufferSize stored n verbatim, and New's make(chan T, b.bufferCap)
// panicked at construction for n<0 ("makechan: size out of range"). After the
// fix a negative n is ignored (the default maxSize cap is kept), so New returns
// without panicking.
func TestWithBufferSize_NegativeClamped_F6(t *testing.T) {
	// Must not panic. On the old code this line panics inside make(chan, -1).
	var flushed [][]int
	b := New[int](2, 0, func(batch []int) { flushed = append(flushed, batch) },
		WithBufferSize[int](-1))

	require.True(t, b.Add(1))
	require.True(t, b.Add(2)) // size-trigger flush -> bufferCap kept at default (2)
	require.NoError(t, b.Close())

	require.Len(t, flushed, 1)
	require.Equal(t, []int{1, 2}, flushed[0])
}
