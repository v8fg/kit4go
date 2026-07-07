package maxprocs_test

import (
	"log"

	"github.com/v8fg/kit4go/maxprocs"
)

// ExampleSet shows the explicit opt-in API. Call Set once near main; it is
// idempotent so a repeat call is safe. Pass nil for a silent apply, or a
// Logger (e.g. log.Printf) to have automaxprocs log the resolved GOMAXPROCS:
//
//	maxprocs.Set(nil)         // silent
//	maxprocs.Set(log.Printf)  // log the status line
//
// The resolved value depends on the cgroup CPU quota, so this example asserts
// no printed output rather than a specific GOMAXPROCS count.
func ExampleSet() {
	maxprocs.Set(nil)        // silent: no status line
	maxprocs.Set(log.Printf) // the status line goes to the stdlib logger
	// Output:
}
