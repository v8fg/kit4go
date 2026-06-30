//go:build unix

package shutdown

import (
	"os"
	"syscall"
)

// unixSIGTERM is SIGTERM on unix platforms; nil elsewhere (see signal_other.go).
var unixSIGTERM os.Signal = syscall.SIGTERM
