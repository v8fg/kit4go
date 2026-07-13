package iterx_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/v8fg/kit4go/iterx"
)

func srcSeq(n int) []int {
	s := make([]int, n)
	for i := range n {
		s[i] = i
	}
	return s
}

func BenchmarkMapCollect(b *testing.B) {
	src := srcSeq(1000)
	b.ResetTimer()
	for b.Loop() {
		iterx.Collect(iterx.Map(slices.Values(src), func(v int) int { return v * 2 }))
	}
}

func BenchmarkFilterCollect(b *testing.B) {
	src := srcSeq(1000)
	b.ResetTimer()
	for b.Loop() {
		iterx.Collect(iterx.Filter(slices.Values(src), func(v int) bool { return v%2 == 0 }))
	}
}

func BenchmarkTakeCollect(b *testing.B) {
	src := srcSeq(1000)
	b.ResetTimer()
	for b.Loop() {
		iterx.Collect(iterx.Take(slices.Values(src), 100))
	}
}

func BenchmarkReduce(b *testing.B) {
	src := srcSeq(1000)
	b.ResetTimer()
	for b.Loop() {
		iterx.Reduce(slices.Values(src), 0, func(a, v int) int { return a + v })
	}
}

func BenchmarkRangeCollect(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		iterx.Collect(iterx.Range(0, 1000, 1))
	}
}

func BenchmarkChainCollect(b *testing.B) {
	s := srcSeq(250)
	seqs := []iter.Seq[int]{
		slices.Values(s),
		slices.Values(s),
		slices.Values(s),
		slices.Values(s),
	}
	b.ResetTimer()
	for b.Loop() {
		iterx.Collect(iterx.Chain(seqs...))
	}
}
