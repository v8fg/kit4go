package memoize_test

import (
	"testing"

	"github.com/v8fg/kit4go/memoize"
)

func BenchmarkMemoizeCached(b *testing.B) {
	m := memoize.Memoize(func(n int) int { return n * n })
	m(42) // prime the cache
	b.ResetTimer()
	for b.Loop() {
		_ = m(42)
	}
}

func BenchmarkMemoizeFirstCall(b *testing.B) {
	// Each iteration uses a fresh key → cache miss + Store.
	m := memoize.Memoize(func(n int) int { return n * n })
	b.ResetTimer()
	for b.Loop() {
		_ = m(b.N) // unique key each iteration
	}
}

func BenchmarkMemoizeErrCached(b *testing.B) {
	m := memoize.MemoizeErr(func(k int) (int, error) { return k + 1, nil })
	_, _ = m(42)
	b.ResetTimer()
	for b.Loop() {
		_, _ = m(42)
	}
}
