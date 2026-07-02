package debounce

import (
	"sync/atomic"
	"testing"
	"time"
)

// Regression: fn must not fire after Close. Before the fix the AfterFunc
// goroutine invoked the raw fn; if it had already been dispatched when Close
// cancelled the timer, fn ran once after Close.
func TestDebounce_NoFireAfterClose(t *testing.T) {
	var n atomic.Int64
	d := New(20*time.Millisecond, func() { n.Add(1) })
	d.Call()
	d.Close()                         // cancels the pending timer
	time.Sleep(60 * time.Millisecond) // well past `after`
	if got := n.Load(); got != 0 {
		t.Fatalf("fn fired %d times after Close, want 0", got)
	}
}

// Regression: Flush fires fn exactly once. Guards the normal Flush path and the
// Stop()-return guard that prevents a Flush/timer double-fire.
func TestDebounce_FlushFiresOnce(t *testing.T) {
	var n atomic.Int64
	d := New(50*time.Millisecond, func() { n.Add(1) })
	defer d.Close()
	d.Call()
	d.Flush()                         // timer pending -> fire once, AfterFunc stopped
	time.Sleep(80 * time.Millisecond) // past `after`; a double-fire would show 2
	if got := n.Load(); got != 1 {
		t.Fatalf("fn fired %d times, want 1", got)
	}
}
