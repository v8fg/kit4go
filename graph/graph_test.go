package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/graph"
)

func TestEmpty(t *testing.T) {
	g := graph.New[int]()
	require.Equal(t, 0, g.Len())
	require.Empty(t, g.Nodes())
	order, err := g.TopoSort()
	require.NoError(t, err)
	require.Empty(t, order)
	require.False(t, g.HasCycle())
}

func TestAddNodeAndEdge(t *testing.T) {
	g := graph.New[string]()
	g.AddNode("a")
	g.AddNode("a") // dedup no-op
	require.Equal(t, 1, g.Len())
	require.True(t, g.HasNode("a"))
	require.False(t, g.HasNode("b"))

	g.AddEdge("a", "b") // auto-adds b
	require.True(t, g.HasNode("b"))
	require.True(t, g.HasEdge("a", "b"))
	require.False(t, g.HasEdge("b", "a"))
	require.Equal(t, 2, g.Len())

	// Duplicate edge collapsed.
	g.AddEdge("a", "b")
	require.Equal(t, []string{"b"}, g.Neighbors("a"))
}

func TestRemoveEdge(t *testing.T) {
	g := graph.New[int]()
	g.AddEdge(1, 2)
	g.AddEdge(1, 3)
	require.True(t, g.HasEdge(1, 2))

	g.RemoveEdge(1, 2)
	require.False(t, g.HasEdge(1, 2))
	require.True(t, g.HasEdge(1, 3))
	require.Equal(t, []int{3}, g.Neighbors(1))

	// Remove absent edge is a no-op.
	g.RemoveEdge(1, 99)
	require.Equal(t, []int{3}, g.Neighbors(1))
}

func TestNeighborsCopy(t *testing.T) {
	g := graph.New[int]()
	g.AddEdge(1, 2)
	nb := g.Neighbors(1)
	nb[0] = 999 // mutating the copy must not affect the graph
	require.True(t, g.HasEdge(1, 2))
}

func TestNodesInsertionOrder(t *testing.T) {
	g := graph.New[string]()
	g.AddNode("c")
	g.AddNode("a")
	g.AddNode("b")
	require.Equal(t, []string{"c", "a", "b"}, g.Nodes())
}

func TestBFS(t *testing.T) {
	//   a -> b -> d
	//   a -> c -> d
	g := graph.New[string]()
	g.AddEdge("a", "b")
	g.AddEdge("a", "c")
	g.AddEdge("b", "d")
	g.AddEdge("c", "d")

	require.Equal(t, []string{"a", "b", "c", "d"}, g.BFS("a"))
	// Start at a non-source node.
	require.Equal(t, []string{"b", "d"}, g.BFS("b"))
	// Missing start.
	require.Nil(t, g.BFS("zzz"))
}

func TestDFS(t *testing.T) {
	g := graph.New[string]()
	g.AddEdge("a", "b")
	g.AddEdge("a", "c")
	g.AddEdge("b", "d")
	g.AddEdge("c", "d")

	// Pre-order DFS: a, then its first successor chain first.
	require.Equal(t, []string{"a", "b", "d", "c"}, g.DFS("a"))
	require.Equal(t, []string{"b", "d"}, g.DFS("b"))
	require.Nil(t, g.DFS("zzz"))
}

func TestDFSSharedSuccessor(t *testing.T) {
	// c is reachable from a directly AND via b. The iterative DFS pushes c
	// twice (once before it is popped), so the "already-visited when popped"
	// continue branch is exercised — and c still appears exactly once.
	g := graph.New[string]()
	g.AddEdge("a", "b")
	g.AddEdge("a", "c")
	g.AddEdge("b", "c")

	require.Equal(t, []string{"a", "b", "c"}, g.DFS("a"))
}

func TestTopoSortDAG(t *testing.T) {
	// Diamond: a->b, a->c, b->d, c->d
	g := graph.New[string]()
	g.AddEdge("a", "b")
	g.AddEdge("a", "c")
	g.AddEdge("b", "d")
	g.AddEdge("c", "d")

	order, err := g.TopoSort()
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c", "d"}, order)
	require.True(t, isTopoOrder(g, order))
	require.False(t, g.HasCycle())
}

func TestTopoSortDisconnected(t *testing.T) {
	// Two disjoint chains: a->b, c->d
	g := graph.New[string]()
	g.AddEdge("a", "b")
	g.AddEdge("c", "d")

	order, err := g.TopoSort()
	require.NoError(t, err)
	require.Len(t, order, 4)
	require.True(t, isTopoOrder(g, order))
}

func TestTopoSortLinearChain(t *testing.T) {
	g := graph.New[int]()
	g.AddEdge(1, 2)
	g.AddEdge(2, 3)
	g.AddEdge(3, 4)
	order, err := g.TopoSort()
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3, 4}, order)
}

func TestCycle(t *testing.T) {
	g := graph.New[string]()
	g.AddEdge("a", "b")
	g.AddEdge("b", "c")
	g.AddEdge("c", "a") // back-edge → cycle

	_, err := g.TopoSort()
	require.ErrorIs(t, err, graph.ErrCycle)
	require.True(t, g.HasCycle())
}

func TestSelfLoopIsCycle(t *testing.T) {
	g := graph.New[int]()
	g.AddEdge(1, 1)
	require.True(t, g.HasCycle())
	_, err := g.TopoSort()
	require.ErrorIs(t, err, graph.ErrCycle)
}

func TestNoCycleAfterBreak(t *testing.T) {
	g := graph.New[int]()
	g.AddEdge(1, 2)
	g.AddEdge(2, 3)
	g.AddEdge(3, 1) // cycle
	require.True(t, g.HasCycle())

	g.RemoveEdge(3, 1) // break the cycle
	require.False(t, g.HasCycle())
	order, err := g.TopoSort()
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3}, order)
}

func TestBFSDiamondReachableSet(t *testing.T) {
	// From "a", every node must be reachable regardless of visit tiebreak.
	g := graph.New[int]()
	g.AddEdge(1, 2)
	g.AddEdge(1, 3)
	g.AddEdge(2, 4)
	g.AddEdge(3, 4)
	g.AddEdge(4, 5)

	reached := map[int]bool{}
	for _, n := range g.BFS(1) {
		reached[n] = true
	}
	for _, want := range []int{1, 2, 3, 4, 5} {
		require.True(t, reached[want], "BFS must reach %d", want)
	}
}

// isTopoOrder verifies every edge from->to has from before to in the order.
func isTopoOrder[T comparable](g *graph.Graph[T], order []T) bool {
	pos := make(map[T]int, len(order))
	for i, n := range order {
		pos[n] = i
	}
	for _, from := range g.Nodes() {
		for _, to := range g.Neighbors(from) {
			if pos[from] >= pos[to] {
				return false
			}
		}
	}
	return true
}
