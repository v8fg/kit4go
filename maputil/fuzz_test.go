package maputil_test

import (
	"testing"

	"github.com/v8fg/kit4go/maputil"
)

// FuzzInvertRoundTrip encodes the Invert contract: when a map's values are all
// distinct, Invert is a bijection and Invert(Invert(m)) reconstructs m exactly.
// The fuzzer supplies keys; values are assigned uniquely so the precondition
// (distinct values) always holds. E10 invariant-encoding fuzz target.
func FuzzInvertRoundTrip(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 0, 4})
	f.Fuzz(func(t *testing.T, data []byte) {
		m := map[int]int{}
		nextVal := 0
		for _, b := range data {
			k := int(b) % 16
			if _, ok := m[k]; !ok {
				m[k] = nextVal // a unique value per distinct key
				nextVal++
			}
		}
		if len(m) == 0 {
			return
		}

		inv := maputil.Invert(m)     // value -> key
		round := maputil.Invert(inv) // back to key -> value

		if !maputil.Equal(round, m) {
			t.Errorf("Invert(Invert) != original:\n got %v\nwant %v", round, m)
		}
	})
}
