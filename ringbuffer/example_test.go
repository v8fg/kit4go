package ringbuffer_test

import (
	"fmt"

	"github.com/v8fg/kit4go/ringbuffer"
)

func ExampleRingBuffer() {
	rb := ringbuffer.New[int](2)
	rb.TryPush(1)
	rb.TryPush(2)
	fmt.Println(rb.TryPush(3)) // buffer full → 3 is dropped (false)
	v, _ := rb.TryPop()
	fmt.Println(v) // FIFO order: 1 was pushed first
	// Output:
	// false
	// 1
}
