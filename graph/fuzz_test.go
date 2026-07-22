package graph_test

import (
	"errors"
	"testing"

	"github.com/v8fg/kit4go/graph"
)

// FuzzTopoSortValid encodes the central TopoSort contract: for any graph,
// TopoSort either returns ErrCycle (a cycle makes ordering impossible) or a
// valid topological order in which every edge from->to places from before to.
// The raw byte blob is decoded as a stream of edges over a small node space (so
// cycles and duplicates are common). E10 invariant-encoding fuzz target.
func FuzzTopoSortValid(f *testing.F) {
	f.Add([]byte{0, 1, 1, 2, 2, 0})       // a cycle (0→1→2→0)
	f.Add([]byte{0, 1, 0, 2, 1, 3, 2, 3}) // a diamond DAG
	f.Add([]byte{0, 1, 1, 2, 3, 4})       // two disjoint chains
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 2 {
			t.Skip()
		}
		g := graph.New[int]()
		for i := 0; i+1 < len(data); i += 2 {
			g.AddEdge(int(data[i])%8, int(data[i+1])%8)
		}

		order, err := g.TopoSort()
		if err != nil {
			if !errors.Is(err, graph.ErrCycle) {
				t.Errorf("unexpected non-cycle error: %v", err)
			}
			return
		}

		// Every edge must respect the order: from appears before to.
		pos := make(map[int]int, len(order))
		for i, n := range order {
			pos[n] = i
		}
		for _, from := range g.Nodes() {
			for _, to := range g.Neighbors(from) {
				if pos[from] >= pos[to] {
					t.Errorf("edge %d->%d violates topological order %v", from, to, order)
				}
			}
		}
		// HasCycle must agree with TopoSort's cycle verdict.
		if g.HasCycle() {
			t.Errorf("HasCycle=true but TopoSort returned an order: %v", order)
		}
	})
}
