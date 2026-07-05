package wtimer

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTimer_Cancelled covers Timer.Cancelled() (the reader side of the cancel
// flag), which was 0% because the existing tests only call Cancel() and rely on
// behaviour, never asserting the flag directly.
func TestTimer_Cancelled(t *testing.T) {
	w := New()
	defer w.Close()

	timer, err := w.Add(1*time.Second, func() {})
	require.NoError(t, err)
	require.False(t, timer.Cancelled(), "fresh timer must report not-cancelled")

	timer.Cancel()
	require.True(t, timer.Cancelled(), "after Cancel the flag must read true")

	// Idempotent: a second Cancel keeps it cancelled.
	timer.Cancel()
	require.True(t, timer.Cancelled())
}

// TestWheel_PanicRecoveredSafeFire covers the recover branch of safeFire: a
// panicking callback must be recovered (counted in Recovered()), surfaced via
// the onPanic hook, and must NOT crash the wheel — subsequent callbacks still
// fire. This exercises safeFire's `if r := recover(); r != nil` path and the
// SetOnPanic / Recovered accessors.
func TestWheel_PanicRecoveredSafeFire(t *testing.T) {
	w := New()
	defer w.Close()

	var hookFired atomic.Value // stores the recovered value
	w.SetOnPanic(func(r any) { hookFired.Store(r) })

	// Before any panic: Recovered is zero, and SetOnPanic installed the hook.
	require.Equal(t, uint64(0), w.Recovered(), "Recovered must start at 0")

	var goodRan atomic.Int64
	// Schedule a panicking one-shot first, then a normal one shortly after.
	_, err := w.Add(15*time.Millisecond, func() { panic("wtimer-boom") })
	require.NoError(t, err)
	_, err = w.Add(40*time.Millisecond, func() { goodRan.Add(1) })
	require.NoError(t, err)

	// Let both fire (panic-recovered, then normal).
	time.Sleep(120 * time.Millisecond)

	require.Equal(t, uint64(1), w.Recovered(), "exactly one panic must be recovered")
	if v := hookFired.Load(); v == nil {
		t.Fatal("onPanic hook was not fired")
	} else if s, ok := v.(string); !ok || s != "wtimer-boom" {
		t.Fatalf("onPanic got %v, want \"wtimer-boom\"", v)
	}
	require.Equal(t, int64(1), goodRan.Load(), "wheel must keep running after a recovered panic")
}

// TestWheel_RecoveredZeroAndAccessor covers Recovered() on a wheel that never
// panics (the non-recovery read path), ensuring the accessor is exercised
// independently of safeFire.
func TestWheel_RecoveredZeroAndAccessor(t *testing.T) {
	w := New()
	defer w.Close()
	require.Equal(t, uint64(0), w.Recovered())

	var ran atomic.Int64
	_, _ = w.Add(10*time.Millisecond, func() { ran.Add(1) })
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int64(1), ran.Load())
	// Still no panics recovered.
	require.Equal(t, uint64(0), w.Recovered())
}

// TestWheel_SetOnPanicNilSafe exercises SetOnPanic with the default (nil) hook:
// a panic must still be recovered and counted even when no hook is installed.
func TestWheel_SetOnPanicNilSafe(t *testing.T) {
	w := New()
	defer w.Close()
	// Do NOT call SetOnPanic — onPanic stays nil; safeFire's `if p.onPanic != nil`
	// branch (false path) must execute without dereferencing nil.
	_, _ = w.Add(15*time.Millisecond, func() { panic("no-hook") })
	time.Sleep(80 * time.Millisecond)
	require.Equal(t, uint64(1), w.Recovered(), "panic recovered even without an onPanic hook")
}
