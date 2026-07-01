package log4go

import (
	"log"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// daemonPanics counts panics recovered inside a writer daemon goroutine
// (file/kafka/net/webhook). A daemon that panics is dead — its records stop
// flowing — so the panic must be observable, not silent. Exposed via
// RuntimeStats().DaemonPanics (L5: observable degradation).
var daemonPanics uint64

// defaultShutdownTimeout bounds a writer's Stop: a dead or stuck daemon cannot
// hang process exit longer than this. Generous enough to flush under normal
// load, short enough that a wedged writer doesn't stall shutdown (L6).
const defaultShutdownTimeout = 5 * time.Second

// recordDaemonPanic logs a recovered daemon panic with its stack and bumps the
// package counter, so a dead writer daemon is visible to monitoring instead of
// silently stopping (and, before this, crashing the whole process).
func recordDaemonPanic(name string, r any) {
	atomic.AddUint64(&daemonPanics, 1)
	log.Printf("[log4go] %s writer daemon panic (recovered; that writer is now DEAD — its records will not flow until restart): %v\n%s",
		name, r, debug.Stack())
}

// waitQuit waits for a daemon's quit signal, but no longer than timeout. If the
// daemon is dead (panicked) or wedged, Stop returns instead of hanging the
// process. Returns true if the daemon signalled a clean shutdown, false on
// timeout. timeout is taken as a param (not a package var) so tests can exercise
// the deadline quickly and no mutable global is introduced (L6).
func waitQuit(name string, quit <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-quit:
		return true
	case <-time.After(timeout):
		log.Printf("[log4go] %s writer shutdown timed out after %s (daemon dead or wedged); continuing", name, timeout)
		return false
	}
}
