package countmin

import (
	"testing"
)

// FuzzNeverUnderCount encodes the core Count-Min Sketch invariant: Estimate
// NEVER under-counts (it is always >= the true count for a key, single-goroutine
// — collisions can only over-count, never under). The sketch is small (width 64)
// to force cross-key collisions and exercise the over-estimate path; the guard
// is the lower bound. Counts are bounded to keep the true count below the
// uint64 wrap edge so a wrap can't mask an under-count. E10 invariant-encoding
// fuzz target.
func FuzzNeverUnderCount(f *testing.F) {
	f.Add("a", "b", uint64(1), uint64(2))
	f.Add("same", "same", uint64(3), uint64(4)) // colliding key — counts add
	f.Add("", "", uint64(0), uint64(0))
	f.Fuzz(func(t *testing.T, ka, kb string, na, nb uint64) {
		na %= 1000
		nb %= 1000
		cm := New(64, 3)
		cm.Add([]byte(ka), na)
		cm.Add([]byte(kb), nb)

		trueA := na
		if ka == kb {
			trueA = na + nb
		}
		if got := cm.Estimate([]byte(ka)); got < trueA {
			t.Errorf("Estimate(%q) = %d < true %d (under-count — countmin invariant violated)", ka, got, trueA)
		}
	})
}
