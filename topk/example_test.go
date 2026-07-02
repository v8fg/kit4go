package topk_test

import (
	"fmt"

	"github.com/v8fg/kit4go/topk"
)

func ExampleTracker() {
	t := topk.New(3) // track top 3 keys
	t.TouchN("a", 5)
	t.TouchN("b", 2)
	t.Touch("c")
	fmt.Println(t.Count("a"))
	// Output: 5
}
