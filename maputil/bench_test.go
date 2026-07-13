package maputil_test

import (
	"testing"

	"github.com/v8fg/kit4go/maputil"
)

func BenchmarkMerge(b *testing.B) {
	a, c := makeMap(500), makeMap(500)
	b.ResetTimer()
	for b.Loop() {
		_ = maputil.Merge(a, c)
	}
}

func BenchmarkInvert(b *testing.B) {
	m := makeMap(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = maputil.Invert(m)
	}
}

func BenchmarkToSlice(b *testing.B) {
	m := makeMap(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = maputil.ToSlice(m)
	}
}

func BenchmarkFromSlice(b *testing.B) {
	s := make([]int, 1000)
	for i := range s {
		s[i] = i
	}
	fn := func(v int) (int, int) { return v, v * 2 }
	b.ResetTimer()
	for b.Loop() {
		_ = maputil.FromSlice(s, fn)
	}
}

func makeMap(n int) map[int]int {
	m := make(map[int]int, n)
	for i := range n {
		m[i] = i
	}
	return m
}
