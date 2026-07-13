package sliceutil_test

import (
	"fmt"

	"github.com/v8fg/kit4go/sliceutil"
)

// ExampleChunk splits a slice into fixed-size batches.
func ExampleChunk() {
	pages := sliceutil.Chunk([]int{1, 2, 3, 4, 5, 6, 7}, 3)
	fmt.Println(pages)
	// Output:
	// [[1 2 3] [4 5 6] [7]]
}

// ExampleDeduplicate removes duplicates while keeping first-occurrence order.
func ExampleDeduplicate() {
	fmt.Println(sliceutil.Deduplicate([]int{3, 1, 3, 2, 1, 4}))
	// Output:
	// [3 1 2 4]
}

// ExampleGroupBy buckets elements by a computed key.
func ExampleGroupBy() {
	groups := sliceutil.GroupBy([]string{"go", "rust", "gem", "red"}, func(s string) byte {
		return s[0]
	})
	fmt.Println(len(groups['g']), len(groups['r']))
	// Output:
	// 2 2
}

// ExamplePartition splits a slice into passing and failing halves.
func ExamplePartition() {
	even, odd := sliceutil.Partition([]int{1, 2, 3, 4, 5, 6}, func(v int) bool {
		return v%2 == 0
	})
	fmt.Println(even, odd)
	// Output:
	// [2 4 6] [1 3 5]
}

// ExampleAssociate builds a lookup map from a slice.
func ExampleAssociate() {
	type item struct {
		ID   int
		Name string
	}
	items := []item{{1, "a"}, {2, "b"}}
	m := sliceutil.Associate(items, func(it item) (int, string) {
		return it.ID, it.Name
	})
	fmt.Println(m[1], m[2])
	// Output:
	// a b
}

// ExampleWindow produces all rolling sub-slices of length n.
func ExampleWindow() {
	for _, w := range sliceutil.Window([]int{1, 2, 3, 4}, 2) {
		fmt.Println(w)
	}
	// Output:
	// [1 2]
	// [2 3]
	// [3 4]
}
