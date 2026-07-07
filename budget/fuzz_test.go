// This file is an internal fuzz test (package budget, not budget_test) so it
// can probe the unexported cumulative weight curve for the normalization and
// monotonicity invariants. It drives contracts the unit tests only spot-check:
//
//   - FuzzDecisionInvariants: New and every decision method (TargetSpend,
//     Deviation, OnPlan, ShouldThrottle, PaceRatio) never panic for any total,
//     period, bucket count, weight vector, tolerance, time and actual spend;
//     the weight curve is normalized to [0,1] and monotonic non-decreasing;
//     TargetSpend stays within [0,total]; PaceRatio stays within [0,1]; and
//     OnPlan / ShouldThrottle stay consistent with Deviation's sign and the
//     configured tolerance.
//   - FuzzSmoothRoundtrip: Smooth never panics across any spend/time schedule
//     (including zero/negative dt and non-monotonic clocks), the returned rate
//     and SmoothedRate agree after each call, and the rate is always finite
//     (no NaN/Inf leaks from divide-by-zero on tiny dt).
//
// These run as ordinary tests (seed corpus only) under `go test -run='^Fuzz'`;
// pass `-fuzz=FuzzDecisionInvariants` / `-fuzz=FuzzSmoothRoundtrip` to expand
// the corpus.
//
// Native fuzz arguments are restricted to primitives and []byte, so the weight
// vector and the (spend, time) schedule are encoded into []byte payloads (one
// byte per weight; a fixed 8-byte stride per schedule tick).
package budget

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// FuzzDecisionInvariants fuzzes a full Pacer configuration plus a probe time
// and actual spend. It asserts, for every input inside New's contract:
//
//  1. New and every decision method never panic.
//  2. The cumulative weight curve is normalized to [0,1] (curve[0]==0,
//     curve[buckets]==1) and monotonic non-decreasing — spend plans must not
//     go backwards within a bucket.
//  3. TargetSpend is within [0, total]; PaceRatio is within [0, 1].
//  4. OnPlan(actual) == |Deviation(actual)| <= tolerance, and ShouldThrottle
//     agrees with Deviation > tolerance (or actual >= total), so the three
//     decision views of the same (actual, t) never disagree.
//
// weightBytes encodes the per-bucket curve one byte per bucket; each byte maps
// linearly to a weight in [0, 255]. An all-zero byte vector reproduces the
// even-fallback path; a single large byte reproduces a dominant (prime-hour)
// bucket. Negatives are covered by TestZeroWeightsFallBackToEven and the
// normalizer's clamp; the fuzzer concentrates on the core invariant space.
func FuzzDecisionInvariants(f *testing.F) {
	// Seeds cover the shapes that matter: even curve, time-of-day shaping,
	// zero weights (even fallback), a single dominant bucket, tiny and
	// non-daily periods, extreme tolerance, and boundary spend.
	f.Add(int64(240), int64(int64(24*time.Hour)), uint8(24),
		[]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		float64(0.05), uint64(12*3600+30*60), float64(120)) // on plan at 12h30m
	f.Add(int64(1000), int64(int64(24*time.Hour)), uint8(24),
		[]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 0, 0, 0, 0},
		float64(0.10), uint64(20*3600), float64(900)) // prime hour, ahead of plan
	f.Add(int64(240), int64(int64(24*time.Hour)), uint8(24),
		make([]byte, 24), float64(0.05), uint64(6*3600), float64(30)) // all-zero weights -> even
	f.Add(int64(400), int64(int64(time.Hour)), uint8(4),
		[]byte{1, 1, 1, 1}, float64(0.05), uint64(30*60), float64(200)) // non-daily 1h period
	f.Add(int64(50), int64(int64(24*time.Hour)), uint8(1),
		[]byte{1}, float64(0.20), uint64(0), float64(50)) // single bucket, spend==total at midnight
	f.Add(int64(1000), int64(int64(24*time.Hour)), uint8(24),
		[]byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
		float64(0.0), uint64(23*3600), float64(999)) // huge weights, zero tolerance, near end of day
	f.Add(int64(1), int64(int64(time.Second)), uint8(2),
		[]byte{1, 1}, float64(0.5), uint64(0), float64(0)) // smallest valid config
	f.Add(int64(100), int64(int64(24*time.Hour)), uint8(24),
		[]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		float64(0.05), uint64(13*3600), float64(60)) // mid-day dominant bucket

	f.Fuzz(func(t *testing.T,
		total int64, periodNs int64, buckets uint8,
		weightBytes []byte, tolerance float64,
		secOfDay uint64, actualSpend float64,
	) {
		// New's documented contract: total > 0 and period > 0. Anything else
		// returns ErrBudget (asserted by TestInvalidInput); skip the rest so we
		// fuzz the decision methods hard rather than the error branch.
		if total <= 0 || periodNs <= 0 {
			t.Skip("total<=0 or period<=0 is outside New's documented contract")
		}
		// Reject non-finite fuzzer inputs for tolerance/spend: NaN/Inf are not
		// values a real pacing system feeds in, and asserting range bounds
		// against them is meaningless. The no-panic guard below still wraps
		// everything so a future normalization change can't silently crash.
		if !isFinite(tolerance) || tolerance < 0 {
			t.Skip("non-finite or negative tolerance is not a realistic pacing input")
		}
		if !isFinite(actualSpend) || actualSpend < 0 {
			t.Skip("non-finite or negative spend is not a realistic pacing input")
		}

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked: total=%d period=%v buckets=%d weights=%v tol=%v sec=%d spend=%v recover=%v",
					total, time.Duration(periodNs), buckets, weightBytes, tolerance, secOfDay, actualSpend, r)
			}
		}()

		// Decode the byte vector into per-bucket weights. Bytes beyond the
		// configured bucket count are ignored (the constructor only reads the
		// first `buckets` entries); missing bytes default to even (weight 1),
		// which is the documented WithWeights behavior.
		weights := make([]float64, buckets)
		for i := range weights {
			if i < len(weightBytes) {
				weights[i] = float64(weightBytes[i])
			} else {
				weights[i] = 1
			}
		}

		opts := []Option{
			WithBuckets(int(buckets)),
			WithWeights(weights),
			WithTolerance(tolerance),
		}
		p, err := New(float64(total), time.Duration(periodNs), opts...)
		require.NoErrorf(t, err, "New failed inside contract: total=%d period=%v", total, time.Duration(periodNs))

		// Invariant 2: the cumulative weight curve is normalized to [0,1] and
		// monotonic non-decreasing. curve[0] is always 0 by construction and
		// curve[buckets] is pinned to 1 to guard float drift; the interior must
		// never step backwards or the planned spend could decrease within a day.
		require.Equalf(t, 0.0, p.weight[0],
			"curve must start at 0: total=%d buckets=%d weights=%v", total, buckets, weights)
		require.InDeltaf(t, 1.0, p.weight[p.buckets], 1e-9,
			"curve must end at 1: total=%d buckets=%d weights=%v", total, buckets, weights)
		for i := 0; i < p.buckets; i++ {
			require.GreaterOrEqualf(t, p.weight[i+1], p.weight[i],
				"curve not monotonic at i=%d: total=%d buckets=%d weights=%v", i, total, buckets, weights)
		}

		// Probe time: clamp secOfDay into a single day so the fraction-of-period
		// math is well-defined for both daily and sub-daily periods.
		daySec := uint64(24 * 3600)
		sec := secOfDay % daySec
		probe := time.Unix(int64(sec), 0).UTC()

		// Invariant 3: TargetSpend is within [0, total].
		target := p.TargetSpend(probe)
		require.GreaterOrEqualf(t, target, 0.0,
			"TargetSpend<0: total=%d sec=%d", total, sec)
		require.LessOrEqualf(t, target, float64(total),
			"TargetSpend>total: total=%d sec=%d target=%v", total, sec, target)

		// Invariant 4: the three decision views of (actual, t) agree. OnPlan is
		// exactly |Deviation| <= tolerance; ShouldThrottle is exactly
		// Deviation > tolerance (or actual >= total, the exhausted-budget case).
		dev := p.Deviation(actualSpend, probe)
		onPlan := p.OnPlan(actualSpend, probe)
		require.Equalf(t, math.Abs(dev) <= p.tolerance, onPlan,
			"OnPlan disagrees with Deviation: total=%d sec=%d spend=%v dev=%v tol=%v",
			total, sec, actualSpend, dev, p.tolerance)

		throttle := p.ShouldThrottle(actualSpend, probe)
		wantThrottle := actualSpend >= float64(total) || dev > p.tolerance
		require.Equalf(t, wantThrottle, throttle,
			"ShouldThrottle disagrees with Deviation: total=%d sec=%d spend=%v dev=%v tol=%v",
			total, sec, actualSpend, dev, p.tolerance)

		// Invariant 3 (cont.): PaceRatio is within [0, 1].
		ratio := p.PaceRatio(actualSpend, probe)
		require.GreaterOrEqualf(t, ratio, 0.0,
			"PaceRatio<0: total=%d sec=%d spend=%v", total, sec, actualSpend)
		require.LessOrEqualf(t, ratio, 1.0,
			"PaceRatio>1: total=%d sec=%d spend=%v ratio=%v", total, sec, actualSpend, ratio)

		// PaceRatio must be consistent with ShouldThrottle: when not throttling
		// the pacer runs at full pace (1.0); when budget is exhausted it is 0.
		if actualSpend >= float64(total) {
			require.InDeltaf(t, 0.0, ratio, 1e-9,
				"PaceRatio must be 0 when budget exhausted: total=%d spend=%v", total, actualSpend)
		} else if !throttle {
			require.InDeltaf(t, 1.0, ratio, 1e-9,
				"PaceRatio must be 1.0 when not throttling: total=%d sec=%d spend=%v",
				total, sec, actualSpend)
		}
	})
}

