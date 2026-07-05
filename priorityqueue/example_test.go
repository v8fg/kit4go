package priorityqueue_test

import (
	"fmt"

	"github.com/v8fg/kit4go/priorityqueue"
)

func ExampleNew() {
	q := priorityqueue.New[string]()
	q.Push("low", 1)
	q.Push("high", 10)
	q.Push("mid", 5)

	for q.Len() > 0 {
		v, _, _ := q.Pop()
		fmt.Println(v)
	}
	// Output:
	// high
	// mid
	// low
}

func ExampleQueue_peekAndLen() {
	q := priorityqueue.New[int]()
	q.Push(100, 1)
	q.Push(42, 9)

	v, prio, ok := q.Peek()
	fmt.Println(v, prio, ok)
	fmt.Println(q.Len())
	// Output:
	// 42 9 true
	// 2
}

func ExampleQueue_update() {
	q := priorityqueue.New[string]()
	low := q.Push("later", 1)
	q.Push("now", 7)

	// Promote "later" above "now" without re-enqueuing.
	q.Update(low, 99)

	v, prio, _ := q.Pop()
	fmt.Println(v, prio)
	// Output:
	// later 99
}
