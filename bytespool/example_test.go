package bytespool_test

import (
	"fmt"

	"github.com/v8fg/kit4go/bytespool"
)

func ExampleGet() {
	b := bytespool.Get(256)
	defer bytespool.Put(b)
	b.WriteString("hello")
	b.WriteByte('!')
	fmt.Println(b.String())
	// Output: hello!
}
