package maxprocs

import (
	automaxprocs "go.uber.org/automaxprocs/maxprocs"
)

// Logger is an optional diagnostic sink passed to Set. A nil Logger discards
// automaxprocs's status line (no default stderr output). Pass log.Printf or a
// log4go bridge if you want the resolved value logged.
type Logger func(format string, args ...any)

// Set tunes GOMAXPROCS to the container CPU quota. It must be called
// explicitly, once near main; this package no longer mutates global state at
// import:
//
//	import "github.com/v8fg/kit4go/maxprocs"
//
//	func main() {
//	    maxprocs.Set(nil) // nil Logger = silent
//	    // or: maxprocs.Set(log.Printf)
//	}
//
// Set is idempotent; calling it more than once is safe.
//
// The returned restore func is intentionally discarded: in a long-running
// server GOMAXPROCS is set once and never rolled back.
func Set(log Logger) {
	var opt automaxprocs.Option
	if log != nil {
		opt = automaxprocs.Logger(log)
	} else {
		opt = silent()
	}
	_, _ = automaxprocs.Set(opt)
}

// silent returns an automaxprocs.Option that suppresses the status log line by
// routing it to a no-op writer. automaxprocs has no built-in "no log" mode, so
// we hand it a func that drops the formatted string.
func silent() automaxprocs.Option {
	return automaxprocs.Logger(func(string, ...any) {})
}
