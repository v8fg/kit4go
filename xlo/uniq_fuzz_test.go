package xlo_test

import (
	"testing"

	"github.com/v8fg/kit4go/xlo"
)

// FuzzUniq validates the Uniq dedup for arbitrary byte slices: the result must
// contain no duplicates, preserve first-occurrence order, and include exactly
// the distinct elements of the input.
func FuzzUniq(f *testing.F) {
	f.Add([]byte{1, 2, 3, 1, 2, 1})
	f.Add([]byte{})
	f.Add([]byte{1, 1, 1, 1})
	f.Add([]byte{255, 0, 255, 1, 0})

	f.Fuzz(func(t *testing.T, input []byte) {
		result := xlo.Uniq(input)

		// Build expected: distinct elements in first-occurrence order.
		seen := map[byte]bool{}
		var expected []byte
		for _, v := range input {
			if !seen[v] {
				seen[v] = true
				expected = append(expected, v)
			}
		}

		// Result must have no duplicates.
		dupCheck := map[byte]bool{}
		for _, v := range result {
			if dupCheck[v] {
				t.Fatalf("Uniq produced duplicate %v", v)
			}
			dupCheck[v] = true
		}

		// Length and order must match expected.
		if len(result) != len(expected) {
			t.Fatalf("Uniq len=%d want %d (input=%v result=%v)", len(result), len(expected), input, result)
		}
		for i := range result {
			if result[i] != expected[i] {
				t.Fatalf("Uniq order mismatch at %d: got %v want %v", i, result[i], expected[i])
			}
		}
	})
}
