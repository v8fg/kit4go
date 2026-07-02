package str_test

import (
	"fmt"

	"github.com/v8fg/kit4go/str"
)

func ExampleEqualIgnoreCase() {
	fmt.Println(str.EqualIgnoreCase("Hello", "HELLO"))
	fmt.Println(str.ContainsAll("hello world", "hello", "world"))
	// Output:
	// true
	// true
}