// FuzzSmoothRoundtrip fuzzes a schedule of (cumulative spend, time) samples fed
// to Smooth, with an optional EMA alpha. The schedule is packed into a []byte
// payload as a sequence of 8-byte ticks: 4 bytes little-endian uint32 spend
// (scaled to float64) and 4 bytes little-endian int32 seconds. It asserts, for
// every input:
//
//  1. Smooth never panics — including zero dt, negative dt (non-monotonic
//     clock), and a first-call seed — and never returns NaN or Inf (no
//     divide-by-zero leak from a tiny dt).
//  2. SmoothedRate matches the value Smooth just returned (roundtrip: the
//     persisted EMA is the same value handed back to the caller).
//  3. When dt <= 0 Smooth returns the prior rate unchanged (no update, no
//     divide-by-zero); the first call always seeds and returns 0.
//
// alpha is the EMA smoothing factor (alpha <= 0 disables smoothing).
func FuzzSmoothRoundtrip(f *testing.F) {
	// Seeds cover the shapes that matter: monotonic increase, a flat segment
	// (zero dt rate), a non-monotonic clock jump backwards, a spend decrease
	// (negative rate), smoothing on vs off, and the single-tick seed path.
	// Each tick is 8 bytes: [spend u32 LE][sec i32 LE].
	f.Add([]byte{
		0, 0, 0, 0, 0, 0, 0, 0, // spend=0,  sec=0
		100, 0, 0, 0, 0x10, 0x0E, 0, 0, // spend=100, sec=3600
		100, 0, 0, 0, 0x10, 0x0E, 0, 0, // spend=100, sec=3600 (zero dt)
		200, 0, 0, 0, 0x20, 0x1C, 0, 0, // spend=200, sec=7200
	}, float64(0.5))
	f.Add([]byte{
		0, 0, 0, 0, 0, 0, 0, 0,
		100, 0, 0, 0, 0x10, 0x0E, 0, 0,
		200, 0, 0, 0, 0x08, 0x07, 0, 0, // sec=1800: clock goes backwards
	}, float64(0.3))
	f.Add([]byte{
		0, 0, 0, 0, 0, 0, 0, 0,
		0xFF, 0xFF, 0xFF, 0x7F, 1, 0, 0, 0, // spend=max uint32, sec=1
	}, float64(0.0)) // smoothing off, huge spend
	f.Add([]byte{
		100, 0, 0, 0, 0, 0, 0, 0, // single tick (seed only)
	}, float64(0.5))
	f.Add([]byte{
		0, 0, 0, 0, 0, 0, 0, 0,
		50, 0, 0, 0, 0, 0, 0, 0, // zero dt
		50, 0, 0, 0, 0, 0, 0, 0, // zero dt
		200, 0, 0, 0, 0x10, 0x0E, 0, 0, // advance
	}, float64(0.5))
	f.Add([]byte{}, float64(0.5)) // empty schedule: just construct
	f.Add([]byte{
		0, 0, 0, 0, 0, 0, 0, 0,
		100, 0, 0, 0, 0x10, 0x0E, 0, 0,
		50, 0, 0, 0, 0x20, 0x1C, 0, 0, // spend decreases (negative rate)
	}, float64(0.2))

	f.Fuzz(func(t *testing.T, schedule []byte, alpha float64) {
		if !isFinite(alpha) {
			t.Skip("non-finite alpha is not a realistic smoothing factor")
		}

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked: schedule=%x alpha=%v recover=%v", schedule, alpha, r)
			}
		}()

		p, err := New(1000.0, 24*time.Hour, WithSmoothing(alpha))
		require.NoError(t, err, "New(1000, 24h) must succeed")

		// Decode the schedule into (spend, time) ticks at an 8-byte stride.
		// A trailing partial tick (< 8 bytes) is dropped — it is a harness
		// artifact of the byte fuzzer, not a meaningful pacing input.
		const stride = 8
		n := len(schedule) / stride

		var prevTime time.Time
		var prevRate float64
		for i := 0; i < n; i++ {
			off := i * stride
			spend := float64(binary.LittleEndian.Uint32(schedule[off : off+4]))
			sec := int32(binary.LittleEndian.Uint32(schedule[off+4 : off+8]))
			ts := time.Unix(int64(sec), 0).UTC()

			rate := p.Smooth(spend, ts)

			// Invariant 1: Smooth never returns NaN/Inf — a divide-by-zero on
			// a tiny dt must be absorbed, not leaked to the caller.
			require.Truef(t, isFinite(rate),
				"Smooth returned non-finite rate: schedule=%x alpha=%v i=%d rate=%v",
				schedule, alpha, i, rate)

			// Invariant 2: SmoothedRate roundtrips — it equals the last value
			// Smooth returned.
			require.InDeltaf(t, rate, p.SmoothedRate(), 1e-9,
				"SmoothedRate != Smooth result: schedule=%x alpha=%v i=%d",
				schedule, alpha, i)

			if i == 0 {
				// First call seeds and returns 0 regardless of input.
				require.InDeltaf(t, 0.0, rate, 1e-9,
					"first Smooth must seed and return 0: schedule=%x", schedule)
			} else {
				dt := ts.Sub(prevTime).Seconds()
				if dt <= 0 {
					// Invariant 3 (zero/negative dt): Smooth returns the prior
					// rate unchanged — no update, no divide-by-zero.
					require.InDeltaf(t, prevRate, rate, 1e-9,
						"dt<=0 must return prior rate: schedule=%x alpha=%v i=%d dt=%v",
						schedule, alpha, i, dt)
				}
				// (dt > 0 bounds are configuration-dependent once the EMA has
				// history; the roundtrip + finite checks above carry the value.)
			}

			prevTime = ts
			prevRate = rate
		}
	})
}

// isFinite reports whether x is a finite float (not NaN, not +/-Inf).
func isFinite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
