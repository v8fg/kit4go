package bit_test

import (
	"math"
	"testing"

	"github.com/v8fg/kit4go/bit"
)

func BenchmarkMax(b *testing.B) {
	for b.Loop() {
		bit.Max(1, 2)
		bit.Max(1, 1024)
	}
}

func BenchmarkMathMax(b *testing.B) {
	for b.Loop() {
		math.Max(1, 2)
		math.Max(1, 1024)
	}
}
