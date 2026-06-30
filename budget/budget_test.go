package budget

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func at(h, m int) time.Time { return time.Date(2026, 7, 1, h, m, 0, 0, time.UTC) }

func TestInvalidInput(t *testing.T) {
	_, err := New(0, 24*time.Hour)
	require.ErrorIs(t, err, ErrBudget)
	_, err = New(100, 0)
	require.ErrorIs(t, err, ErrBudget)
}

func TestEvenCurveIsLinear(t *testing.T) {
	// Default weights (even): TargetSpend grows linearly across the day.
	p, _ := New(240.0, 24*time.Hour)
	// At 25% of the day (6h), planned spend ~25% of total.
	require.InDelta(t, 60.0, p.TargetSpend(at(6, 0)), 1.0)
	require.InDelta(t, 120.0, p.TargetSpend(at(12, 0)), 1.0)
	require.InDelta(t, 180.0, p.TargetSpend(at(18, 0)), 1.0)
}

func TestTimeOfDayWeightsShape(t *testing.T) {
	// Heavily weight hour 19-20 (prime); spend should be back-loaded there.
	w := make([]float64, 24)
	for i := range w {
		w[i] = 1
	}
	w[19] = 100 // prime hour dominates
	p, _ := New(1000.0, 24*time.Hour, WithWeights(w))
	// Before prime hour, spend is small (most budget reserved for 19h).
	before := p.TargetSpend(at(18, 0))
	after := p.TargetSpend(at(20, 0))
	require.Greater(t, after-before, 500.0, "prime hour should absorb most budget")
	require.Less(t, before, 300.0, "little spent before prime hour")
}

func TestShouldThrottleAheadOfPlan(t *testing.T) {
	p, _ := New(240.0, 24*time.Hour, WithTolerance(0.05))
	// At 12h, plan = 120. Spending 150 (25% over) -> throttle.
	require.True(t, p.ShouldThrottle(150, at(12, 0)))
	// Spending 120 (on plan) -> no throttle.
	require.False(t, p.ShouldThrottle(120, at(12, 0)))
	// Behind plan (spend 60) -> no throttle.
	require.False(t, p.ShouldThrottle(60, at(12, 0)))
}

func TestBudgetExhaustedThrottles(t *testing.T) {
	p, _ := New(100.0, 24*time.Hour)
	require.True(t, p.ShouldThrottle(100, at(12, 0)))
	require.True(t, p.ShouldThrottle(101, at(12, 0)))
}

func TestPaceRatioTapers(t *testing.T) {
	p, _ := New(240.0, 24*time.Hour, WithTolerance(0.05))
	// On plan (spend 120 at 12h): full pace.
	require.InDelta(t, 1.0, p.PaceRatio(120, at(12, 0)), 1e-9)
	// Behind plan: full pace.
	require.InDelta(t, 1.0, p.PaceRatio(60, at(12, 0)), 1e-9)
	// 50% over plan: ~0.45 pace (1 - 0.5 + tolerance ...).
	r := p.PaceRatio(180, at(12, 0))
	require.Greater(t, r, 0.0)
	require.Less(t, r, 0.6)
	// Way over (2x): 0 pace.
	require.InDelta(t, 0.0, p.PaceRatio(300, at(12, 0)), 1e-9)
}

func TestDeviationSign(t *testing.T) {
	p, _ := New(240.0, 24*time.Hour)
	require.Positive(t, p.Deviation(150, at(12, 0))) // ahead
	require.Negative(t, p.Deviation(60, at(12, 0)))  // behind
	require.InDelta(t, 0.0, p.Deviation(120, at(12, 0)), 0.05)
}

func TestOnPlanTolerance(t *testing.T) {
	p, _ := New(240.0, 24*time.Hour, WithTolerance(0.10))
	// Plan at 12h = 120; ±10% = [108, 132].
	require.True(t, p.OnPlan(120, at(12, 0)))
	require.True(t, p.OnPlan(110, at(12, 0)))
	require.False(t, p.OnPlan(140, at(12, 0)))
}

func TestSmoothEMA(t *testing.T) {
	p, _ := New(1000.0, 24*time.Hour, WithSmoothing(0.5))
	// First call seeds (returns 0).
	r0 := p.Smooth(0, at(0, 0))
	require.InDelta(t, 0.0, r0, 1e-9)
	// Spend 100 over 1 hour (3600s) -> inst rate ~0.0278/s; EMA seeds to it.
	r1 := p.Smooth(100, at(1, 0))
	require.Greater(t, r1, 0.0)
	// SmoothedRate persists.
	require.InDelta(t, r1, p.SmoothedRate(), 1e-9)

	// Two ticks: a normal rate, then a big spike. EMA damps the spike below
	// its instantaneous value.
	inst := 1000.0 / 3600.0 // the spike's instantaneous rate (1000 in 1h)
	p2, _ := New(1000.0, 24*time.Hour, WithSmoothing(0.3))
	p2.Smooth(0, at(0, 0))
	p2.Smooth(100, at(1, 0))       // normal rate: 100/3600 ~ 0.028/s -> seeds EMA
	r := p2.Smooth(1100, at(2, 0)) // spike: +1000 in 1h -> inst 0.278/s
	require.Less(t, r, inst, "EMA should damp the spike below instantaneous")
	require.Greater(t, r, 0.028) // but above the prior normal rate
}

func TestSmoothDisabledIsInstantaneous(t *testing.T) {
	p, _ := New(1000.0, 24*time.Hour) // smoothing off
	p.Smooth(0, at(0, 0))
	r := p.Smooth(3600, at(1, 0)) // 3600 in 3600s = 1/s
	require.InDelta(t, 1.0, r, 1e-6)
}

func TestNonDailyPeriod(t *testing.T) {
	// A 1-hour period with 4 buckets (15-min each), even weights.
	p, _ := New(400.0, time.Hour, WithBuckets(4))
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	// At 30min (half), planned ~200.
	require.InDelta(t, 200.0, p.TargetSpend(base.Add(30*time.Minute)), 5.0)
}

func TestZeroWeightsFallBackToEven(t *testing.T) {
	// All 24 buckets zero -> falls back to even (linear).
	p, _ := New(240.0, 24*time.Hour, WithWeights(make([]float64, 24)))
	require.InDelta(t, 120.0, p.TargetSpend(at(12, 0)), 1.0)
}

func TestTotalAndPeriod(t *testing.T) {
	p, _ := New(500.0, 12*time.Hour)
	require.Equal(t, 500.0, p.Total())
	require.Equal(t, 12*time.Hour, p.Period())
}
