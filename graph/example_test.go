package graph_test

import (
	"fmt"

	"github.com/v8fg/kit4go/graph"
)

// ExampleGraph_TopoSort resolves a build-order: every edge "from -> to" means
// `from` must be built before `to`.
func ExampleGraph_TopoSort() {
	g := graph.New[string]()
	g.AddEdge("base", "auth")
	g.AddEdge("base", "db")
	g.AddEdge("auth", "api")
	g.AddEdge("db", "api")
	g.AddEdge("api", "web")

	order, err := g.TopoSort()
	fmt.Println(order, err)
	// Output:
	// [base auth db api web] <nil>
}

// ExampleGraph_HasCycle detects an unsatisfiable dependency cycle.
func ExampleGraph_HasCycle() {
	g := graph.New[string]()
	g.AddEdge("a", "b")
	g.AddEdge("b", "c")
	g.AddEdge("c", "a") // cycle

	fmt.Println(g.HasCycle())
	// Output:
	// true
}

// ExampleGraph_BFS traverses a graph breadth-first from a start node.
func ExampleGraph_BFS() {
	g := graph.New[int]()
	g.AddEdge(1, 2)
	g.AddEdge(1, 3)
	g.AddEdge(2, 4)
	g.AddEdge(3, 4)

	fmt.Println(g.BFS(1))
	// Output:
	// [1 2 3 4]
}
