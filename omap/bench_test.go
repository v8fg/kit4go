package omap_test

import (
	"testing"

	"github.com/v8fg/kit4go/omap"
)

func BenchmarkSetNew(b *testing.B) {
	m := omap.New[int, int]()
	b.ResetTimer()
	for b.Loop() {
		m.Set(b.N, b.N)
	}
}

func BenchmarkSetUpdate(b *testing.B) {
	m := omap.New[int, int]()
	m.Set(1, 1)
	b.ResetTimer()
	for b.Loop() {
		m.Set(1, b.N) // existing key
	}
}

func BenchmarkGet(b *testing.B) {
	m := omap.New[int, int]()
	for i := range 1000 {
		m.Set(i, i)
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = m.Get(500)
	}
}

func BenchmarkKeys(b *testing.B) {
	m := omap.New[int, int]()
	for i := range 1000 {
		m.Set(i, i)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = m.Keys()
	}
}

func BenchmarkEach(b *testing.B) {
	m := omap.New[int, int]()
	for i := range 1000 {
		m.Set(i, i)
	}
	b.ResetTimer()
	for b.Loop() {
		m.Each(func(int, int) bool { return true })
	}
}

func BenchmarkDelete(b *testing.B) {
	// Delete is O(n); benchmark on a 1000-entry map, reinserting each iteration.
	m := omap.New[int, int]()
	for i := range 1000 {
		m.Set(i, i)
	}
	b.ResetTimer()
	for b.Loop() {
		m.Delete(500)
		m.Set(500, 500) // restore for next iteration
	}
}
