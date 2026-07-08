package budget

// Regression tests for the R13 F4/F5/F6 fixes. Each test FAILS on the pre-fix
// code and passes after the fix:
//
//   - F4 (TestFractionOfPeriod_Non24hMultipleMonotonic, TestTargetSpend_48hLinear,
//     TestTargetSpend_72hLinear, TestTargetSpend_24hStillDaily): a period that is
//     a >= 24h multiple of an hour but != 24h (e.g. 48h, 72h) used to be silently
//     shaped as a repeating 24h daily curve (wall-clock time-of-day), so the plan
//     reset to 0 every midnight and over-paced the first 24h. Now the daily shape
//     fires only for an exactly-24h period; every other period advances linearly
//     via UnixNano()%period.
//   - F5 (TestSmooth_SmoothedRateConcurrent): SmoothedRate() (read) raced Smooth()
//     (write) on p.emaRate, contradicting the "Safe for concurrent read of
//     decision methods" doc. emaRate is now an atomic; concurrent reads under
//     -race are clean.
//   - F6 (TestShouldThrottle_NaNInfThrottles, TestPaceRatio_NaNInfZero,
//     TestShouldThrottle_NegativeClamped, TestPaceRatio_NegativeClamped): a
//     budget-PROTECTION primitive must fail-safe (throttle / zero pace) on bad
//     input. ShouldThrottle(NaN/Inf) previously returned false (full pace);
//     PaceRatio(NaN/Inf) returned 1.0. Now NaN/Inf are treated conservatively
//     (ShouldThrottle -> true, PaceRatio -> 0) and negative spend is clamped to 0.

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// epochAlignedBase returns a midnight UTC whose UnixNano() is a multiple of
// period, so the arbitrary-period branch (UnixNano()%period) starts the plan at
// fraction 0. Used to assert absolute linear values for multi-day periods.
func epochAlignedBase(period time.Duration) time.Time {
	p := int64(period)
	// Scan a few days from a fixed epoch for a midnight aligned to the period.
	origin := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for d := 0; d < 400; d++ {
		base := origin.Add(time.Duration(d) * 24 * time.Hour)
		if base.UnixNano()%p == 0 {
			return base
		}
	}
	// Fallback: epoch itself is always aligned (UnixNano==0).
	return time.Unix(0, 0).UTC()
}

// --- F4 ---

// TestFractionOfPeriod_Non24hMultipleMonotonic is the core F4 regression: across
// one full 48h campaign window (from an epoch-aligned base), TargetSpend must
// NEVER reset/decrease. The old daily-shape code reset to 0 at every midnight
// (the +24h mark), so the plan never reached the total in the first 24h and
// dropped to 0 mid-campaign. With the fix the plan advances monotonically across
// the whole 48h. (At the exact period boundary the next campaign cycle begins and
// the plan wraps to 0; this is the intended repeating-period behavior, so the
// sweep stops one nanosecond before the boundary.)
func TestFractionOfPeriod_Non24hMultipleMonotonic(t *testing.T) {
	p, err := New(480.0, 48*time.Hour)
	require.NoError(t, err)
	// Start at an epoch-aligned base so the sweep covers exactly one campaign.
	base := epochAlignedBase(48 * time.Hour)
	prev := -1.0
	// Sweep 48h minus 1ns (the period wraps to 0 at exactly +48h, which is the
	// next campaign start — out of scope for a single-flight monotonicity check).
	for m := 0; m < 48*60; m += 30 { // every 30 min, exclusive of the boundary
		ts := base.Add(time.Duration(m) * time.Minute)
		cur := p.TargetSpend(ts)
		// Allow tiny float-noise plateaus but never a decrease. The old code
		// dropped from ~480 to 0 at the 24h mark.
		require.GreaterOrEqualf(t, cur, prev-1e-6,
			"TargetSpend decreased at +%dm: prev=%v cur=%v (48h plan must be monotonic)", m, prev, cur)
		// And it must stay within [0, total].
		require.GreaterOrEqualf(t, cur, 0.0, "TargetSpend<0 at +%dm", m)
		require.LessOrEqualf(t, cur, 480.0, "TargetSpend>total at +%dm: %v", m, cur)
		prev = cur
	}
	// The plan must have advanced near the total by end of the 48h campaign
	// (old code oscillated 0..240..0..240 and never reached ~480).
	end := p.TargetSpend(base.Add(48*time.Hour - 1))
	require.Greaterf(t, end, 400.0, "48h plan should be near total near the end, got %v", end)
}

