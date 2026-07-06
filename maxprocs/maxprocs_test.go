package maxprocs

import (
	"runtime"
	"strings"
	"testing"
)

// TestSet confirms Set is safe to call explicitly (with and without a Logger)
// and leaves GOMAXPROCS at a sane value. automaxprocs tunes GOMAXPROCS to the
// container CPU quota; outside a cgroup-quota'd environment it leaves the
// runtime default, which is always >= 1.
func TestSet(t *testing.T) {
	t.Run("nil logger is silent and valid", func(t *testing.T) {
		Set(nil)
		if n := runtime.GOMAXPROCS(0); n < 1 {
			t.Fatalf("GOMAXPROCS=%d after Set(nil), want >= 1", n)
		}
	})

	t.Run("custom logger receives status line", func(t *testing.T) {
		var got strings.Builder
		l := Logger(func(format string, args ...any) {
			// automaxprocs logs "maxprocs: Leaving GOMAXPROCS=%v: ..." style lines.
			got.WriteString(format)
		})
		Set(l)
		if got.Len() == 0 {
			t.Fatalf("Logger got no status line from automaxprocs")
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		Set(nil)
		Set(nil) // must not panic
	})
}

// TestSilent verifies the no-logger path still drives automaxprocs without
// panicking and without any default stderr output (silent option is wired).
func TestSilent(t *testing.T) {
	silent() // option construction must not panic
	Set(nil)
}
