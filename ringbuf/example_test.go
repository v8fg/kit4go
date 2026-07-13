package ringbuf_test

import (
	"fmt"

	"github.com/v8fg/kit4go/ringbuf"
)

func ExampleNew() {
	c := ringbuf.New[int](3)
	c.Push(1)
	c.Push(2)
	c.Push(3)
	c.Push(4) // overwrites 1 (oldest)
	fmt.Println(c.ToSlice())
	// Output: [2 3 4]
}
