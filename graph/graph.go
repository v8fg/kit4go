// Package graph provides a generic directed graph with traversal and analysis:
// BFS, DFS, topological sort, and cycle detection. Backed by an adjacency list
// keyed by comparable node values.
//
// Edges are stored in insertion order, so traversals (BFS/DFS/TopoSort) are
// deterministic — the same graph always yields the same result. Pure standard
// library.
//
// Typical use: dependency resolution — TopoSort orders nodes so every edge
// points from an earlier node to a later one; a cycle (ErrCycle) means the
// dependencies are unsatisfiable.
package graph

import (
	"errors"
	"slices"
)

// ErrCycle is returned by TopoSort when the graph contains a cycle, making a
// topological ordering impossible.
var ErrCycle = errors.New("graph: contains a cycle")

// Graph is a directed graph over comparable node keys.
type Graph[T comparable] struct {
	adj   map[T][]T // node -> successors (insertion order, deduped)
	order []T       // all nodes in insertion order (deduped)
}

// New creates an empty directed Graph.
func New[T comparable]() *Graph[T] {
	return &Graph[T]{adj: make(map[T][]T)}
}

func (g *Graph[T]) ensure(n T) {
	if _, ok := g.adj[n]; !ok {
		g.adj[n] = nil
		g.order = append(g.order, n)
	}
}

// AddNode adds an isolated node. No-op if it already exists.
func (g *Graph[T]) AddNode(n T) { g.ensure(n) }

// AddEdge adds a directed edge from->to, registering both nodes. Duplicate edges
// are collapsed. Self-loops (from==to) are permitted and count as a cycle.
func (g *Graph[T]) AddEdge(from, to T) {
	g.ensure(from)
	g.ensure(to)
	if slices.Contains(g.adj[from], to) {
		return
	}
	g.adj[from] = append(g.adj[from], to)
}

// RemoveEdge removes the directed edge from->to. No-op if absent.
func (g *Graph[T]) RemoveEdge(from, to T) {
	edges := g.adj[from]
	for i, e := range edges {
		if e == to {
			g.adj[from] = append(edges[:i], edges[i+1:]...)
			return
		}
	}
}

// HasEdge reports whether the directed edge from->to exists.
func (g *Graph[T]) HasEdge(from, to T) bool {
	return slices.Contains(g.adj[from], to)
}

// HasNode reports whether n is in the graph (including isolated nodes).
func (g *Graph[T]) HasNode(n T) bool {
	_, ok := g.adj[n]
	return ok
}

// Nodes returns all nodes in insertion order (a copy).
func (g *Graph[T]) Nodes() []T {
	return append([]T(nil), g.order...)
}

// Neighbors returns the direct successors of n (a copy), in insertion order.
// Returns nil if n is not in the graph.
func (g *Graph[T]) Neighbors(n T) []T {
	return append([]T(nil), g.adj[n]...)
}

// Len returns the number of nodes.
func (g *Graph[T]) Len() int { return len(g.order) }

// BFS traverses breadth-first from start, returning nodes in visit order.
// Returns nil if start is not in the graph.
func (g *Graph[T]) BFS(start T) []T {
	if _, ok := g.adj[start]; !ok {
		return nil
	}
	visited := map[T]bool{start: true}
	order := []T{start} // doubles as the FIFO queue: index advances as we dequeue
	for i := 0; i < len(order); i++ {
		for _, to := range g.adj[order[i]] {
			if !visited[to] {
				visited[to] = true
				order = append(order, to)
			}
		}
	}
	return order
}

// DFS traverses depth-first from start (iterative pre-order). Returns nil if
// start is not in the graph.
func (g *Graph[T]) DFS(start T) []T {
	if _, ok := g.adj[start]; !ok {
		return nil
	}
	visited := map[T]bool{}
	var order []T
	stack := []T{start}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[n] {
			continue
		}
		visited[n] = true
		order = append(order, n)
		// Push successors in reverse so the first successor is popped first.
		succ := g.adj[n]
		for i := len(succ) - 1; i >= 0; i-- {
			if !visited[succ[i]] {
				stack = append(stack, succ[i])
			}
		}
	}
	return order
}

// TopoSort returns a topological ordering of all nodes (Kahn's algorithm). Every
// edge from->to places `from` before `to` in the result. Returns ErrCycle if the
// graph contains a cycle. The ordering is deterministic (insertion-order tiebreak).
func (g *Graph[T]) TopoSort() ([]T, error) {
	indeg := make(map[T]int, len(g.order))
	for _, n := range g.order {
		indeg[n] = 0
	}
	for _, from := range g.order {
		for _, to := range g.adj[from] {
			indeg[to]++
		}
	}

	var ready []T
	for _, n := range g.order {
		if indeg[n] == 0 {
			ready = append(ready, n)
		}
	}

	result := make([]T, 0, len(g.order))
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		result = append(result, n)
		for _, to := range g.adj[n] {
			indeg[to]--
			if indeg[to] == 0 {
				ready = append(ready, to)
			}
		}
	}

	if len(result) != len(g.order) {
		return nil, ErrCycle
	}
	return result, nil
}

// HasCycle reports whether the graph contains a cycle (including self-loops).
func (g *Graph[T]) HasCycle() bool {
	_, err := g.TopoSort()
	return errors.Is(err, ErrCycle)
}
