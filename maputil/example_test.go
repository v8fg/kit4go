package maputil_test

import (
	"fmt"

	"github.com/v8fg/kit4go/maputil"
)

// ExampleMerge combines several maps with last-wins precedence.
func ExampleMerge() {
	defaults := map[string]int{"a": 1, "b": 2}
	override := map[string]int{"b": 20, "c": 30}
	merged := maputil.Merge(defaults, override)
	fmt.Println(merged["a"], merged["b"], merged["c"])
	// Output:
	// 1 20 30
}

// ExampleInvert swaps keys and values (e.g. reverse a lookup table).
func ExampleInvert() {
	wordToNum := map[string]int{"one": 1, "two": 2}
	numToWord := maputil.Invert(wordToNum)
	fmt.Println(numToWord[1], numToWord[2])
	// Output:
	// one two
}

// ExampleFromSlice builds a map from a slice via a key+value function.
func ExampleFromSlice() {
	type row struct {
		ID  int
		Val string
	}
	rows := []row{{1, "a"}, {2, "b"}}
	m := maputil.FromSlice(rows, func(r row) (int, string) { return r.ID, r.Val })
	fmt.Println(m[1], m[2])
	// Output:
	// a b
}

// ExampleToSlice materializes a map into key-value pairs.
func ExampleToSlice() {
	m := map[int]string{1: "a", 2: "b"}
	for _, kv := range maputil.ToSlice(m) {
		fmt.Println(kv.Key, kv.Value)
	}
}
