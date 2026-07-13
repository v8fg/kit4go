package mathx_test

import (
	"testing"

	"github.com/v8fg/kit4go/mathx"
)

// FuzzClampRange encodes the invariant: Clamp(v, lo, hi) always returns a value
// in [lo, hi]. E10 invariant-encoding fuzz target.
func FuzzClampRange(f *testing.F) {
	f.Add(50.0, 0.0, 100.0)
	f.Add(-10.0, 0.0, 100.0)
	f.Add(200.0, 0.0, 100.0)
	f.Fuzz(func(t *testing.T, v, lo, hi float64) {
		// Skip invalid ranges — Clamp panics on lo > hi (documented).
		if lo > hi {
			return
		}
		result := mathx.Clamp(v, lo, hi)
		if result < lo || result > hi {
			t.Errorf("Clamp(%g, %g, %g) = %g, outside [%g, %g]", v, lo, hi, result, lo, hi)
		}
	})
}
