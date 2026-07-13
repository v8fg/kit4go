package multimap_test

import (
	"testing"

	"github.com/v8fg/kit4go/multimap"
)

func BenchmarkAdd(b *testing.B) {
	mm := multimap.New[int, int]()
	b.ResetTimer()
	for b.Loop() {
		mm.Add(1, 1)
	}
}

func BenchmarkAddDistinctKeys(b *testing.B) {
	mm := multimap.New[int, int]()
	b.ResetTimer()
	for b.Loop() {
		for i := range b.N {
			mm.Add(i, i)
		}
	}
}

func BenchmarkGet(b *testing.B) {
	mm := multimap.New[int, int]()
	for i := range 100 {
		mm.Add(0, i)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = mm.Get(0)
	}
}

func BenchmarkKeys(b *testing.B) {
	mm := multimap.New[int, int]()
	for i := range 1000 {
		mm.Add(i, i)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = mm.Keys()
	}
}

func BenchmarkEach(b *testing.B) {
	mm := multimap.New[int, int]()
	for i := range 1000 {
		mm.Add(i%100, i)
	}
	b.ResetTimer()
	for b.Loop() {
		mm.Each(func(int, int) bool { return true })
	}
}
