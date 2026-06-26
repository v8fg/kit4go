package latency

import (
	"sort"
	"time"
)

// DefaultBoundaries are the default histogram bucket upper bounds, tuned for
// low-latency RPC workloads (RTB/DSP/ADX bidding). Each entry is the inclusive
// upper bound of a bucket: a value v lands in bucket i when
// boundaries[i-1] < v <= boundaries[i] (bucket 0 spans (0, boundaries[0]]).
//
// The 1ms–50ms range — the RTB bidding budget — is covered by 9 buckets so a
// percentile can be resolved to ~5ms granularity, which is what tells you
// whether p99 is inside budget. The final bucket (10s) is an overflow
// catch-all so Quantile never returns infinity for a pathological outlier.
var DefaultBoundaries = [...]time.Duration{
	100 * time.Microsecond, //  0: (0,      100µs]
	500 * time.Microsecond, //  1: (100µs,  500µs]
	time.Millisecond,       //  2: (500µs,  1ms]
	2 * time.Millisecond,   //  3: (1ms,    2ms]
	3 * time.Millisecond,   //  4: (2ms,    3ms]
	5 * time.Millisecond,   //  5: (3ms,    5ms]
	10 * time.Millisecond,  //  6: (5ms,   10ms]
	15 * time.Millisecond,  //  7: (10ms,  15ms]
	20 * time.Millisecond,  //  8: (15ms,  20ms]
	30 * time.Millisecond,  //  9: (20ms,  30ms]
	50 * time.Millisecond,  // 10: (30ms,  50ms]   — 50ms budget line
	75 * time.Millisecond,  // 11: (50ms,  75ms]
	100 * time.Millisecond, // 12: (75ms, 100ms]
	200 * time.Millisecond, // 13: (100ms,200ms]
	500 * time.Millisecond, // 14: (200ms,500ms]
	time.Second,            // 15: (500ms, 1s]
	10 * time.Second,       // 16: (1s,    10s]    — overflow
}

// bucketIndex returns the bucket index for v given monotonic-increasing upper
// bounds. v <= boundaries[0] is bucket 0; v greater than the last boundary is
// the last bucket (overflow). O(log len(boundaries)) via binary search;
// boundaries is read-only after construction so the lookup is lock-free.
func bucketIndex(boundaries []time.Duration, v time.Duration) int {
	idx := sort.Search(len(boundaries), func(i int) bool {
		return boundaries[i] >= v
	})
	if idx >= len(boundaries) {
		return len(boundaries) - 1
	}
	return idx
}

// lowerBound returns the inclusive lower bound of bucket i: 0 for bucket 0,
// boundaries[i-1] otherwise. Used for within-bucket linear interpolation in
// Quantile.
func lowerBound(boundaries []time.Duration, i int) time.Duration {
	if i <= 0 {
		return 0
	}
	return boundaries[i-1]
}

// validBoundaries reports whether bs is non-empty, strictly increasing, and all
// positive — the invariants [NewHistogram] requires of caller-supplied bounds.
func validBoundaries(bs []time.Duration) bool {
	if len(bs) == 0 {
		return false
	}
	for i, b := range bs {
		if b <= 0 {
			return false
		}
		if i > 0 && b <= bs[i-1] {
			return false
		}
	}
	return true
}
