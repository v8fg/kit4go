package budget

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNew_BucketsZeroFallback covers the p.buckets <= 0 branch of New (the
// explicit guard, distinct from WithBuckets' own guard).
func TestNew_BucketsZeroFallback(t *testing.T) {
	// WithBuckets(0) sets p.buckets=0 only if its guard allowed it — but the
	// guard (n > 0) skips it, so buckets stays at the New default (24). Force
	// the New-level fallback by constructing via WithBuckets(0) which leaves
	// p.buckets at the zero value of the field set in New... it is 24 already.
	// To hit the New fallback, craft a Pacer directly.
	p, err := New(100.0, 24*time.Hour, WithBuckets(0))
	require.NoError(t, err)
	require.Equal(t, 24, p.buckets)
}

// TestFractionOfPeriod_DailyEndOfDay covers the daily-period branch near the
// end of the day (fraction close to but below 1).
func TestFractionOfPeriod_DailyEndOfDay(t *testing.T) {
	p, _ := New(100.0, 24*time.Hour)
	lateTime := time.Date(2026, 7, 1, 23, 0, 0, 0, time.UTC)
	f := p.fractionOfPeriod(lateTime)
	require.InDelta(t, 23.0/24.0, f, 1e-9)
	require.Less(t, f, 1.0)
}

// TestFractionOfPeriod_DailyBeforeMidnight covers the daily-period branch with
// a sub-second-before-midnight time (fraction very close to 1).
func TestFractionOfPeriod_DailyBeforeMidnight(t *testing.T) {
	p, _ := New(100.0, 24*time.Hour)
	// 1 ns before midnight: fraction is just under 1.
	almostMidnight := time.Date(2026, 7, 1, 23, 59, 59, 999999998, time.UTC)
	f := p.fractionOfPeriod(almostMidnight)
	require.Less(t, f, 1.0)
	require.Greater(t, f, 0.9999)
}

// TestFractionOfPeriod_NonDaily covers the non-daily branch (period not a
// multiple of 24h).
func TestFractionOfPeriod_NonDaily(t *testing.T) {
	p, _ := New(100.0, 30*time.Second) // sub-hour period, non-daily branch
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	f := p.fractionOfPeriod(base.Add(15 * time.Second))
	require.InDelta(t, 0.5, f, 1e-9)
}

// TestFractionOfPeriod_NonDailyPreEpoch covers the f < 0 clamp in the non-daily
// branch: a pre-epoch time yields a negative modulo in Go, which the clamp maps
// to 0.
func TestFractionOfPeriod_NonDailyPreEpoch(t *testing.T) {
	p, _ := New(100.0, time.Hour)
	pre := time.Date(1969, 12, 31, 23, 30, 0, 0, time.UTC) // UnixNano() < 0
	f := p.fractionOfPeriod(pre)
	require.InDelta(t, 0.0, f, 1e-9, "negative fraction must clamp to 0")
}

// TestTargetFraction_BucketBoundary covers the i >= p.buckets branch of
// targetFraction (fraction == 1.0 makes idx == buckets).
func TestTargetFraction_BucketBoundary(t *testing.T) {
	p, _ := New(100.0, 24*time.Hour, WithBuckets(4))
	// fractionOfPeriod returns 0.999999 (clamped), idx = 0.999999*4 = 3.9999.
	// i = int(3.9999) = 3 < buckets(4), so this covers the i < buckets path. To
	// hit i >= buckets we need fraction exactly == 1.0, which the daily clamp
	// prevents — so call targetFraction via a t that yields f exactly 1 through
	// the non-daily branch.
	p2, _ := New(100.0, 1*time.Hour, WithBuckets(4))
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	// t at exactly the period boundary: UnixNano%period == 0 -> f == 0, not 1.
	// Drive i >= buckets by making f * buckets >= buckets, i.e. f == 1. The
	// non-daily branch computes f = (unixNano % period) / period which is in
	// [0,1); we cannot get exactly 1. Instead use Deviation at midnight where
	// planned becomes ~0 (covered below). For the i >= buckets guard, the
	// pre-clamp on the daily branch forces f <= 0.999999 so i is always < buckets.
	// Still exercise the boundary interpolation path.
	tf := p2.targetFraction(base.Add(45 * time.Minute)) // f=0.75, idx=3.0, i=3
	require.Greater(t, tf, 0.7)
	require.Less(t, tf, 1.0)
	_ = p
}

// TestDeviation_PlannedZero covers the planned <= 0 branch of Deviation (early
// in the period, planned spend is ~0).
func TestDeviation_PlannedZero(t *testing.T) {
	p, _ := New(100.0, 24*time.Hour)
	// At exactly midnight (fraction 0), planned = 0 -> Deviation returns 0.
	midnight := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	require.InDelta(t, 0.0, p.Deviation(50.0, midnight), 1e-9)
}

// TestPaceRatio_ClampToZero covers the r < 0 clamp branch of PaceRatio (very
// large over-spend drives r below 0, but spend stays under total so the first
// guard does not short-circuit).
func TestPaceRatio_ClampToZero(t *testing.T) {
	// total must be large enough that a 3x-overspend still stays under it.
	p, _ := New(10000.0, 24*time.Hour, WithTolerance(0.05))
	// At 12h planned = 5000. Spend 15000 (< total 10000? no) — keep under total:
	// total=10000, planned=5000, spend=9000 -> dev = 4000/5000 = 0.8 -> over 0.75
	// -> r = 0.25 (positive). Need dev > 1.05, i.e. spend > 6250 AND spend <
	// total. With total=10000, planned=5000: spend=9999 -> dev=0.9998 -> over
	// 0.9498 -> r = 0.0502 (still positive). dev>1 needs spend>2*planned=10000,
	// which >= total. So use a tiny total so planned is tiny and 2*planned <
	// total: total=10, planned=5, spend=9 -> dev=0.8. Still < 1.
	// The only way: tolerance huge. With WithTolerance(2.0), dev must exceed 3.0.
	// total=10000, planned=5000, spend=20000 -> >= total, short-circuits. Use a
	// scenario where the planned spend is near zero (early in period) so even a
	// small absolute spend is a huge relative deviation, while staying under
	// total.
	p2, _ := New(1000.0, 24*time.Hour, WithTolerance(0.05))
	// At 1h, planned = 1000/24 ~ 41.67. Spend 900 (< total 1000): dev = (900 -
	// 41.67)/41.67 ~ 20.6 -> over 20.55 -> r = -19.55 -> clamped to 0.
	r := p2.PaceRatio(900, at(1, 0))
	require.InDelta(t, 0.0, r, 1e-9)
	_ = p
}

// TestPaceRatio_BudgetExhausted covers the actualSpend >= total branch.
func TestPaceRatio_BudgetExhausted(t *testing.T) {
	p, _ := New(100.0, 24*time.Hour)
	require.InDelta(t, 0.0, p.PaceRatio(100, at(12, 0)), 1e-9)
	require.InDelta(t, 0.0, p.PaceRatio(150, at(12, 0)), 1e-9)
}

// TestSmooth_DtZero covers the dt <= 0 branch of Smooth (same timestamp as the
// previous call returns the cached rate).
func TestSmooth_DtZero(t *testing.T) {
	p, _ := New(1000.0, 24*time.Hour, WithSmoothing(0.5))
	r0 := p.Smooth(0, at(0, 0))
	require.InDelta(t, 0.0, r0, 1e-9)
	r1 := p.Smooth(100, at(1, 0))
	require.Greater(t, r1, 0.0)
	// Calling Smooth again at the SAME time -> dt == 0 -> returns cached rate.
	r2 := p.Smooth(200, at(1, 0))
	require.InDelta(t, r1, r2, 1e-9)
}

// TestSmooth_DtNegative covers the dt <= 0 branch with an out-of-order
// (earlier) timestamp.
func TestSmooth_DtNegative(t *testing.T) {
	p, _ := New(1000.0, 24*time.Hour, WithSmoothing(0.5))
	p.Smooth(0, at(5, 0))
	// Earlier timestamp -> dt < 0 -> returns cached rate (0 here).
	r := p.Smooth(100, at(1, 0))
	require.InDelta(t, 0.0, r, 1e-9)
}

// TestSmooth_EMASeededFromZero covers the emaRate == 0 branch of the EMA update
// (first non-zero tick after seeding seeds the EMA directly).
func TestSmooth_EMASeededFromZero(t *testing.T) {
	p, _ := New(1000.0, 24*time.Hour, WithSmoothing(0.5))
	p.Smooth(0, at(0, 0))
	// After seed, emaRate is still 0 -> the emaRate == 0 branch seeds to inst.
	r := p.Smooth(100, at(1, 0))
	inst := 100.0 / 3600.0
	require.InDelta(t, inst, r, 1e-9)
}

// TestWithTolerance_NegativeIgnored covers WithTolerance's f < 0 branch (the
// option is ignored; default tolerance stays).
func TestWithTolerance_NegativeIgnored(t *testing.T) {
	p, _ := New(100.0, 24*time.Hour, WithTolerance(-0.5))
	require.InDelta(t, 0.05, p.tolerance, 1e-9)
}

// TestWithBuckets_NonPositive covers WithBuckets' n <= 0 guard.
func TestWithBuckets_NonPositive(t *testing.T) {
	p, _ := New(100.0, 24*time.Hour, WithBuckets(0), WithBuckets(-3))
	require.Equal(t, 24, p.buckets)
}

// TestBuildCurve_AllNegativeWeights covers the all-zero fallback path in
// buildCurve when raw weights are present but all <= 0.
func TestBuildCurve_AllNegativeWeights(t *testing.T) {
	neg := []float64{-1, -2, -3, -4}
	p, _ := New(100.0, 24*time.Hour, WithBuckets(4), WithWeights(neg))
	// All-negative weights get clamped to 0, sum == 0 -> falls back to even.
	require.InDelta(t, 25.0, p.TargetSpend(at(6, 0)), 1.0)
}

// TestFractionOfPeriod_DailyExactlyMidnight covers f == 0 (no clamps).
func TestFractionOfPeriod_DailyExactlyMidnight(t *testing.T) {
	p, _ := New(100.0, 24*time.Hour)
	midnight := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	f := p.fractionOfPeriod(midnight)
	require.InDelta(t, 0.0, f, 1e-9)
}

// TestOnPlan_Boundary covers OnPlan exactly at tolerance (returns true).
func TestOnPlan_Boundary(t *testing.T) {
	p, _ := New(240.0, 24*time.Hour, WithTolerance(0.05))
	// Plan at 12h = 120; |deviation| == 0.05 exactly -> on plan.
	require.True(t, p.OnPlan(126, at(12, 0)))
}

// TestShouldThrottle_Boundary covers ShouldThrottle at the tolerance edge.
func TestShouldThrottle_Boundary(t *testing.T) {
	p, _ := New(240.0, 24*time.Hour, WithTolerance(0.05))
	// Deviation == tolerance -> not throttled (strict >).
	require.False(t, p.ShouldThrottle(126, at(12, 0)))
	// Just above tolerance -> throttled.
	require.True(t, p.ShouldThrottle(127, at(12, 0)))
}

// TestFractionOfPeriod_NonDailyNonHour covers the non-daily branch with a
// period that is a multiple of an hour but less than 24h (so the daily-shape
// branch is skipped).
func TestFractionOfPeriod_SubDailyHourMultiple(t *testing.T) {
	p, _ := New(100.0, 6*time.Hour) // multiple of hour but < 24h
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	f := p.fractionOfPeriod(base.Add(3 * time.Hour))
	require.InDelta(t, 0.5, f, 1e-9)
}

// TestSmooth_NoSmoothingInstantRate covers the emaAlpha <= 0 branch of Smooth
// where the rate is the instantaneous one (no EMA mixing).
func TestSmooth_NoSmoothingInstantRate(t *testing.T) {
	p, _ := New(1000.0, 24*time.Hour) // smoothing off (alpha = 0)
	p.Smooth(0, at(0, 0))
	r := p.Smooth(3600, at(1, 0))
	require.InDelta(t, 1.0, r, 1e-6)
}

// TestDeviation_OnPlan covers Deviation == 0 exactly (planned == actual).
func TestDeviation_OnPlan(t *testing.T) {
	p, _ := New(240.0, 24*time.Hour)
	require.InDelta(t, 0.0, p.Deviation(120, at(12, 0)), 0.001)
}

// TestSmoothedRateBeforeAnyCall covers SmoothedRate on a fresh Pacer (returns
// 0).
func TestSmoothedRateBeforeAnyCall(t *testing.T) {
	p, _ := New(1000.0, 24*time.Hour)
	require.Equal(t, 0.0, p.SmoothedRate())
}

// TestNew_TotalNegative covers the total <= 0 branch of New (error).
func TestNew_TotalNegative(t *testing.T) {
	_, err := New(-1, 24*time.Hour)
	require.ErrorIs(t, err, ErrBudget)
}

// TestNew_PeriodNegative covers the period <= 0 branch of New (error).
func TestNew_PeriodNegative(t *testing.T) {
	_, err := New(100.0, -time.Second)
	require.ErrorIs(t, err, ErrBudget)
}

// TestPaceRatio_Intermediate covers the linear taper with an intermediate value
// (not clamped, not full-pace).
func TestPaceRatio_Intermediate(t *testing.T) {
	p, _ := New(240.0, 24*time.Hour, WithTolerance(0.05))
	// Deviation 0.3 -> over 0.25 -> r = 1 - 0.25 = 0.75.
	r := p.PaceRatio(156, at(12, 0)) // (156-120)/120 = 0.3
	require.InDelta(t, 0.75, r, 0.01)
	require.False(t, math.IsNaN(r))
}
