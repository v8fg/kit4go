package consistenthash_test

import (
	"testing"

	"github.com/v8fg/kit4go/consistenthash"
)

// FuzzGetN verifies the rendezvous-hashing multi-pick contract for arbitrary
// node sets and request counts:
//   - GetN returns min(n, nodeCount) DISTINCT nodes (the selection sort must
//     not emit a node twice and must respect the requested count),
//   - its first element is the argmax, i.e. matches Get(key) (top-1 == top-N[0]).
//
// A regression in the GetN selection sort (off-by-one range, a swap that
// duplicates a node, or a top-n that disagrees with the argmax in Get) would
// break one of these. Nodes are distinct single-byte IDs derived from the
// fuzzer blob; the id func is the identity so dedup is by the byte itself.
func FuzzGetN(f *testing.F) {
	f.Add([]byte("abc"), "key1", 2)
	f.Add([]byte("nodes"), "rk", 5)
	f.Add([]byte("xy"), "k", 0) // n=0 -> nil
	f.Add([]byte("z"), "k", 3)  // n > nodeCount

	f.Fuzz(func(t *testing.T, nodeBlob []byte, key string, n int) {
		seen := map[byte]bool{}
		var nodes []string
		for _, b := range nodeBlob {
			if !seen[b] {
				seen[b] = true
				nodes = append(nodes, string(b))
			}
		}
		if len(nodes) == 0 || n < 0 || n > 512 {
			t.Skip("empty node set or out-of-range n")
		}
		m := consistenthash.New[string](func(s string) string { return s },
			consistenthash.WithNodes(nodes...))

		got := m.GetN(key, n)
		want := n
		if want > len(nodes) {
			want = len(nodes)
		}
		if len(got) != want {
			t.Fatalf("GetN len=%d want %d (nodes=%d n=%d)", len(got), want, len(nodes), n)
		}
		// Distinct nodes.
		dup := map[string]bool{}
		for _, g := range got {
			if dup[g] {
				t.Fatalf("GetN returned duplicate node %q (n=%d)", g, n)
			}
			dup[g] = true
		}
		// Top-1 must agree with Get (both are the argmax of HRW score).
		if want > 0 {
			top, ok := m.Get(key)
			if !ok {
				t.Fatalf("Get returned ok=false with %d nodes", len(nodes))
			}
			if top != got[0] {
				t.Fatalf("GetN[0]=%q disagrees with Get=%q (nodes=%v key=%q)", got[0], top, nodes, key)
			}
		}
	})
}
