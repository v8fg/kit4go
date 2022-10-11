package bit_test

import (
	"math"
	"testing"

	"github.com/v8fg/kit4go/bit"
)

func BenchmarkMax(b *testing.B) {
	for i := 0; i < b.N; i++ {
		bit.Max(1, 2)
		bit.Max(1, 1024)
	}
}

func BenchmarkMathMax(b *testing.B) {
	for i := 0; i < b.N; i++ {
		math.Max(1, 2)
		math.Max(1, 1024)
	}
}
