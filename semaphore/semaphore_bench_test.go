package semaphore

import (
	"context"
	"testing"
)

// BenchmarkAcquireRelease measures the hot-path cost of acquiring and releasing
// a single permit. The channel-based implementation must show 0 allocs/op here
// (vs the previous cond+goroutine-per-acquire design which allocated a channel
// and spawned a goroutine on every Acquire).
func BenchmarkAcquireRelease(b *testing.B) {
	s := New(1024)
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		if err := s.Acquire(ctx, 1); err != nil {
			b.Fatal(err)
		}
		s.Release(1)
	}
}

// BenchmarkAcquireReleaseParallel hammers the semaphore from multiple goroutines
// to exercise contention on the underlying channel.
func BenchmarkAcquireReleaseParallel(b *testing.B) {
	s := New(1024)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := s.Acquire(ctx, 1); err != nil {
				b.Fatal(err)
			}
			s.Release(1)
		}
	})
}

// BenchmarkTryAcquire measures the non-blocking fast path.
func BenchmarkTryAcquire(b *testing.B) {
	s := New(1024)
	b.ReportAllocs()

	for b.Loop() {
		if !s.TryAcquire(1) {
			b.Fatal("TryAcquire failed")
		}
		s.Release(1)
	}
}
