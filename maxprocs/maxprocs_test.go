package maxprocs

import (
	"runtime"
	"testing"
)

// TestSet confirms Set is safe to call explicitly (it is also run once via
// init() at import) and leaves GOMAXPROCS at a sane value. automaxprocs tunes
// GOMAXPROCS to the container CPU quota; outside a cgroup-quota'd environment
// it leaves the runtime default, which is always >= 1.
func TestSet(t *testing.T) {
	Set()
	if n := runtime.GOMAXPROCS(0); n < 1 {
		t.Fatalf("GOMAXPROCS=%d after Set, want >= 1", n)
	}
	// Set is documented as idempotent; a second call must not panic.
	Set()
}
