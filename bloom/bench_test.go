package bloom

import (
	"fmt"
	"testing"
)

// benchFilter is sized for 100k elements at 1% FPR — a realistic ad-tech
// dedup workload. Built once and reused across Add/Test benchmarks so the
// per-op numbers reflect the hot path, not construction.
func benchFilter() *Filter {
	return New(100_000, 0.01)
}

// preFill inserts n keys so Test benchmarks probe a populated filter.
func preFill(f *Filter, n int) {
	for i := 0; i < n; i++ {
		f.AddString(fmt.Sprintf("item-%d", i))
	}
}

// BenchmarkNew measures filter construction (sizing math + backing alloc).
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = New(100_000, 0.01)
	}
}

// BenchmarkAdd measures a single insertion into a fresh filter.
func BenchmarkAdd(b *testing.B) {
	f := benchFilter()
	data := []byte("benchmark-key")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Add(data)
	}
}

// BenchmarkAddString is the string-wrapper variant of Add.
func BenchmarkAddString(b *testing.B) {
	f := benchFilter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.AddString("benchmark-key")
	}
}

// BenchmarkTestHit measures Test on a key known to be present.
func BenchmarkTestHit(b *testing.B) {
	f := benchFilter()
	preFill(f, 50_000)
	data := []byte("item-42")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.Test(data)
	}
}

// BenchmarkTestMiss measures Test on a key definitely absent (the
// common "have I seen this?" negative path).
func BenchmarkTestMiss(b *testing.B) {
	f := benchFilter()
	preFill(f, 50_000)
	data := []byte("never-added")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.Test(data)
	}
}

// BenchmarkTestAndAdd is the combined duplicate-check-plus-record path,
// the typical hot loop in dedup pipelines.
func BenchmarkTestAndAdd(b *testing.B) {
	f := benchFilter()
	data := []byte("benchmark-key")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.TestAndAdd(data)
	}
}

// BenchmarkIndices isolates the hashing + index-derivation cost, which
// dominates Add/Test and allocates the k-element index slice each call.
func BenchmarkIndices(b *testing.B) {
	f := benchFilter()
	data := []byte("benchmark-key")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.indices(data)
	}
}
