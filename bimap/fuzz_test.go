package bimap_test

import (
	"testing"

	"github.com/v8fg/kit4go/bimap"
)

// FuzzBidirectionalLookup encodes the invariant: after Insert(k, v), both
// Get(k) == v and GetKey(v) == k hold. E10 invariant-encoding fuzz target.
func FuzzBidirectionalLookup(f *testing.F) {
	f.Add(1, 100)
	f.Add(0, 0)
	f.Add(-5, 999)
	f.Fuzz(func(t *testing.T, k, v int) {
		bm := bimap.New[int, int]()
		// If k==v it's still a valid one-to-one pair.
		if err := bm.Insert(k, v); err != nil {
			return // duplicate on same k/v pair — skip
		}
		got, ok := bm.Get(k)
		if !ok || got != v {
			t.Errorf("Get(%d) = (%d, %v), want (%d, true)", k, got, ok, v)
		}
		gotK, ok := bm.GetKey(v)
		if !ok || gotK != k {
			t.Errorf("GetKey(%d) = (%d, %v), want (%d, true)", v, gotK, ok, k)
		}
	})
}
