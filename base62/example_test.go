package base62_test

import (
	"fmt"

	"github.com/v8fg/kit4go/base62"
)

func ExampleEncode() {
	// Encode/Decode round-trip a uint64 id (e.g. a short link id).
	n, _ := base62.Decode(base62.Encode(12345))
	fmt.Println(n)
	// Output: 12345
}
