package stack_test

import (
	"testing"

	"github.com/v8fg/kit4go/stack"
)

// FuzzLIFOOrder encodes the LIFO invariant: pushing values then popping all
// yields them in reverse insertion order.
func FuzzLIFOOrder(f *testing.F) {
	f.Add(1, 2, 3)
	f.Add(0, 0, 0)
	f.Add(-1, 100, 50)
	f.Fuzz(func(t *testing.T, a, b, c int) {
		s := stack.New(a, b, c) // a=bottom, c=top
		r1, ok1 := s.Pop()
		r2, ok2 := s.Pop()
		r3, ok3 := s.Pop()
		if !ok1 || !ok2 || !ok3 {
			t.Fatalf("Pop returned ok=false on non-empty stack")
		}
		if r1 != c || r2 != b || r3 != a {
			t.Errorf("LIFO violated: pushed [%d,%d,%d], popped [%d,%d,%d]", a, b, c, r3, r2, r1)
		}
	})
}
