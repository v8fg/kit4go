package queue_test

import (
	"fmt"

	"github.com/v8fg/kit4go/queue"
)

func ExampleQueue() {
	q := queue.New[string]()
	q.Enqueue("first")
	q.Enqueue("second")

	v, _ := q.Dequeue()
	fmt.Println(v)
	v, _ = q.Dequeue()
	fmt.Println(v)

	// Output:
	// first
	// second
}
