package log4go

import (
	"fmt"
	"os"
	"runtime/debug"
)

// Sync flushes all in-flight records and writer buffers, blocking until the
// logger's pipeline has drained. It closes the pipeline (the logger is unusable
// afterward); intended as the pre-exit flush for Panic/Fatal so no record is
// lost before the process exits or unwinds.
func (l *Logger) Sync() { l.Close() }

// Panic logs the message at CRITICAL, flushes the pipeline, then panics with the
// formatted message — so the record is captured before the stack unwinds. Pair
// with Recover (or a top-level recover) to surface the panic after logging.
func (l *Logger) Panic(format string, args ...interface{}) {
	l.deliverRecordToWriter(CRITICAL, format, args...)
	l.Sync()
	panic(fmt.Sprintf(format, args...))
}

// Fatal logs the message at CRITICAL, flushes the pipeline, then exits the
// process with status 1. The flush guarantees the record reaches every
// registered writer before exit.
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.deliverRecordToWriter(CRITICAL, format, args...)
	l.Sync()
	os.Exit(1)
}

// Recover is a deferred panic handler: it logs the recovered value and stack at
// CRITICAL, flushes, then re-raises the panic (so the crash is not swallowed).
// Use it to capture panics into the log pipeline (optionally a WebhookWriter for
// sentry-style alerting) before they propagate:
//
//	defer log4go.Recover(func() *log4go.Logger { return lg })
//
// getLogger selects the logger (nil/nil-returning -> the package singleton).
func Recover(getLogger func() *Logger) {
	r := recover()
	if r == nil {
		return
	}
	lg := defaultLogger()
	if getLogger != nil {
		if l := getLogger(); l != nil {
			lg = l
		}
	}
	lg.deliverRecordToWriter(CRITICAL, "panic recovered: %v\n%s", r, debug.Stack())
	lg.Sync()
	panic(r)
}

// Panic/Fatal on the package singleton.
func Panic(format string, args ...interface{}) { defaultLogger().Panic(format, args...) }

// Fatal on the package singleton.
func Fatal(format string, args ...interface{}) { defaultLogger().Fatal(format, args...) }
