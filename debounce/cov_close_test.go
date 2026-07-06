package debounce

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestDebounce_NewClosedGuardHit covers the defensive closed-guard branch inside
// the wrapped closure built by New (debounce.go:54-56): when d.fn runs after
// Close set d.closed, it returns without invoking the user fn.
//
// Why a direct white-box call: the AfterFunc goroutine that normally runs d.fn is
// cancelled by Close's Cancel() (Close stops the timer before it can fire), so
// under any real Call→Close ordering the closure is simply never entered. The
// only way to actually reach the closed-guard `return` is to invoke the wrapped
// closure d.fn after d.closed is true. We do that directly (same package) and
// assert the user fn does not run.
func TestDebounce_NewClosedGuardHit(t *testing.T) {
	var ran atomic.Bool
	d := New(time.Hour, func() { ran.Store(true) })
	d.Close()

	// The wrapped closure must short-circuit on the closed flag without calling
	// the user fn. If the guard regresses, ran flips to true.
	d.fn()
	if ran.Load() {
		t.Fatal("wrapped fn ran after Close; closed-guard regressed")
	}
}

// TestThrottle_CallBlockingAfterClose covers CallBlocking's closed-throttle
// branch (debounce.go:188-190): a closed throttle returns false without taking
// the lock or running fn.
func TestThrottle_CallBlockingAfterClose(t *testing.T) {
	var ran atomic.Bool
	th := NewThrottle(time.Millisecond, func() { ran.Store(true) })
	th.Close()
	if th.CallBlocking() {
		t.Fatal("CallBlocking after Close returned true, want false")
	}
	if ran.Load() {
		t.Fatal("fn ran on CallBlocking after Close")
	}
}
