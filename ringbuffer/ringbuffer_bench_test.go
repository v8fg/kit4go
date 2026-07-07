package ringbuffer

import (
	"testing"
)

// BenchmarkTryPushTryPop measures the non-blocking push/pop fast path. It must
// stay 0 allocs/op — both ops mutate only preallocated slots under the mutex.
func BenchmarkTryPushTryPop(b *testing.B) {
	rb := New[int](1024)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !rb.TryPush(i) {
			b.Fatal("TryPush failed")
		}
		if _, ok := rb.TryPop(); !ok {
			b.Fatal("TryPop failed")
		}
	}
}

// BenchmarkTryPushTryPopParallel hammers the buffer from multiple goroutines to
// exercise mutex contention on the contended slot.
func BenchmarkTryPushTryPopParallel(b *testing.B) {
	rb := New[int](1024)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if !rb.TryPush(1) {
				continue
			}
			rb.TryPop()
		}
	})
}

// BenchmarkLen measures the Len accessor (acquires the mutex).
func BenchmarkLen(b *testing.B) {
	rb := New[int](16)
	for i := range 16 {
		rb.TryPush(i)
	}
	b.ReportAllocs()

	for b.Loop() {
		_ = rb.Len()
	}
}
