package bitset_test

import (
	"testing"

	"github.com/v8fg/kit4go/bitset"
)

func BenchmarkSet(b *testing.B) {
	bs := bitset.New(10000)
	b.ResetTimer()
	for b.Loop() {
		bs.Set(5000)
	}
}

func BenchmarkTest(b *testing.B) {
	bs := bitset.New(10000)
	for i := range 1000 {
		bs.Set(i)
	}
	b.ResetTimer()
	for b.Loop() {
		bs.Test(500)
	}
}

func BenchmarkLen(b *testing.B) {
	bs := bitset.New(10000)
	for i := range 1000 {
		bs.Set(i)
	}
	b.ResetTimer()
	for b.Loop() {
		bs.Len()
	}
}
