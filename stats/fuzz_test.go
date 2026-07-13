package stats_test

import (
	"math"
	"testing"

	"github.com/v8fg/kit4go/stats"
)

// finite reports whether v is a regular float64 (not NaN/Inf), so it cannot
// pollute the aggregate into NaN.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// FuzzPercentileBounds encodes the percentile invariant: for any non-empty
// slice and p in [0,100], Percentile(p) MUST lie within [Min, Max]; p=0 == Min
// and p=100 == Max. Catches off-by-one / interpolation bugs. E10
// invariant-encoding fuzz target.
func FuzzPercentileBounds(f *testing.F) {
	f.Add(1.0, 2.0, 3.0, 4.0, 5.0, 50.0)
	f.Add(10.0, 10.0, 10.0, 10.0, 10.0, 0.0)
	f.Add(-3.0, -1.0, 0.0, 2.0, 2.0, 99.0)
	f.Fuzz(func(t *testing.T, a, b, c, d, e, p float64) {
		if !finite(p) || p < 0 || p > 100 {
			t.Skip()
		}
		s := []float64{a, b, c, d, e}
		for _, v := range s {
			if !finite(v) {
				t.Skip()
			}
		}
		got := stats.Percentile(s, p)
		lo, hi := stats.Min(s), stats.Max(s)
		if got < lo-1e-9 || got > hi+1e-9 {
			t.Errorf("Percentile(%v, %v)=%v outside [%v, %v]", s, p, got, lo, hi)
		}
		if p == 0 && math.Abs(got-lo) > 1e-9 {
			t.Errorf("Percentile(p=0)=%v != Min=%v", got, lo)
		}
		if p == 100 && math.Abs(got-hi) > 1e-9 {
			t.Errorf("Percentile(p=100)=%v != Max=%v", got, hi)
		}
	})
}

// FuzzMedianEqualsP50 encodes: Median(s) == Percentile(s, 50) for all finite
// non-empty slices. Catches divergence between the two code paths.
func FuzzMedianEqualsP50(f *testing.F) {
	f.Add(1.0, 2.0, 3.0, 4.0, 5.0)
	f.Add(7.0, 7.0, 7.0, 7.0, 7.0)
	f.Add(-2.0, -1.0, 0.0, 1.0, 3.0)
	f.Fuzz(func(t *testing.T, a, b, c, d, e float64) {
		s := []float64{a, b, c, d, e}
		for _, v := range s {
			if !finite(v) {
				t.Skip()
			}
		}
		med := stats.Median(s)
		p50 := stats.Percentile(s, 50)
		if math.Abs(med-p50) > 1e-9 {
			t.Errorf("Median=%v != Percentile(50)=%v for %v", med, p50, s)
		}
	})
}

// FuzzVarianceNonNegative encodes: population variance is always >= 0. Catches
// sign bugs or catastrophic-cancellation regressions.
func FuzzVarianceNonNegative(f *testing.F) {
	f.Add(1.0, 2.0, 3.0, 4.0, 5.0)
	f.Add(5.0, 5.0, 5.0, 5.0, 5.0)
	f.Add(-1e6, 1e6, 0.0, 5e5, -5e5)
	f.Fuzz(func(t *testing.T, a, b, c, d, e float64) {
		s := []float64{a, b, c, d, e}
		for _, v := range s {
			if !finite(v) {
				t.Skip()
			}
		}
		v := stats.Variance(s)
		if v < -1e-9 {
			t.Errorf("Variance=%v < 0 for %v", v, s)
		}
	})
}
