package graph_test

import (
	"testing"

	"github.com/v8fg/kit4go/graph"
)

// buildChain builds a linear chain 0->1->2->...->n-1.
func buildChain(n int) *graph.Graph[int] {
	g := graph.New[int]()
	for i := range n - 1 {
		g.AddEdge(i, i+1)
	}
	return g
}

// buildDAG builds a layered DAG: each layer L nodes, each node in layer i
// connects to every node in layer i+1.
func buildDAG(layers, width int) *graph.Graph[int] {
	g := graph.New[int]()
	node := func(layer, idx int) int { return layer*width + idx }
	for l := range layers - 1 {
		for a := range width {
			for b := range width {
				g.AddEdge(node(l, a), node(l+1, b))
			}
		}
	}
	return g
}

func BenchmarkAddEdge(b *testing.B) {
	g := graph.New[int]()
	b.ResetTimer()
	for b.Loop() {
		g.AddEdge(b.N, b.N+1)
	}
}

func BenchmarkBFS(b *testing.B) {
	g := buildChain(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = g.BFS(0)
	}
}

func BenchmarkDFS(b *testing.B) {
	g := buildChain(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = g.DFS(0)
	}
}

func BenchmarkTopoSort(b *testing.B) {
	g := buildDAG(20, 10) // 200 nodes, ~1900 edges
	b.ResetTimer()
	for b.Loop() {
		_, _ = g.TopoSort()
	}
}

func BenchmarkHasCycle(b *testing.B) {
	g := buildDAG(20, 10)
	b.ResetTimer()
	for b.Loop() {
		_ = g.HasCycle()
	}
}
