package bloom

import (
	"testing"
)

// FuzzAddTestNoFalseNegative encodes the core Bloom-filter invariant: a key
// that has been Added MUST Test true (Bloom filters have no false negatives —
// only false positives). Any index/overflow/buffer bug that makes an added key
// Test false violates the contract. E10 invariant-encoding fuzz target.
func FuzzAddTestNoFalseNegative(f *testing.F) {
	f.Add([]byte("hello"), []byte("world"), []byte(""))
	f.Add([]byte("a"), []byte("a"), []byte("a"))      // duplicate key
	f.Add([]byte{}, []byte{0x00}, []byte{0xff, 0xfe}) // empty + binary keys
	f.Fuzz(func(t *testing.T, a, b, c []byte) {
		fltr := New(100, 0.01)
		keys := [][]byte{a, b, c}
		for _, k := range keys {
			fltr.Add(k)
		}
		for _, k := range keys {
			if !fltr.Test(k) {
				t.Errorf("Test(%q) = false after Add (false negative — bloom invariant violated)", string(k))
			}
		}
	})
}
