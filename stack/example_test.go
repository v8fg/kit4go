package stack_test

import (
	"fmt"

	"github.com/v8fg/kit4go/stack"
)

// ExampleStack demonstrates push / pop (LIFO order).
func ExampleStack() {
	s := stack.New[string]()
	s.Push("hello")
	s.Push("world")

	v, _ := s.Pop()
	fmt.Println(v)
	v, _ = s.Pop()
	fmt.Println(v)

	// Output:
	// world
	// hello
}
