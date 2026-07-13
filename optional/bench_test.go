package optional_test

import (
	"testing"

	"github.com/v8fg/kit4go/optional"
)

func BenchmarkSomeGet(b *testing.B) {
	o := optional.Some(42)
	b.ResetTimer()
	for b.Loop() {
		o.Get()
	}
}

func BenchmarkUnwrapOr(b *testing.B) {
	o := optional.None[int]()
	b.ResetTimer()
	for b.Loop() {
		o.UnwrapOr(99)
	}
}

func BenchmarkMap(b *testing.B) {
	o := optional.Some(21)
	b.ResetTimer()
	for b.Loop() {
		optional.Map(o, func(x int) int { return x * 2 })
	}
}
