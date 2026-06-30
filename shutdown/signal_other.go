//go:build !unix

package shutdown

import "os"

// unixSIGTERM is nil on non-unix platforms (no SIGTERM). WithSignal filters it
// out so the default set stays valid.
var unixSIGTERM os.Signal = nil
