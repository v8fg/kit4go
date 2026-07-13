package stack_test

import (
	"testing"

	"github.com/v8fg/kit4go/stack"
)

func BenchmarkStackPushPrealloc(b *testing.B) {
	s := stack.WithCapacity[int](b.N)
	b.ResetTimer()
	for b.Loop() {
		s.Push(1)
	}
}

func BenchmarkStackPeek(b *testing.B) {
	s := stack.New[int]()
	for range 1000 {
		s.Push(1)
	}
	b.ResetTimer()
	for b.Loop() {
		s.Peek()
	}
}
