package log4go

import (
	"sync/atomic"
	"testing"
	"time"
)

// L5b/L6: a recovered daemon panic is counted + surfaced (RuntimeStats), and
// shutdown waits on the daemon only up to a deadline (no hang).

func TestRecordDaemonPanic_Counted(t *testing.T) {
	before := atomic.LoadUint64(&daemonPanics)
	defer atomic.StoreUint64(&daemonPanics, before)

	recordDaemonPanic("test", "boom-1")
	recordDaemonPanic("test", "boom-2")

	if got := atomic.LoadUint64(&daemonPanics); got != before+2 {
		t.Fatalf("daemonPanics: want %d, got %d", before+2, got)
	}
	m := RuntimeStats()
	if m.DaemonPanics != before+2 {
		t.Fatalf("RuntimeStats().DaemonPanics: want %d, got %d", before+2, m.DaemonPanics)
	}
}

func TestWaitQuit_SignalledReturnsImmediately(t *testing.T) {
	quit := make(chan struct{})
	close(quit)
	if !waitQuit("test", quit, time.Second) {
		t.Fatal("waitQuit should return true when quit is signalled")
	}
}

func TestWaitQuit_TimesOutOnDeadDaemon(t *testing.T) {
	quit := make(chan struct{}) // never signalled (simulates a dead daemon)
	start := time.Now()
	if waitQuit("test", quit, 30*time.Millisecond) {
		t.Fatal("waitQuit should return false when the daemon never signals")
	}
	elapsed := time.Since(start)
	if elapsed < 25*time.Millisecond || elapsed > 400*time.Millisecond {
		t.Fatalf("waitQuit should honor the deadline; elapsed=%v", elapsed)
	}
}
