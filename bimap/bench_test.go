package bimap_test

import (
	"testing"

	"github.com/v8fg/kit4go/bimap"
)

func BenchmarkBiMapInsert(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		bm := bimap.New[int, string]()
		for i := range 100 {
			bm.MustInsert(i, "val")
			_ = bm
		}
	}
}

func BenchmarkBiMapGet(b *testing.B) {
	bm := bimap.New[int, string]()
	for i := range 1000 {
		bm.MustInsert(i, "v")
	}
	b.ResetTimer()
	for b.Loop() {
		bm.Get(500)
	}
}

func BenchmarkBiMapGetKey(b *testing.B) {
	bm := bimap.New[int, string]()
	for i := range 1000 {
		bm.MustInsert(i, "v")
	}
	b.ResetTimer()
	for b.Loop() {
		bm.GetKey("v")
	}
}
