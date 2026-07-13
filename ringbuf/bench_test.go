package ringbuf_test

import (
	"testing"

	"github.com/v8fg/kit4go/ringbuf"
)

func BenchmarkPush(b *testing.B) {
	c := ringbuf.New[int](1024)
	b.ResetTimer()
	for b.Loop() {
		c.Push(1)
	}
}

func BenchmarkAt(b *testing.B) {
	c := ringbuf.New[int](1024)
	for i := range 1024 {
		c.Push(i)
	}
	b.ResetTimer()
	for b.Loop() {
		c.At(512)
	}
}
