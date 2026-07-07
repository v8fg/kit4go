// This file is an internal coverage test (package latency) so it can reach the
// unexported helpers (quantileFromMerged, h.base/h.windowSec) and exercise the
// branches the public API either can't reach directly or only reaches through
// awkward timing: the Observe clock-regression clamp, AutoShardCount's low-core
// clamp, the sharded Quantile/Snapshot edge paths, and quantileFromMerged's
// defensive clamps.
package latency

import (
	"runtime"
	"testing"
	"time"
)

// TestObserve_ClockRegressionClamp forces base into the future and confirms
// Observe routes a stale/regressed second to the current bucket (the `if sec <
// h.base` clamp) instead of dropping or mis-placing the sample.
func TestObserve_ClockRegressionClamp(t *testing.T) {
	h := NewHistogram(Options{})
	h.mu.Lock()
	h.base = time.Now().Add(60 * time.Second).Unix() // base in the future
	h.mu.Unlock()
	h.Observe(5 * time.Millisecond) // now < base -> clamp to base's bucket
	if s := h.Snapshot(); s.Count != 1 {
		t.Fatalf("Count=%d want 1 (regressed-second sample was dropped)", s.Count)
	}
}

// TestQuantileFromMerged_DefensiveBranches covers the target>total clamp and
// the post-loop fallback. Both are unreachable through the public API (Quantile
// clamps q to [0,1] and Snapshot passes a consistent total == sum(merged)), so
// they are driven here with crafted inputs.
func TestQuantileFromMerged_DefensiveBranches(t *testing.T) {
	b := DefaultBoundaries[:]
	// q > 1 makes target exceed total -> clamped down to total.
	if q := quantileFromMerged(b, []int{0, 0, 5}, 5, 1.5); q <= 0 {
		t.Errorf("q>1 quantile=%v, want > 0 (target clamped to total)", q)
	}
	// Inconsistent input (merged sums to 2 < total 5): the prefix sum never
	// reaches target, so the post-loop fallback returns the last boundary.
	if fb := quantileFromMerged(b, []int{2}, 5, 0.99); fb != b[len(b)-1] {
		t.Errorf("fallback=%v want last boundary %v", fb, b[len(b)-1])
	}
}

// TestAutoShardCount_LowGOMAXPROCS covers the n<2 clamp by forcing a low
// GOMAXPROCS (saved and restored). On a >=4-core machine the clamp is otherwise
// never taken.
func TestAutoShardCount_LowGOMAXPROCS(t *testing.T) {
	prev := runtime.GOMAXPROCS(2) // 2/2 = 1 -> clamped to 2
	defer runtime.GOMAXPROCS(prev)
	if got := AutoShardCount(); got != 2 {
		t.Errorf("AutoShardCount with GOMAXPROCS=2 = %d, want 2 (clamped)", got)
	}
}

// TestShardHistogram_QuantileClamp covers the q<0 / q>1 clamps on the sharded
// Quantile path.
func TestShardHistogram_QuantileClamp(t *testing.T) {
	s := NewShardHistogram(4, Options{})
	s.Observe(time.Millisecond)
	if qLo, q0 := s.Quantile(-1), s.Quantile(0); qLo != q0 {
		t.Errorf("q<0 not clamped to 0: %v != %v", qLo, q0)
	}
	if qHi, q1 := s.Quantile(2), s.Quantile(1); qHi != q1 {
		t.Errorf("q>1 not clamped to 1: %v != %v", qHi, q1)
	}
}

// TestShardHistogram_EmptySnapshot covers the total==0 early return on the
// sharded Snapshot (and an empty Quantile fold).
func TestShardHistogram_EmptySnapshot(t *testing.T) {
	s := NewShardHistogram(4, Options{})
	st := s.Snapshot()
	if st.Count != 0 || st.P99 != 0 || st.Mean != 0 {
		t.Fatalf("empty shard snapshot = %+v, want zeros", st)
	}
	if q := s.Quantile(0.5); q != 0 {
		t.Errorf("empty shard Quantile=%v want 0", q)
	}
}

// TestShardHistogram_AllZeroSamples covers the `sum > 0` false branch: when
// every sample is 0, the window sum is 0 and Mean stays 0.
func TestShardHistogram_AllZeroSamples(t *testing.T) {
	s := NewShardHistogram(4, Options{})
	for range 10 {
		s.Observe(0)
	}
	st := s.Snapshot()
	if st.Count != 10 {
		t.Fatalf("Count=%d want 10", st.Count)
	}
	if st.Mean != 0 {
		t.Errorf("Mean=%v want 0 (all samples were 0)", st.Mean)
	}
	if st.Min != 0 || st.Max != 0 {
		t.Errorf("Min=%v Max=%v want 0/0", st.Min, st.Max)
	}
}
