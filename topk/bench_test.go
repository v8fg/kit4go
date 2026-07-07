package topk

import (
	"fmt"
	"strconv"
	"testing"
)

// benchKey returns a deterministic string key for index i.
func benchKey(i int) string { return strconv.Itoa(i) }

// BenchmarkNew measures the cost of constructing a Tracker. New preallocates an
// empty counts map and a zero-length itemHeap slice; it should be allocation-light.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = New(10)
	}
}

// BenchmarkTouchAdmit is the hot path while the set is still filling (len < K):
// every Touch inserts a new key via heap.Push — no eviction, no linear scan hit.
func BenchmarkTouchAdmit(b *testing.B) {
	const k = 10
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tr := New(k)
		b.StartTimer()
		// Cycle keys across [0, k) so each iteration admits a distinct key.
		tr.Touch(benchKey(i % k))
	}
}

// BenchmarkTouchIncrement targets the in-set increment path: a key already in the
// heap is touched again, triggering a map update + heap.Fix (sift). This is the
// steady-state cost for a leader whose key is already admitted.
func BenchmarkTouchIncrement(b *testing.B) {
	const k = 10
	tr := New(k)
	// Fill the set so "hot" is already admitted.
	for i := range k {
		tr.TouchN(benchKey(i), int64(k-i)) // distinct counts
	}
	b.ReportAllocs()

	for b.Loop() {
		tr.Touch("hot")
	}
}

// BenchmarkTouchEvict exercises the full-set eviction branch: a new competitive
// key beats the current min, displacing it (heap.Pop + delete + heap.Push). This
// is the worst-case per-touch cost on a high-cardinality stream.
func BenchmarkTouchEvict(b *testing.B) {
	const k = 10
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tr := New(k)
		// Pre-fill with low counts so each subsequent touch evicts the min.
		for j := range k {
			tr.TouchN(benchKey(j), 1)
		}
		b.StartTimer()
		// New key with count 2 > min count 1 → eviction.
		tr.TouchN(fmt.Sprintf("new-%d", i), 2)
	}
}

// BenchmarkTouchHighCardinality simulates the documented use case (auction/user
// IDs over a stream): mostly-distinct keys flowing through a small-K tracker. It
// covers the full mix of admit / increment / evict / drop paths per key and is
// the closest single-number proxy for real streaming throughput.
func BenchmarkTouchHighCardinality(b *testing.B) {
	const k = 10
	b.ReportAllocs()
	tr := New(k)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 1k-cardinality key space vs K=10 → frequent misses and evictions,
		// with a few hot keys ("0".."9") re-entering repeatedly.
		tr.Touch(benchKey(i % 1000))
	}
}

// BenchmarkTouchN is the bulk-increment entry point (Touch delegates to it).
// Measures TouchN with n=1 to isolate the dispatch overhead vs Touch's wrapper.
func BenchmarkTouchN(b *testing.B) {
	const k = 10
	tr := New(k)
	for i := range k {
		tr.TouchN(benchKey(i), 1)
	}
	b.ReportAllocs()

	for b.Loop() {
		tr.TouchN("hot", 1)
	}
}

// BenchmarkTop measures the read path: snapshot the heap, copy into Entries, and
// sort descending. Allocates a slice of len(entries) plus the sort closure.
func BenchmarkTop(b *testing.B) {
	const k = 10
	tr := New(k)
	for i := range k {
		tr.TouchN(benchKey(i), int64(k-i))
	}
	b.ReportAllocs()

	for b.Loop() {
		_ = tr.Top()
	}
}

// BenchmarkCount measures the per-key count lookup (map read under lock).
func BenchmarkCount(b *testing.B) {
	const k = 10
	tr := New(k)
	tr.TouchN("hot", 42)
	b.ReportAllocs()

	for b.Loop() {
		_ = tr.Count("hot")
	}
}

// BenchmarkTopK100 scales K to 100 to show how heap depth (log K) and the Top()
// sort cost (K log K) grow relative to the small-K case.
func BenchmarkTopK100(b *testing.B) {
	const k = 100
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		tr := New(k)
		for j := range k {
			tr.TouchN(benchKey(j), int64(j))
		}
		b.StartTimer()
		_ = tr.Top()
	}
}

// BenchmarkTouchHighCardinalityK100 repeats the streaming scenario at K=100,
// where more keys are admitted and the heap stays deeper.
func BenchmarkTouchHighCardinalityK100(b *testing.B) {
	const k = 100
	b.ReportAllocs()
	tr := New(k)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Touch(benchKey(i % 1000))
	}
}
