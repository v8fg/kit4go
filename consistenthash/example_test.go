package consistenthash_test

import (
	"fmt"

	"github.com/v8fg/kit4go/consistenthash"
)

func ExampleNew() {
	m := consistenthash.New(func(s string) string { return s },
		consistenthash.WithNodes("node-a", "node-b", "node-c"),
	)

	// The same key always maps to the same node (rendezvous hashing).
	n1, _ := m.Get("auction/123")
	n2, _ := m.Get("auction/123")
	fmt.Println(n1 == n2)
	fmt.Println(m.Len())
	// Output:
	// true
	// 3
}
