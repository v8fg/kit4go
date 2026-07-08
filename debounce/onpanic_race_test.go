package debounce

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestDebounce_SetOnPanic_Race is the R11-F1 regression test for Debounce.
// SetOnPanic writes the hook field from the caller goroutine while the wrapped
// fn's recover path (on the AfterFunc/Flush goroutine) reads it. Under -race the
// bare-field version flags a data race; the atomic.Pointer storage must be clean.
//
// One goroutine hammers SetOnPanic(fn)/SetOnPanic(nil) toggling; another hammers
// Flush so the wrapped fn (which panics) fires on a fresh goroutine and the
// recover path reads the hook concurrently with the writer. Run under -race.
func TestDebounce_SetOnPanic_Race(t *testing.T) {
	d := New(time.Microsecond, func() { panic("boom") })

	hook := func(any) {}
	stop := atomic.Bool{}
	done := make(chan struct{})

	// Writer goroutine: toggle the hook on and off.
	go func() {
		defer close(done)
		for !stop.Load() {
			d.SetOnPanic(hook)
			d.SetOnPanic(nil)
		}
	}()

	// Producer goroutine: Call + Flush so the panicking wrapped fn fires on its
	// own goroutine and the recover path reads the hook concurrently.
	go func() {
		for !stop.Load() {
			d.Call()
			d.Flush()
		}
	}()

	// Let the race window be exercised.
	time.Sleep(100 * time.Millisecond)
	stop.Store(true)
	<-done

	d.Close()
	if d.Recovered() == 0 {
		t.Fatal("no panics recovered during race test; recover path never ran")
	}
}

// TestThrottle_SetOnPanic_Race is the R11-F1 regression test for Throttle.
// SetOnPanic writes the hook field from the caller goroutine while safeFire's
// recover path (on Call's spawned goroutine) reads it. Under -race the
// bare-field version flags a data race; the atomic.Pointer storage must be clean.
//
// One goroutine hammers SetOnPanic(fn)/SetOnPanic(nil) toggling; another hammers
// Call so the panicking fn fires via safeFire and the recover path reads the
// hook concurrently with the writer. Run under -race.
func TestThrottle_SetOnPanic_Race(t *testing.T) {
	th := NewThrottle(time.Microsecond, func() { panic("boom") })

	hook := func(any) {}
	stop := atomic.Bool{}
	done := make(chan struct{})

	// Writer goroutine: toggle the hook on and off.
	go func() {
		defer close(done)
		for !stop.Load() {
			th.SetOnPanic(hook)
			th.SetOnPanic(nil)
		}
	}()

	// Producer goroutine: Call so the panicking fn fires via safeFire and the
	// recover path reads the hook concurrently with the writer.
	go func() {
		for !stop.Load() {
			th.Call()
		}
	}()

	// Let the race window be exercised.
	time.Sleep(100 * time.Millisecond)
	stop.Store(true)
	<-done

	th.Close()
	if th.Recovered() == 0 {
		t.Fatal("no panics recovered during race test; recover path never ran")
	}
}