// TestTargetSpend_48hLinear asserts the exact linear values for a 48h campaign
// using an epoch-aligned base (where UnixNano()%48h==0). The task's reproduce
// case: New(480, 48h) -> h=12->120, h=24->240, h=48->480. Fails on old code
// (which gave h=12->240, h=24->0 at any aligned base too, since it used
// time-of-day).
func TestTargetSpend_48hLinear(t *testing.T) {
	p, err := New(480.0, 48*time.Hour)
	require.NoError(t, err)
	base := epochAlignedBase(48 * time.Hour)
	require.Zerof(t, base.UnixNano()%int64(48*time.Hour),
		"test base must be 48h-aligned, got %v", base)
	// Even curve over 48h: fraction = h/48, target = 480*h/48 = 10*h.
	require.InDeltaf(t, 120.0, p.TargetSpend(base.Add(12*time.Hour)), 1.0, "h=12")
	require.InDeltaf(t, 240.0, p.TargetSpend(base.Add(24*time.Hour)), 1.0, "h=24")
	require.InDeltaf(t, 360.0, p.TargetSpend(base.Add(36*time.Hour)), 1.0, "h=36")
	// At exactly 48h the period wraps (UnixNano()%period==0 -> fraction 0), so
	// probe one nanosecond before the boundary to read the end-of-period value.
	require.InDeltaf(t, 480.0, p.TargetSpend(base.Add(48*time.Hour-1)), 1.0, "h=~48")
}

// TestTargetSpend_72hLinear covers a 72h (3-day) campaign with the same
// invariant: linear advancement at an epoch-aligned base. Fails on old code.
func TestTargetSpend_72hLinear(t *testing.T) {
	p, err := New(720.0, 72*time.Hour)
	require.NoError(t, err)
	base := epochAlignedBase(72 * time.Hour)
	require.Zerof(t, base.UnixNano()%int64(72*time.Hour),
		"test base must be 72h-aligned, got %v", base)
	// Even curve over 72h: fraction = h/72, target = 720*h/72 = 10*h.
	require.InDeltaf(t, 120.0, p.TargetSpend(base.Add(12*time.Hour)), 1.0, "h=12")
	require.InDeltaf(t, 240.0, p.TargetSpend(base.Add(24*time.Hour)), 1.0, "h=24")
	require.InDeltaf(t, 480.0, p.TargetSpend(base.Add(48*time.Hour)), 1.0, "h=48")
	require.InDeltaf(t, 720.0, p.TargetSpend(base.Add(72*time.Hour-1)), 1.0, "h=~72")
}

// TestTargetSpend_24hStillDaily confirms the 24h daily case is unchanged: it
// still uses the wall-clock time-of-day shape (h=12->120 for New(240,24h)). This
// guards against over-broadening the F4 fix to break the legitimate daily case.
func TestTargetSpend_24hStillDaily(t *testing.T) {
	p, err := New(240.0, 24*time.Hour)
	require.NoError(t, err)
	// 24h is a daily period: time-of-day shape, so any midnight works as base.
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	require.InDeltaf(t, 120.0, p.TargetSpend(base.Add(12*time.Hour)), 1.0, "h=12")
	require.InDeltaf(t, 240.0, p.TargetSpend(base.Add(24*time.Hour-1)), 1.0, "h=~24")
	// And the even-curve spot checks from the existing suite still hold.
	require.InDeltaf(t, 60.0, p.TargetSpend(base.Add(6*time.Hour)), 1.0, "h=6")
	require.InDeltaf(t, 180.0, p.TargetSpend(base.Add(18*time.Hour)), 1.0, "h=18")
}

// TestFractionOfPeriod_48hIsNotDailyShape directly checks the branch decision:
// a 48h period must NOT route through the daily (time-of-day) branch. On the old
// code fractionOfPeriod(48h, noon) == 0.5 (time-of-day); on the fixed code it is
// 0.0 at an epoch-aligned base (start of the 48h window).
func TestFractionOfPeriod_48hIsNotDailyShape(t *testing.T) {
	p, err := New(480.0, 48*time.Hour)
	require.NoError(t, err)
	base := epochAlignedBase(48 * time.Hour)
	// At the aligned base the 48h fraction must be 0 (start of period), NOT 0.5
	// (which is what the daily time-of-day branch yields at noon). Use a base at
	// noon to make the distinction crisp: daily branch -> 0.5, arbitrary branch
	// at aligned base -> 0.0.
	noon := base.Add(12 * time.Hour)
	f := p.fractionOfPeriod(noon)
	// aligned base + 12h -> arbitrary-branch fraction = 12/48 = 0.25. The daily
	// branch would have returned 0.5 (12h/24h).
	require.InDeltaf(t, 0.25, f, 1e-9,
		"48h period at +12h must use arbitrary branch (0.25), not daily shape (0.5)")
}

// --- F5 ---

