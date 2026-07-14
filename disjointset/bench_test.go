package disjointset_test

import (
	"testing"

	"github.com/v8fg/kit4go/disjointset"
)

func BenchmarkUnionFindUnion(b *testing.B) {
	uf := disjointset.New[int]()
	b.ResetTimer()
	for b.Loop() {
		uf.Union(b.N, b.N+1)
	}
}

func BenchmarkUnionFindFind(b *testing.B) {
	uf := disjointset.New[int]()
	// Build a deep chain so path compression matters.
	for i := range 1000 {
		uf.Union(i, i+1)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = uf.Find(1000)
	}
}

func BenchmarkUnionFindConnected(b *testing.B) {
	uf := disjointset.New[int]()
	for i := range 1000 {
		uf.Union(i, i+1)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = uf.Connected(0, 1000)
	}
}

func BenchmarkUnionFindComponents(b *testing.B) {
	// 1000 nodes, 100 components of 10.
	b.ResetTimer()
	for b.Loop() {
		uf := disjointset.New[int]()
		for c := range 100 {
			for i := range 9 {
				uf.Union(c*10, c*10+i+1)
			}
		}
		_ = uf.Count()
	}
}
