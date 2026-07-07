package fanout

import (
	"testing"
)

// BenchmarkPublish measures the broadcast hot path with one subscribed consumer
// that drains in a separate goroutine, so the channel is rarely full and the
// non-blocking send completes. Alloc-free on the publish path.
func BenchmarkPublish(b *testing.B) {
	f := New[int](WithBufferSize[int](256))
	sub := f.Subscribe()
	done := make(chan struct{})
	go func() {
		for range sub.Ch {
		}
		close(done)
	}()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Publish(i)
	}
	b.StopTimer()
	f.Close()
	<-done
}

// BenchmarkPublishBlocking measures the blocking broadcast (waits for delivery).
func BenchmarkPublishBlocking(b *testing.B) {
	f := New[int](WithBufferSize[int](256))
	sub := f.Subscribe()
	done := make(chan struct{})
	go func() {
		for range sub.Ch {
		}
		close(done)
	}()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.PublishBlocking(b.Context(), i)
	}
	b.StopTimer()
	f.Close()
	<-done
}

// BenchmarkSubscribers measures the subscriber-count accessor (read lock).
func BenchmarkSubscribers(b *testing.B) {
	f := New[int]()
	for range 8 {
		f.Subscribe()
	}
	b.ReportAllocs()

	for b.Loop() {
		_ = f.Subscribers()
	}
}
