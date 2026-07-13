package queue_test

import (
	"testing"

	"github.com/v8fg/kit4go/queue"
)

func BenchmarkQueueFront(b *testing.B) {
	q := queue.New[int]()
	for range 1000 {
		q.Enqueue(1)
	}
	b.ResetTimer()
	for b.Loop() {
		q.Front()
	}
}
