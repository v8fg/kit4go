package stats_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/stats"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9 || (math.IsNaN(a) && math.IsNaN(b))
}

func TestEmptyReturnsNaN(t *testing.T) {
	for _, fn := range []func([]float64) float64{
		stats.Mean, stats.Median, stats.Mode, stats.Variance, stats.StdDev,
		stats.Min, stats.Max, stats.Range,
	} {
		require.True(t, math.IsNaN(fn(nil)), "expected NaN for empty input")
		require.True(t, math.IsNaN(fn([]float64{})), "expected NaN for empty slice")
	}
	// Sum of empty is 0 (defined), not NaN.
	require.Equal(t, 0.0, stats.Sum(nil))
	require.Equal(t, 0.0, stats.Sum([]float64{}))
}

func TestSum(t *testing.T) {
	require.Equal(t, 15.0, stats.Sum([]float64{1, 2, 3, 4, 5}))
	require.Equal(t, -6.0, stats.Sum([]float64{-1, -2, -3}))
	require.Equal(t, 42.0, stats.Sum([]float64{42}))
}

func TestMean(t *testing.T) {
	require.True(t, approxEqual(3.0, stats.Mean([]float64{1, 2, 3, 4, 5})))
	require.True(t, approxEqual(2.0, stats.Mean([]float64{1, 3})))
}

func TestMedian(t *testing.T) {
	// Odd → middle of sorted [1,2,3] is 2.
	require.True(t, approxEqual(2.0, stats.Median([]float64{3, 1, 2})))
	// Even → average of two middle.
	require.True(t, approxEqual(2.5, stats.Median([]float64{1, 2, 3, 4})))
	// Single.
	require.True(t, approxEqual(7.0, stats.Median([]float64{7})))
	// Unsorted input not mutated.
	src := []float64{5, 1, 3}
	_ = stats.Median(src)
	require.Equal(t, []float64{5, 1, 3}, src)
}

func TestMode(t *testing.T) {
	// Clear winner.
	require.True(t, approxEqual(3.0, stats.Mode([]float64{1, 2, 3, 3, 4})))
	// Tie → smallest value among the modes.
	require.True(t, approxEqual(1.0, stats.Mode([]float64{1, 1, 2, 2})))
	// Single.
	require.True(t, approxEqual(5.0, stats.Mode([]float64{5})))
}

func TestVariance(t *testing.T) {
	// Population variance of 1..5: mean=3, variance = (4+1+0+1+4)/5 = 2.
	require.True(t, approxEqual(2.0, stats.Variance([]float64{1, 2, 3, 4, 5})))
	// Constant series → zero variance.
	require.True(t, approxEqual(0.0, stats.Variance([]float64{5, 5, 5})))
}

func TestStdDev(t *testing.T) {
	// StdDev = sqrt(variance). For 1..5: sqrt(2).
	require.True(t, approxEqual(math.Sqrt(2), stats.StdDev([]float64{1, 2, 3, 4, 5})))
	require.True(t, approxEqual(0.0, stats.StdDev([]float64{5, 5, 5})))
}

func TestPercentile(t *testing.T) {
	s := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	require.True(t, approxEqual(1.0, stats.Percentile(s, 0)))
	require.True(t, approxEqual(10.0, stats.Percentile(s, 100)))
	require.True(t, approxEqual(5.5, stats.Percentile(s, 50))) // median
	require.True(t, approxEqual(3.25, stats.Percentile(s, 25)))

	// Out of range → NaN.
	require.True(t, math.IsNaN(stats.Percentile(s, -1)))
	require.True(t, math.IsNaN(stats.Percentile(s, 101)))
	require.True(t, math.IsNaN(stats.Percentile(nil, 50)))

	// Single element.
	require.True(t, approxEqual(42.0, stats.Percentile([]float64{42}, 73)))

	// Exact rank position (no interpolation): n=5 → rank=p/100*4; p=25 → rank=1 → sorted[1]=2.
	require.True(t, approxEqual(2.0, stats.Percentile([]float64{1, 2, 3, 4, 5}, 25)))
}

func TestMinMaxRange(t *testing.T) {
	s := []float64{3, 1, 4, 1, 5, 9, 2, 6}
	require.True(t, approxEqual(1.0, stats.Min(s)))
	require.True(t, approxEqual(9.0, stats.Max(s)))
	require.True(t, approxEqual(8.0, stats.Range(s)))

	// Negatives.
	neg := []float64{-5, -1, -3}
	require.True(t, approxEqual(-5.0, stats.Min(neg)))
	require.True(t, approxEqual(-1.0, stats.Max(neg)))
	require.True(t, approxEqual(4.0, stats.Range(neg)))

	// Single.
	require.True(t, approxEqual(7.0, stats.Min([]float64{7})))
	require.True(t, approxEqual(7.0, stats.Max([]float64{7})))
	require.True(t, approxEqual(0.0, stats.Range([]float64{7})))
}

func TestMinDoesNotMutate(t *testing.T) {
	src := []float64{5, 1, 3}
	_ = stats.Min(src)
	_ = stats.Max(src)
	require.Equal(t, []float64{5, 1, 3}, src)
}
