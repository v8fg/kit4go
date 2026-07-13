package stats_test

import (
	"fmt"

	"github.com/v8fg/kit4go/stats"
)

// ExampleMean computes the arithmetic mean of a latency sample.
func ExampleMean() {
	latency := []float64{12.1, 14.3, 11.8, 13.0, 99.2}
	fmt.Printf("%.2f\n", stats.Mean(latency))
	// Output:
	// 30.08
}

// ExampleMedian is robust to outliers — the p50 latency.
func ExampleMedian() {
	latency := []float64{12.1, 14.3, 11.8, 13.0, 99.2}
	fmt.Printf("%.2f\n", stats.Median(latency))
	// Output:
	// 13.00
}

// ExamplePercentile computes the p95 latency.
func ExamplePercentile() {
	latency := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	fmt.Printf("%.2f\n", stats.Percentile(latency, 95))
	// Output:
	// 9.55
}

// ExampleStdDev measures dispersion of a sample.
func ExampleStdDev() {
	sample := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	fmt.Printf("%.4f\n", stats.StdDev(sample))
	// Output:
	// 2.0000
}
