package omap_test

import (
	"slices"
	"testing"

	"github.com/v8fg/kit4go/omap"
)

// FuzzInsertionOrder encodes the omap contract: after any sequence of Set (which
// appends new keys, updates existing in place) and Delete, Keys() returns the
// remaining keys in first-add order. The byte blob is a stream of (op,key)
// pairs (op even = Set, op odd = Delete); a parallel reference slice tracks the
// expected order. E10 invariant-encoding fuzz target.
func FuzzInsertionOrder(f *testing.F) {
	f.Add([]byte{0, 0, 0, 1, 1, 0, 0, 2}) // Set(0), Set(1), Del(0), Set(2)
	f.Fuzz(func(t *testing.T, data []byte) {
		m := omap.New[int, int]()
		var order []int // expected first-add order of present keys
		present := map[int]bool{}

		for i := 0; i+1 < len(data); i += 2 {
			op := data[i]
			k := int(data[i+1]) % 12
			if op%2 == 0 { // Set
				m.Set(k, int(op))
				if !present[k] {
					order = append(order, k)
					present[k] = true
				}
			} else { // Delete
				if m.Delete(k) {
					order = slices.DeleteFunc(order, func(x int) bool { return x == k })
					present[k] = false
				}
			}
		}

		if got, want := m.Keys(), order; !slices.Equal(got, want) {
			t.Errorf("Keys order mismatch:\n got %v\nwant %v", got, want)
		}
		if m.Len() != len(order) {
			t.Errorf("Len=%d want %d", m.Len(), len(order))
		}
	})
}
