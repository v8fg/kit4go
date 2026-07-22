package multimap_test

import (
	"slices"
	"testing"

	"github.com/v8fg/kit4go/multimap"
)

// FuzzAddCountConsistency encodes the multimap contract: after a stream of Add
// operations, Count(k) equals the number of values added under k and Get(k)
// returns them in insertion order. E10 invariant-encoding fuzz target.
func FuzzAddCountConsistency(f *testing.F) {
	f.Add([]byte{0, 1, 0, 2, 0, 1, 1, 3}) // Add(0,1),(0,2),(0,1),(1,3)
	f.Fuzz(func(t *testing.T, data []byte) {
		mm := multimap.New[int, int]()
		ref := map[int][]int{}

		for i := 0; i+1 < len(data); i += 2 {
			k := int(data[i]) % 8
			v := int(data[i+1]) % 8
			mm.Add(k, v)
			ref[k] = append(ref[k], v)
		}

		for k, vals := range ref {
			if got := mm.Count(k); got != len(vals) {
				t.Errorf("Count(%d)=%d want %d", k, got, len(vals))
			}
			if got := mm.Get(k); !slices.Equal(got, vals) {
				t.Errorf("Get(%d)=%v want %v (insertion order)", k, got, vals)
			}
		}
	})
}
