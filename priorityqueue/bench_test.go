package priorityqueue_test

import (
	"testing"

	"github.com/v8fg/kit4go/priorityqueue"
)

// BenchmarkPushPopBatch pushes N items then drains them — the canonical
// priority-queue workload (heapify + extract-min/max).
func BenchmarkPushPopBatch(b *testing.B) {
	const n = 1000
	b.ReportAllocs()
	for b.Loop() {
		q := priorityqueue.New[int]()
		for i := range n {
			q.Push(i, i)
		}
		for range n {
			_, _, _ = q.Pop()
		}
	}
}

// BenchmarkPushOnly isolates insertion (heap up-sift) throughput.
func BenchmarkPushOnly(b *testing.B) {
	const n = 1000
	b.ReportAllocs()
	for b.Loop() {
		q := priorityqueue.New[int]()
		for i := range n {
			q.Push(i, i)
		}
	}
}

// BenchmarkPopAmortized isolates extraction (down-sift): a large queue is kept
// topped up, draining one item per iteration so the per-Pop cost dominates.
func BenchmarkPopAmortized(b *testing.B) {
	const n = 1000
	q := priorityqueue.New[int]()
	for i := range n {
		q.Push(i, i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		if q.Len() == 0 {
			// refill (not measured meaningfully — dominated by Pop below)
			for range n {
				q.Push(i, i)
				i++
			}
		}
		_, _, _ = q.Pop()
	}
}
