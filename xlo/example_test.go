package xlo_test

import (
	"fmt"

	"github.com/v8fg/kit4go/xlo"
)

// ExampleUniq demonstrates deduplicating a slice while preserving the order
// of first occurrence. This is the package's headline helper.
func ExampleUniq() {
	nums := xlo.Uniq([]int{1, 2, 2, 3, 1, 3, 4})
	fmt.Println(nums)

	words := xlo.Uniq([]string{"go", "go", "rust", "go", "rust", "zig"})
	fmt.Println(words)

	// Output:
	// [1 2 3 4]
	// [go rust zig]
}

// ExampleLoMap demonstrates transforming a slice into a slice of another type.
// The iteratee receives both the element and its index.
func ExampleLoMap() {
	doubled := xlo.LoMap([]int{1, 2, 3}, func(v, _ int) int { return v * 2 })
	fmt.Println(doubled)

	// LopMap runs the iteratee concurrently but returns results in the
	// original order; no // Output: since it is a parallel operation.
	_ = xlo.LopMap([]int{1, 2, 3}, func(v, _ int) int { return v * 2 })

	// Output:
	// [2 4 6]
}
