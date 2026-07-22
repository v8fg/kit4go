package disjointset_test

import (
	"testing"

	"github.com/v8fg/kit4go/disjointset"
)

// FuzzUnionConnectivity encodes the core union-find contract: after any
// sequence of Union operations, Connected must be an EQUIVALENCE RELATION over
// the elements seen — reflexive, symmetric, and transitive. (Count is NOT
// asserted to decrease: Union auto-registers new elements, so a fresh element
// legitimately raises Count.) The byte blob is a stream of (x,y) union pairs
// over a small element space. E10 invariant-encoding fuzz target.
func FuzzUnionConnectivity(f *testing.F) {
	f.Add([]byte{0, 1, 1, 2, 0, 2})
	f.Add([]byte{0, 1, 2, 3, 4, 5})
	f.Add([]byte{0, 1, 1, 0, 2, 2})
	f.Fuzz(func(t *testing.T, data []byte) {
		uf := disjointset.New[int]()
		seen := map[int]struct{}{}
		for i := 0; i+1 < len(data); i += 2 {
			x := int(data[i]) % 12
			y := int(data[i+1]) % 12
			uf.Union(x, y)
			seen[x] = struct{}{}
			seen[y] = struct{}{}
			// Union makes its operands connected — a free, immediate check.
			if !uf.Connected(x, y) {
				t.Errorf("Union(%d,%d): operands not connected afterwards", x, y)
			}
		}

		elems := make([]int, 0, len(seen))
		for e := range seen {
			elems = append(elems, e)
		}

		// Connected must be an equivalence relation over the seen elements.
		for _, a := range elems {
			if !uf.Connected(a, a) { // reflexive
				t.Errorf("Connected(%d,%d)=false (not reflexive)", a, a)
			}
			for _, b := range elems {
				if uf.Connected(a, b) != uf.Connected(b, a) { // symmetric
					t.Errorf("Connected(%d,%d) != Connected(%d,%d)", a, b, b, a)
				}
				if !uf.Connected(a, b) {
					continue
				}
				for _, c := range elems {
					if uf.Connected(b, c) && !uf.Connected(a, c) { // transitive
						t.Errorf("not transitive: %d~%d and %d~%d but not %d~%d", a, b, b, c, a, c)
					}
				}
			}
		}
	})
}
