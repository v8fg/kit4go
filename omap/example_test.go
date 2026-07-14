package omap_test

import (
	"fmt"

	"github.com/v8fg/kit4go/omap"
)

// ExampleNew builds a map whose Keys/Each are deterministic (insertion order),
// unlike a built-in map.
func ExampleNew() {
	m := omap.New[string, int]()
	m.Set("first", 1)
	m.Set("second", 2)
	m.Set("third", 3)

	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		fmt.Println(k, v)
	}
	// Output:
	// first 1
	// second 2
	// third 3
}

// ExampleMap_Delete preserves the order of remaining keys.
func ExampleMap_Delete() {
	m := omap.New[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("c", 3)

	m.Delete("b")
	fmt.Println(m.Keys())
	// Output:
	// [a c]
}
