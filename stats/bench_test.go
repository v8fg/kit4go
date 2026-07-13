package stats_test

import (
	"testing"

	"github.com/v8fg/kit4go/stats"
)

func makeData(n int) []float64 {
	s := make([]float64, n)
	for i := range n {
		s[i] = float64(i)
	}
	return s
}

func BenchmarkSum(b *testing.B) {
	data := makeData(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = stats.Sum(data)
	}
}

func BenchmarkMean(b *testing.B) {
	data := makeData(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = stats.Mean(data)
	}
}

func BenchmarkMedian(b *testing.B) {
	data := makeData(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = stats.Median(data)
	}
}

func BenchmarkPercentile(b *testing.B) {
	data := makeData(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = stats.Percentile(data, 95)
	}
}

func BenchmarkVariance(b *testing.B) {
	data := makeData(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = stats.Variance(data)
	}
}

func BenchmarkMin(b *testing.B) {
	data := makeData(1000)
	b.ResetTimer()
	for b.Loop() {
		_ = stats.Min(data)
	}
}
