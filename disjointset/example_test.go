package disjointset_test

import (
	"fmt"

	"github.com/v8fg/kit4go/disjointset"
)

// ExampleUnionFind counts connected components of an undirected graph given its
// edge list.
func ExampleUnionFind() {
	edges := [][2]int{
		{0, 1}, {1, 2}, // component {0,1,2}
		{3, 4}, // component {3,4}
	}
	uf := disjointset.New[int]()
	for _, e := range edges {
		uf.Union(e[0], e[1])
	}
	uf.Add(5) // isolated node

	fmt.Println("components:", uf.Count())
	fmt.Println("0-2 connected:", uf.Connected(0, 2))
	fmt.Println("0-3 connected:", uf.Connected(0, 3))
	// Output:
	// components: 3
	// 0-2 connected: true
	// 0-3 connected: false
}
