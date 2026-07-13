package queue_test

import (
	"testing"

	"github.com/v8fg/kit4go/queue"
)

// FuzzFIFOOrder encodes the FIFO invariant: enqueuing values then dequeuing all
// yields them in insertion order.
func FuzzFIFOOrder(f *testing.F) {
	f.Add(1, 2, 3)
	f.Add(0, 0, 0)
	f.Add(-1, 100, 50)
	f.Fuzz(func(t *testing.T, a, b, c int) {
		q := queue.New(a, b, c)
		r1, ok1 := q.Dequeue()
		r2, ok2 := q.Dequeue()
		r3, ok3 := q.Dequeue()
		if !ok1 || !ok2 || !ok3 {
			t.Fatalf("Dequeue returned ok=false on non-empty queue")
		}
		if r1 != a || r2 != b || r3 != c {
			t.Errorf("FIFO violated: enqueued [%d,%d,%d], dequeued [%d,%d,%d]", a, b, c, r1, r2, r3)
		}
	})
}
