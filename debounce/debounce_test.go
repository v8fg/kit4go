package debounce

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDebounceFiresAfterQuiet(t *testing.T) {
	var fired atomic.Int64
	d := New(30*time.Millisecond, func() { fired.Add(1) })
	defer d.Close()
	d.Call()
	time.Sleep(10 * time.Millisecond)
	d.Call() // reset
	time.Sleep(10 * time.Millisecond)
	d.Call()                                 // reset again
	require.Equal(t, int64(0), fired.Load()) // not yet
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int64(1), fired.Load()) // fired once after quiet
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
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, int64(1), fired.Load())
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
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, int64(1), fired.Load())
}

func TestThrottleDropsWithinInterval(t *testing.T) {
	var fired atomic.Int64
	th := NewThrottle(50*time.Millisecond, func() { fired.Add(1) })
	defer th.Close()
	th.Call() // fires
	for i := 0; i < 10; i++ {
		require.False(t, th.Call()) // throttled
	}
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, int64(1), fired.Load())
}

func TestThrottleFiresAfterInterval(t *testing.T) {
	var fired atomic.Int64
	th := NewThrottle(30*time.Millisecond, func() { fired.Add(1) })
	defer th.Close()
	th.Call()
	time.Sleep(40 * time.Millisecond)
	require.True(t, th.Call()) // interval elapsed
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, int64(2), fired.Load())
}

func TestThrottleCallBlocking(t *testing.T) {
	var fired atomic.Int64
	th := NewThrottle(30*time.Millisecond, func() { fired.Add(1) })
	defer th.Close()
	require.True(t, th.CallBlocking())
	require.False(t, th.CallBlocking())
	require.Equal(t, int64(1), fired.Load())
}

func TestThrottleCalls(t *testing.T) {
	th := NewThrottle(20*time.Millisecond, func() {})
	defer th.Close()
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
