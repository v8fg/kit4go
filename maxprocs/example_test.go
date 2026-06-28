package maxprocs_test

import "github.com/v8fg/kit4go/maxprocs"

// ExampleSet shows the explicit form. The package also runs Set automatically
// via init(), so a blank import is enough for most programs:
//
//	import _ "github.com/v8fg/kit4go/maxprocs"
//
// Set's diagnostic output goes to the standard logger (stderr), not stdout.
func ExampleSet() {
	maxprocs.Set()
	// Output:
}
