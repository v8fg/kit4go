package set_test

import (
	"testing"

	"github.com/v8fg/kit4go/set"
)

// FuzzAddContains encodes the core set invariant: every value that has been
// Added MUST be found by Contains — no false negatives. Catches any map-state
// corruption. E10 invariant-encoding fuzz target.
func FuzzAddContains(f *testing.F) {
	f.Add(1, 2, 3)
	f.Add(0, 0, 0)
	f.Add(-1, 100, 50)
	f.Fuzz(func(t *testing.T, a, b, c int) {
		s := set.New(a, b, c)
		for _, v := range []int{a, b, c} {
			if !s.Contains(v) {
				t.Errorf("Contains(%d) = false after Add (false negative)", v)
			}
		}
	})
}
