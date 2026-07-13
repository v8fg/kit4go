package tuple_test

import (
	"fmt"

	"github.com/v8fg/kit4go/tuple"
)

func ExampleNewPair() {
	p := tuple.NewPair("US", 331)
	fmt.Printf("%s pop %dM\n", p.First, p.Second)
	// Output: US pop 331M
}

func ExampleNewTriple() {
	t := tuple.NewTriple("Alice", 30, true)
	fmt.Println(t.First, t.Second, t.Third)
	// Output: Alice 30 true
}
