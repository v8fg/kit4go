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
	for i := range n {
		f.AddString(fmt.Sprintf("item-%d", i))
	}
}

// BenchmarkNew measures filter construction (sizing math + backing alloc).
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = New(100_000, 0.01)
	}
}

// BenchmarkAdd measures a single insertion into a fresh filter.
func BenchmarkAdd(b *testing.B) {
	f := benchFilter()
	data := []byte("benchmark-key")
	b.ReportAllocs()

	for b.Loop() {
		f.Add(data)
	}
}

// BenchmarkAddString is the string-wrapper variant of Add.
func BenchmarkAddString(b *testing.B) {
	f := benchFilter()
	b.ReportAllocs()

	for b.Loop() {
		f.AddString("benchmark-key")
	}
}

// BenchmarkTestHit measures Test on a key known to be present.
func BenchmarkTestHit(b *testing.B) {
	f := benchFilter()
	preFill(f, 50_000)
	data := []byte("item-42")
	b.ReportAllocs()

	for b.Loop() {
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

	for b.Loop() {
		_ = f.Test(data)
	}
}

// BenchmarkTestAndAdd is the combined duplicate-check-plus-record path,
// the typical hot loop in dedup pipelines.
func BenchmarkTestAndAdd(b *testing.B) {
	f := benchFilter()
	data := []byte("benchmark-key")
	b.ReportAllocs()

	for b.Loop() {
		_ = f.TestAndAdd(data)
	}
}

// BenchmarkIndices isolates the hashing + index-derivation cost, which
// dominates Add/Test. The slice is borrowed from the pool and returned each
// iteration, so this measures the true hot-path cost (0 allocs after warmup).
func BenchmarkIndices(b *testing.B) {
	f := benchFilter()
	data := []byte("benchmark-key")
	b.ReportAllocs()

	for b.Loop() {
		idxp := f.indices(data)
		f.ipool.Put(idxp)
	}
}

// BenchmarkAddTestMixed simulates the typical dedup loop: Test-then-Add on a
// steady mix of hit/miss keys. The whole loop must be 0 allocs/op thanks to
// the pooled index slice.
func BenchmarkAddTestMixed(b *testing.B) {
	f := benchFilter()
	preFill(f, 50_000)
	b.ReportAllocs()

	for b.Loop() {
		key := []byte("item-42")
		_ = f.Test(key)
		f.Add(key)
	}
}
