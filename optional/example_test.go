package optional_test

import (
	"fmt"

	"github.com/v8fg/kit4go/optional"
)

// ExampleOption demonstrates Some/None/UnwrapOr.
func ExampleOption() {
	name := optional.Some("Alice")
	age := optional.None[int]()

	fmt.Println(name.Unwrap())
	fmt.Println(age.UnwrapOr(0))

	// Output:
	// Alice
	// 0
}

// ExampleMap demonstrates transforming an optional value.
func ExampleMap() {
	len := optional.Map(optional.Some("hello"), func(s string) int {
		return len(s)
	})
	fmt.Println(len.Unwrap())

	// Output:
	// 5
}
