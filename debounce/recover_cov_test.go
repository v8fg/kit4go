package debounce

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestDebounce_SetOnPanic(t *testing.T) {
	d := New(10*time.Millisecond, func() {})
	d.SetOnPanic(func(any) {})
	if d.Recovered() != 0 {
		t.Fatal("fresh debouncer should have 0 recovered")
	}
}

func TestDebounce_Recovered(t *testing.T) {
	d := New(10*time.Millisecond, func() {})
	if d.Recovered() != 0 {
		t.Fatal("initial recovered should be 0")
	}
}

func TestDebounce_FlushRecoversPanic(t *testing.T) {
	var fired atomic.Bool
	d := New(time.Hour, func() { panic("boom") })
	d.SetOnPanic(func(any) { fired.Store(true) })
	d.Call()
	d.Flush()
	time.Sleep(50 * time.Millisecond)
	if !fired.Load() {
		t.Fatal("onPanic should have fired")
	}
	if d.Recovered() != 1 {
		t.Fatalf("Recovered = %d, want 1", d.Recovered())
	}
}

func TestThrottle_SetOnPanic(t *testing.T) {
	th := NewThrottle(time.Hour, func() {})
	th.SetOnPanic(func(any) {})
	if th.Recovered() != 0 {
		t.Fatal("initial recovered should be 0")
	}
}

func TestThrottle_Recovered(t *testing.T) {
	th := NewThrottle(time.Hour, func() {})
	if th.Recovered() != 0 {
		t.Fatal("initial recovered should be 0")
	}
}

func TestThrottle_CallPanicRecovered(t *testing.T) {
	var fired atomic.Bool
	th := NewThrottle(time.Millisecond, func() { panic("boom") })
	th.SetOnPanic(func(any) { fired.Store(true) })
	th.Call()
	time.Sleep(50 * time.Millisecond)
	if !fired.Load() {
		t.Fatal("onPanic should have fired")
	}
	if th.Recovered() != 1 {
		t.Fatalf("Recovered = %d, want 1", th.Recovered())
	}
}

func TestDebounce_LastArgEmpty(t *testing.T) {
	d := New(10*time.Millisecond, func() {})
	if d.LastArg() != nil {
		t.Fatal("LastArg before CallWith should be nil")
	}
}