// TestSmooth_SmoothedRateConcurrent drives Smooth (writer) and SmoothedRate
// (reader) concurrently under -race. On the old code emaRate was a plain float64
// field read/written without synchronization -> data race. With the atomic it is
// clean. Run with `go test -race -run TestSmooth_SmoothedRateConcurrent`.
func TestSmooth_SmoothedRateConcurrent(t *testing.T) {
	p, err := New(10000.0, 24*time.Hour, WithSmoothing(0.4))
	require.NoError(t, err)

	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	// Seed first so the reader observes non-zero state quickly.
	p.Smooth(0, base)

	var stop atomic.Bool
	var wg sync.WaitGroup

	// Writer: keep updating the EMA.
	wg.Add(1)
	go func() {
		defer wg.Done()
		spend := 0.0
		ts := base
		for !stop.Load() {
			ts = ts.Add(time.Second)
			spend += 10
			_ = p.Smooth(spend, ts)
		}
	}()

	// Reader: concurrent reads of the smoothed rate (the documented
	// "Safe for concurrent read of decision methods" contract).
	const readers = 4
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				r := p.SmoothedRate()
				// Result must always be finite (never a torn-read NaN).
				require.Falsef(t, math.IsNaN(r), "SmoothedRate returned NaN under concurrent Smooth")
				require.Falsef(t, math.IsInf(r, 0), "SmoothedRate returned Inf under concurrent Smooth")
			}
		}()
	}

	// Let the race detector observe contention, then stop. Short enough to keep
	// the suite fast; the race (if present) fires within microseconds.
	time.Sleep(20 * time.Millisecond)
	stop.Store(true)
	wg.Wait()
}

// TestSmoothedRate_ReflectsSmoothUpdate is a non-concurrent functional check
// that the atomic migration did not change SmoothedRate's observable value: it
// still equals the last value Smooth returned.
func TestSmoothedRate_ReflectsSmoothUpdate(t *testing.T) {
	p, _ := New(1000.0, 24*time.Hour, WithSmoothing(0.5))
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	p.Smooth(0, base)
	r := p.Smooth(100, base.Add(time.Hour))
	require.Greater(t, r, 0.0)
	require.InDelta(t, r, p.SmoothedRate(), 1e-12)
}

// --- F6 ---

// TestShouldThrottle_NaNInfThrottles: a budget-PROTECTION primitive must
// fail-safe (throttle) on NaN/Inf actualSpend. Old code returned false (full
// pace) because NaN comparisons are always false.
func TestShouldThrottle_NaNInfThrottles(t *testing.T) {
	p, err := New(240.0, 24*time.Hour, WithTolerance(0.05))
	require.NoError(t, err)
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	require.True(t, p.ShouldThrottle(math.NaN(), ts), "NaN spend must throttle (fail-safe)")
	require.True(t, p.ShouldThrottle(math.Inf(1), ts), "+Inf spend must throttle")
	require.True(t, p.ShouldThrottle(math.Inf(-1), ts), "-Inf spend must throttle")
}

// TestPaceRatio_NaNInfZero: PaceRatio must return 0 (no pace) on NaN/Inf so a
// bad/spend-counter read cannot open the bidding floodgates. Old code returned
// 1.0 (full pace) because the NaN deviation path fell through to the full-pace
// branch.
func TestPaceRatio_NaNInfZero(t *testing.T) {
	p, err := New(240.0, 24*time.Hour, WithTolerance(0.05))
	require.NoError(t, err)
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	require.InDelta(t, 0.0, p.PaceRatio(math.NaN(), ts), 1e-9, "NaN spend -> 0 pace")
	require.InDelta(t, 0.0, p.PaceRatio(math.Inf(1), ts), 1e-9, "+Inf spend -> 0 pace")
	require.InDelta(t, 0.0, p.PaceRatio(math.Inf(-1), ts), 1e-9, "-Inf spend -> 0 pace")
}

// TestShouldThrottle_NegativeClamped: negative cumulative spend is undefined for
// a monotonic spend counter; treat it conservatively as 0 (clamp) rather than
// letting it drift the deviation. A negative spend at mid-day is "behind plan",
// so ShouldThrottle is false (no throttle) — the point is it must not crash or
// produce a nonsense throttle from a negative-total deviation.
func TestShouldThrottle_NegativeClamped(t *testing.T) {
	p, err := New(240.0, 24*time.Hour, WithTolerance(0.05))
	require.NoError(t, err)
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	// Negative spend clamped to 0 -> behind plan (plan=120) -> no throttle.
	require.False(t, p.ShouldThrottle(-50, ts), "negative spend clamped to 0 -> behind plan -> no throttle")
	// Deviation must reflect the clamp (0 spend vs 120 plan -> -1, not a
	// divide-by-something weirdness).
	require.InDelta(t, -1.0, p.Deviation(-50, ts), 1e-9)
}

// TestPaceRatio_NegativeClamped: negative spend clamped to 0 -> behind plan ->
// full pace (1.0), matching the on/behind-plan semantics.
func TestPaceRatio_NegativeClamped(t *testing.T) {
	p, err := New(240.0, 24*time.Hour, WithTolerance(0.05))
	require.NoError(t, err)
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	require.InDelta(t, 1.0, p.PaceRatio(-50, ts), 1e-9, "negative spend clamped -> full pace")
}

// TestOnPlan_NaNFalse: OnPlan on NaN spend must be false (not on plan) rather
// than silently true.
func TestOnPlan_NaNFalse(t *testing.T) {
	p, err := New(240.0, 24*time.Hour, WithTolerance(0.05))
	require.NoError(t, err)
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	require.False(t, p.OnPlan(math.NaN(), ts), "NaN spend is never on plan")
}
