package batcher

import (
	"testing"
)

// BenchmarkAdd measures the Add hot path (a buffered channel send under a read
// lock). The collector drains in the background, so Add is non-blocking here.
func BenchmarkAdd(b *testing.B) {
	bt := New[int](64, 0, func([]int) {}, WithBufferSize[int](1024))
	defer bt.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !bt.Add(i) {
			b.Fatal("Add returned false")
		}
	}
}

// BenchmarkAddParallel measures Add under multiple producers.
func BenchmarkAddParallel(b *testing.B) {
	bt := New[int](64, 0, func([]int) {}, WithBufferSize[int](1024))
	defer bt.Close()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			bt.Add(i)
			i++
		}
	})
}

// BenchmarkFlush measures a manual flush of an empty buffer (the cheap path).
func BenchmarkFlush(b *testing.B) {
	bt := New[int](64, 0, func([]int) {}, WithBufferSize[int](1024))
	defer bt.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bt.Flush()
	}
}
