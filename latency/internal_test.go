// This file is an internal test (package latency, not latency_test) so it can
// reach the unexported helpers — advance, zeroBucket, bucketIndex, lowerBound,
// foldLocked — and exercise the sliding-window ring directly. It mirrors the
// advance-branch tests in limiter/bench_test.go.
package latency

import (
	"testing"
	"time"
)

// TestBucketIndex covers the binary bucket lookup: values below/at the first
// boundary, between boundaries, exactly on a boundary, the overflow case, and
// negative inputs (Observe clamps those to 0 before lookup, but the helper
// itself must not panic).
func TestBucketIndex(t *testing.T) {
	b := DefaultBoundaries[:]
	last := len(b) - 1
	cases := []struct {
		v   time.Duration
		exp int
	}{
		{0, 0},
		{50 * time.Microsecond, 0},  // below boundaries[0] (100µs)
		{100 * time.Microsecond, 0}, // == boundaries[0]
		{101 * time.Microsecond, 1},
		{500 * time.Microsecond, 1}, // == boundaries[1]
		{time.Millisecond, 2},
		{5 * time.Millisecond, 5},    // == boundaries[5]
		{49 * time.Millisecond, 10},  // (30ms, 50ms]
		{50 * time.Millisecond, 10},  // == boundaries[10]
		{time.Hour, last},            // overflow -> last bucket
		{-time.Millisecond, 0},       // negative -> bucket 0
	}
	for _, c := range cases {
		if got := bucketIndex(b, c.v); got != c.exp {
			t.Errorf("bucketIndex(%v) = %d, want %d", c.v, got, c.exp)
		}
	}
}

func TestLowerBound(t *testing.T) {
	b := DefaultBoundaries[:]
	if got := lowerBound(b, 0); got != 0 {
		t.Errorf("lowerBound(0) = %v, want 0", got)
	}
	if got := lowerBound(b, 5); got != b[4] {
		t.Errorf("lowerBound(5) = %v, want %v", got, b[4])
	}
}

func TestValidBoundaries(t *testing.T) {
	if !validBoundaries(DefaultBoundaries[:]) {
		t.Error("DefaultBoundaries should be valid")
	}
	for _, bad := range [][]time.Duration{
		nil,
		{},
		{time.Millisecond, 0},                                  // contains <=0
		{-time.Millisecond, time.Millisecond},                  // negative
		{2 * time.Millisecond, time.Millisecond},               // decreasing
		{time.Millisecond, time.Millisecond},                   // equal (not strict)
	} {
		if validBoundaries(bad) {
			t.Errorf("validBoundaries(%v) = true, want false", bad)
		}
	}
}

// fillBucket seeds window bucket i with c samples of value v (placed in the
// latency bucket for v), so tests can populate the ring and observe expiry
// without going through the public Observe path (which would advance the
// window itself).
func fillBucket(h *Histogram, i, c int, v time.Duration) {
	b := &h.buckets[i]
	ns := uint64(v)
	idx := bucketIndex(h.boundaries, v)
	b.counts[idx] = c
	b.total = c
	b.sum = ns * uint64(c)
	b.min = ns
	b.max = ns
}

func TestHistogram_Advance_SameSecond(t *testing.T) {
	h := NewHistogram(Options{Window: 5 * time.Second})
	h.mu.Lock()
	h.base = 1000
	fillBucket(h, 0, 7, time.Millisecond)
	h.advance(1000) // same second -> no-op
	if h.buckets[0].total != 7 {
		t.Fatalf("same-second advance cleared the bucket: total=%d", h.buckets[0].total)
	}
	if h.base != 1000 {
		t.Fatalf("base=%d want 1000 (unchanged)", h.base)
	}
	h.mu.Unlock()
}

func TestHistogram_Advance_BackwardClock(t *testing.T) {
	h := NewHistogram(Options{Window: 5 * time.Second})
	h.mu.Lock()
	h.base = 1000
	for i := range h.buckets {
		fillBucket(h, i, 7, time.Millisecond)
	}
	h.advance(997) // 997 < 1000 -> clear only the target slot
	idx := int(int64(997) % int64(h.windowSec))
	if h.buckets[idx].total != 0 {
		t.Fatalf("backward advance did not clear slot %d: total=%d", idx, h.buckets[idx].total)
	}
	if h.base != 997 {
		t.Fatalf("base=%d want 997", h.base)
	}
	// Other buckets are untouched.
	untouched := 0
	for i := range h.buckets {
		if h.buckets[i].total == 7 {
			untouched++
		}
	}
	if untouched != h.windowSec-1 {
		t.Fatalf("untouched buckets=%d, want %d", untouched, h.windowSec-1)
	}
	h.mu.Unlock()
}

func TestHistogram_Advance_ForwardOne(t *testing.T) {
	h := NewHistogram(Options{Window: 5 * time.Second})
	h.mu.Lock()
	h.base = 1000
	for i := range h.buckets {
		fillBucket(h, i, 3, time.Millisecond)
	}
	h.advance(1002) // roll forward 2 buckets
	// Buckets for seconds 1001 and 1002 are cleared; 1000/1003/1004 keep data.
	for _, sec := range []int64{1001, 1002} {
		idx := int(sec % int64(h.windowSec))
		if h.buckets[idx].total != 0 {
			t.Fatalf("forward advance did not clear second %d (idx %d): total=%d", sec, idx, h.buckets[idx].total)
		}
	}
	if h.base != 1002 {
		t.Fatalf("base=%d want 1002", h.base)
	}
	h.mu.Unlock()
}

func TestHistogram_Advance_FullWindowExpiry(t *testing.T) {
	h := NewHistogram(Options{Window: 3 * time.Second})
	h.mu.Lock()
	h.base = 1000
	for i := range h.buckets {
		fillBucket(h, i, 4, time.Millisecond)
	}
	h.advance(1005) // 1005-1000 = 5 >= 3 -> every bucket expired
	for i := range h.buckets {
		if h.buckets[i].total != 0 {
			t.Fatalf("full-window expiry did not clear bucket %d: total=%d", i, h.buckets[i].total)
		}
	}
	if h.base != 1005 {
		t.Fatalf("base=%d want 1005", h.base)
	}
	h.mu.Unlock()
}

// TestHistogram_FoldLocked verifies the window fold produces the right total,
// sum, min and max across a couple of populated buckets.
func TestHistogram_FoldLocked(t *testing.T) {
	h := NewHistogram(Options{Window: 5 * time.Second})
	h.mu.Lock()
	h.base = time.Now().Unix()
	fillBucket(h, 0, 2, 10*time.Millisecond)
	fillBucket(h, 1, 3, 20*time.Millisecond)
	out := make([]int, h.numBuckets)
	total, sum, mn, mx, hasMin := h.foldLocked(h.base, out)
	h.mu.Unlock()
	if total != 5 {
		t.Errorf("total=%d want 5", total)
	}
	if sum != 2*10_000_000+3*20_000_000 {
		t.Errorf("sum=%d want %d", sum, 2*10_000_000+3*20_000_000)
	}
	if !hasMin || mn != 10_000_000 {
		t.Errorf("min=%d hasMin=%v want 10000000", mn, hasMin)
	}
	if mx != 20_000_000 {
		t.Errorf("max=%d want 20000000", mx)
	}
	if out[6] != 2 { // 10ms -> bucket 6
		t.Errorf("merged[6]=%d want 2", out[6])
	}
}
