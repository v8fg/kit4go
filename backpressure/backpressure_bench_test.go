package backpressure

import "testing"

// These benchmarks extend bench_test.go (TryAcquire_Release,
// TryAcquire_Contended) with the rejection and accessor paths.

// BenchmarkTryAcquireRejected measures the rejection path (gate full): the
// rejected counter is incremented atomically. Must stay 0 allocs/op.
func BenchmarkTryAcquireRejected(b *testing.B) {
	g := New(0) // always full
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.TryAcquire()
	}
}

// BenchmarkCurrent measures the in-flight accessor (single atomic load).
func BenchmarkCurrent(b *testing.B) {
	g := New(1 << 20)
	g.TryAcquire()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.Current()
	}
}
