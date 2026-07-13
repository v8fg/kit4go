package sliceutil_test

import (
	"testing"

	"github.com/v8fg/kit4go/sliceutil"
)

func BenchmarkChunk(b *testing.B) {
	src := makeSlice(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = sliceutil.Chunk(src, 32)
	}
}

func BenchmarkFlatten(b *testing.B) {
	src := make([][]int, 100)
	for i := range src {
		src[i] = makeSlice(10)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = sliceutil.Flatten(src)
	}
}

func BenchmarkDeduplicate(b *testing.B) {
	src := makeSliceWithDups(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = sliceutil.Deduplicate(src)
	}
}

func BenchmarkPartition(b *testing.B) {
	src := makeSlice(1000)
	pred := func(v int) bool { return v%2 == 0 }
	b.ResetTimer()
	for b.Loop() {
		_, _ = sliceutil.Partition(src, pred)
	}
}

func BenchmarkGroupBy(b *testing.B) {
	src := makeSlice(1000)
	keyFn := func(v int) int { return v % 10 }
	b.ResetTimer()
	for b.Loop() {
		_ = sliceutil.GroupBy(src, keyFn)
	}
}

func BenchmarkReverse(b *testing.B) {
	src := makeSlice(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = sliceutil.Reverse(src)
	}
}

func makeSlice(n int) []int {
	s := make([]int, n)
	for i := range s {
		s[i] = i
	}
	return s
}

func makeSliceWithDups(n int) []int {
	s := make([]int, n)
	for i := range s {
		s[i] = i % 100
	}
	return s
}
