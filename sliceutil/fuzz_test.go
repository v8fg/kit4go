package sliceutil_test

import (
	"testing"

	"github.com/v8fg/kit4go/sliceutil"
)

// FuzzChunkReassembles encodes the Chunk contract: flattening the chunks back
// together must reconstruct the original slice exactly, and no chunk may exceed
// the requested size (the last may be shorter). E10 invariant-encoding fuzz.
func FuzzChunkReassembles(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5}, 2)
	f.Add([]byte{10, 20, 30}, 5)
	f.Add([]byte{}, 3)
	f.Fuzz(func(t *testing.T, data []byte, size int) {
		if size <= 0 || len(data) == 0 {
			t.Skip()
		}
		s := make([]int, len(data))
		for i, b := range data {
			s[i] = int(b)
		}

		chunks := sliceutil.Chunk(s, size)

		// No chunk exceeds size; total element count is preserved.
		total := 0
		for i, c := range chunks {
			if len(c) > size {
				t.Errorf("chunk %d len %d > size %d", i, len(c), size)
			}
			total += len(c)
		}
		if total != len(s) {
			t.Errorf("chunks lost/duplicated elements: got %d want %d", total, len(s))
		}

		// Flattening reconstructs the original.
		flat := make([]int, 0, len(s))
		for _, c := range chunks {
			flat = append(flat, c...)
		}
		for i := range s {
			if flat[i] != s[i] {
				t.Errorf("reassembled slice differs at %d: got %d want %d", i, flat[i], s[i])
				break
			}
		}
	})
}
