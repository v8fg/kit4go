package disjointset_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/disjointset"
)

func TestEmpty(t *testing.T) {
	uf := disjointset.New[int]()
	require.Equal(t, 0, uf.Count())
}

func TestAddAndCount(t *testing.T) {
	uf := disjointset.New[int]()
	uf.Add(1)
	uf.Add(2)
	uf.Add(3)
	require.Equal(t, 3, uf.Count())
	uf.Add(1) // dedup
	require.Equal(t, 3, uf.Count())
}

func TestAutoRegister(t *testing.T) {
	uf := disjointset.New[int]()
	// Find on an unknown element auto-registers it as a singleton.
	require.Equal(t, 42, uf.Find(42))
	require.Equal(t, 1, uf.Count())
	require.Equal(t, 1, uf.Size(42))
}

func TestUnionAndConnected(t *testing.T) {
	uf := disjointset.New[int]()
	uf.Union(1, 2)
	uf.Union(3, 4)

	require.True(t, uf.Connected(1, 2))
	require.True(t, uf.Connected(3, 4))
	require.False(t, uf.Connected(1, 3))
	require.False(t, uf.Connected(1, 4))
	require.Equal(t, 2, uf.Count()) // two sets

	// Merge the two sets.
	uf.Union(2, 3)
	require.True(t, uf.Connected(1, 4))
	require.Equal(t, 1, uf.Count())
	require.Equal(t, 4, uf.Size(1))
}

func TestUnionIdempotent(t *testing.T) {
	uf := disjointset.New[int]()
	uf.Union(1, 2)
	before := uf.Count()
	uf.Union(1, 2) // already merged
	uf.Union(2, 1) // reversed, still same set
	require.Equal(t, before, uf.Count())
}

func TestUnionByRankSwap(t *testing.T) {
	// Force union-by-rank to swap (x's root shorter than y's): merge two rank-1
	// trees into a rank-2 root, then union a rank-1 tree INTO it from the x side.
	uf := disjointset.New[int]()
	uf.Union(1, 2) // {1,2} rank 1
	uf.Union(3, 4) // {3,4} rank 1
	uf.Union(1, 3) // equal ranks → {1,2,3,4} root 1, rank 2
	uf.Union(5, 6) // {5,6} rank 1

	// Union(5, 1): rank[Find(5)=5]=1 < rank[Find(1)=1]=2 → swap attaches 5 under 1.
	uf.Union(5, 1)
	require.True(t, uf.Connected(6, 2))
	require.Equal(t, 6, uf.Size(6))
	require.Equal(t, 1, uf.Count())
}

func TestPathCompression(t *testing.T) {
	// Chain 0-1-2-3-4 via successive unions; Find must flatten the tree.
	uf := disjointset.New[int]()
	for i := range 4 {
		uf.Union(i, i+1)
	}
	require.Equal(t, 1, uf.Count())
	// All elements share a root after path compression.
	root := uf.Find(0)
	for i := range 5 {
		require.Equal(t, root, uf.Find(i))
	}
}

func TestSize(t *testing.T) {
	uf := disjointset.New[string]()
	uf.Union("a", "b")
	uf.Union("a", "c")
	require.Equal(t, 3, uf.Size("a"))
	require.Equal(t, 3, uf.Size("b"))
	require.Equal(t, 3, uf.Size("c"))
}

func TestConnectivityScenario(t *testing.T) {
	// Edges in an undirected graph; count connected components.
	edges := [][2]int{{0, 1}, {1, 2}, {3, 4}} // components: {0,1,2}, {3,4}, {5}
	uf := disjointset.New[int]()
	for _, e := range edges {
		uf.Union(e[0], e[1])
	}
	uf.Add(5) // isolated node = its own component
	require.Equal(t, 3, uf.Count())
	require.True(t, uf.Connected(0, 2))
	require.False(t, uf.Connected(0, 3))
	require.False(t, uf.Connected(4, 5))
}

func TestReset(t *testing.T) {
	uf := disjointset.New[int]()
	uf.Union(1, 2)
	uf.Union(3, 4)
	require.Equal(t, 2, uf.Count())
	uf.Reset()
	require.Equal(t, 0, uf.Count())
	// Reusable after reset.
	uf.Union(1, 2)
	require.Equal(t, 1, uf.Count())
	require.True(t, uf.Connected(1, 2))
}
