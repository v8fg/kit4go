package maxprocs_test

import (
	"log"

	"github.com/v8fg/kit4go/maxprocs"
)

// ExampleSet shows the explicit opt-in API. Pass nil for a silent apply, or a
// Logger (e.g. log.Printf) to log the resolved value:
//
//	maxprocs.Set(nil)
//	maxprocs.Set(log.Printf)
func ExampleSet() {
	maxprocs.Set(nil)
	maxprocs.Set(log.Printf)
	// Output:
}
