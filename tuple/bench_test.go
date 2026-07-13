package tuple_test

import (
	"testing"

	"github.com/v8fg/kit4go/tuple"
)

func BenchmarkNewPair(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		tuple.NewPair(1, "hello")
	}
}

func BenchmarkPairValues(b *testing.B) {
	p := tuple.NewPair(1, "hello")
	b.ResetTimer()
	for b.Loop() {
		p.Values()
	}
}
