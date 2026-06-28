package maxprocs

import (
	"log"

	"go.uber.org/automaxprocs/maxprocs"
)

// Set tunes GOMAXPROCS to the container CPU quota, logging the change via the
// standard logger (interop with log4go is left to the caller — set
// log4go.SetOutput on the standard logger if you want it captured). It runs at
// package import via init(), so a blank import is enough:
//
//	import _ "github.com/v8fg/kit4go/maxprocs"
//
// Calling Set again is safe (idempotent).
func Set() {
	// automaxprocs.Set returns a restore func + error; in a long-running server
	// we set once at import and never restore, and it reports issues via the
	// Logger — so both are intentionally unused.
	_, _ = maxprocs.Set(maxprocs.Logger(log.Printf))
}

func init() {
	Set()
}
