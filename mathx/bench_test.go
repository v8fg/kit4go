package mathx_test

import (
	"testing"

	"github.com/v8fg/kit4go/mathx"
)

func BenchmarkSum(b *testing.B) {
	vals := make([]int, 1000)
	for i := range 1000 {
		vals[i] = i
	}
	b.ResetTimer()
	for b.Loop() {
		mathx.Sum(vals...)
	}
}

func BenchmarkClamp(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		mathx.Clamp(50, 0, 100)
	}
}

func BenchmarkLerp(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		mathx.Lerp(0.0, 100.0, 0.5)
	}
}
