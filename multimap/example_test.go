package multimap_test

import (
	"fmt"

	"github.com/v8fg/kit4go/multimap"
)

// ExampleNew models multi-valued query parameters (?tag=go&tag=lib&env=prod).
func ExampleNew() {
	params := multimap.New[string, string]()
	params.Add("tag", "go")
	params.Add("tag", "lib")
	params.Add("env", "prod")

	fmt.Println("tags:", params.Get("tag"))
	fmt.Println("env:", params.Get("env"))
	fmt.Println("keys:", params.Len())
	// Output:
	// tags: [go lib]
	// env: [prod]
	// keys: 2
}

// ExampleDeleteValue removes a single value from a key's bucket.
func ExampleDeleteValue() {
	mm := multimap.New[string, string]()
	mm.Add("tag", "a")
	mm.Add("tag", "b")
	mm.Add("tag", "c")

	multimap.DeleteValue(mm, "tag", "b")
	fmt.Println(mm.Get("tag"))
	// Output:
	// [a c]
}
