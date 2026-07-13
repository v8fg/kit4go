package iterx_test

import (
	"fmt"
	"slices"

	"github.com/v8fg/kit4go/iterx"
)

func seqFrom[T any](s []T) func(func(T) bool) { return slices.Values(s) }

// ExampleMap doubles each element of a sequence.
func ExampleMap() {
	out := iterx.Collect(iterx.Map(seqFrom([]int{1, 2, 3}), func(v int) int { return v * 2 }))
	fmt.Println(out)
	// Output:
	// [2 4 6]
}

// ExampleFilter keeps only even numbers.
func ExampleFilter() {
	out := iterx.Collect(iterx.Filter(seqFrom([]int{1, 2, 3, 4, 5, 6}), func(v int) bool {
		return v%2 == 0
	}))
	fmt.Println(out)
	// Output:
	// [2 4 6]
}

// ExampleTake takes the first N — lazily, without consuming the rest.
func ExampleTake() {
	out := iterx.Collect(iterx.Take(iterx.Range(0, 1000, 1), 5))
	fmt.Println(out)
	// Output:
	// [0 1 2 3 4]
}

// ExampleReduce sums a sequence.
func ExampleReduce() {
	total := iterx.Reduce(seqFrom([]int{1, 2, 3, 4}), 0, func(acc, v int) int {
		return acc + v
	})
	fmt.Println(total)
	// Output:
	// 10
}

// ExampleChain concatenates sequences.
func ExampleChain() {
	out := iterx.Collect(iterx.Chain(
		seqFrom([]int{1, 2}),
		seqFrom([]int{3, 4, 5}),
	))
	fmt.Println(out)
	// Output:
	// [1 2 3 4 5]
}

// ExampleZip pairs two sequences element-wise.
func ExampleZip() {
	for p := range iterx.Zip(seqFrom([]int{1, 2, 3}), seqFrom([]string{"a", "b", "c"})) {
		fmt.Println(p.First, p.Second)
	}
	// Output:
	// 1 a
	// 2 b
	// 3 c
}

// ExampleRange generates an integer sequence.
func ExampleRange() {
	fmt.Println(iterx.Collect(iterx.Range(0, 10, 2)))
	// Output:
	// [0 2 4 6 8]
}
