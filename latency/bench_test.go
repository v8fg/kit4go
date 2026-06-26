package latency_test

import (
	"testing"
	"time"

	"github.com/v8fg/kit4go/latency"
)

// BenchmarkHistogram_Observe measures the single-goroutine observe hot path:
// binary bucket lookup + lock + advance + increment. Same shape as limiter's
// sliding window, allocation-free.
func BenchmarkHistogram_Observe(b *testing.B) {
	h := latency.NewHistogram(latency.Options{})
	d := 5 * time.Millisecond
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Observe(d)
	}
}

// BenchmarkHistogram_Observe_Parallel measures write contention. Use -cpu to
// scale; this is where a single mutex saturates (~3-4M ops/s) and where a
// sharded histogram helps.
func BenchmarkHistogram_Observe_Parallel(b *testing.B) {
	h := latency.NewHistogram(latency.Options{})
	d := 5 * time.Millisecond
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			h.Observe(d)
		}
	})
}

// BenchmarkHistogram_Quantile measures a read: fold the window (60 buckets ×
// 17 counts) into mergeBuf and scan for the percentile. 0 alloc via mergeBuf
// reuse.
func BenchmarkHistogram_Quantile(b *testing.B) {
	h := latency.NewHistogram(latency.Options{})
	for i := 0; i < 10000; i++ {
		h.Observe(time.Duration(i%50) * time.Millisecond)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.Quantile(0.99)
	}
}

// BenchmarkHistogram_Snapshot measures one fold + four percentile scans.
func BenchmarkHistogram_Snapshot(b *testing.B) {
	h := latency.NewHistogram(latency.Options{})
	for i := 0; i < 10000; i++ {
		h.Observe(time.Duration(i%50) * time.Millisecond)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.Snapshot()
	}
}

// BenchmarkShardHistogram_Observe_Parallel shows the payoff of sharding: the
// atomic round-robin picks a shard, so contention is divided by N. Compare
// against BenchmarkHistogram_Observe_Parallel.
func BenchmarkShardHistogram_Observe_Parallel(b *testing.B) {
	h := latency.NewShardHistogram(32, latency.Options{})
	d := 5 * time.Millisecond
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			h.Observe(d)
		}
	})
}

// BenchmarkNewHistogram_Factory measures construction cost (ring + per-bucket
// counts + boundaries copy + mergeBuf). Useful for callers that build a fresh
// histogram per scope.
func BenchmarkNewHistogram_Factory(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = latency.NewHistogram(latency.Options{})
	}
}
