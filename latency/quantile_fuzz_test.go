package latency_test

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/v8fg/kit4go/latency"
)

// FuzzQuantileMonotonic verifies Quantile is non-decreasing in q for any stream
// of non-negative samples. A regression in the bucket interpolation — off-by-one
// in the running cumulative, a frac landing outside (0,1), or non-monotonic
// bounds — would violate monotonicity, the load-bearing property for percentile
// reporting (p50 must never exceed p99). The stream is packed as 8-byte
// little-endian uint64 samples (Go fuzzing allows []byte, not []int64).
func FuzzQuantileMonotonic(f *testing.F) {
	f.Add([]byte{})                                 // empty stream
	f.Add(binary.LittleEndian.AppendUint64(nil, 0)) // single zero
	f.Add(append(
		binary.LittleEndian.AppendUint64(nil, 100),
		binary.LittleEndian.AppendUint64(nil, 9_000_000_000)...)) // spread: tiny + huge

	f.Fuzz(func(t *testing.T, data []byte) {
		h := latency.NewHistogram(latency.Options{})
		if h == nil {
			t.Fatal("NewHistogram returned nil for default options")
		}
		for i := 0; i+8 <= len(data) && i < 8000; i += 8 {
			s := int64(binary.LittleEndian.Uint64(data[i : i+8]))
			if s < 0 {
				s = -s // Observe clamps negatives to 0; mirror non-negative here
			}
			h.Observe(time.Duration(s))
		}

		// Quantile must be non-decreasing across q in [0,1].
		prev := h.Quantile(0)
		for q := 0.0; q <= 1.0+1e-9; q += 0.01 {
			cur := h.Quantile(q)
			if cur < prev {
				t.Fatalf("Quantile not monotonic: q=%.2f -> %v < prev %v", q, cur, prev)
			}
			prev = cur
		}
		if h.Quantile(0) > h.Quantile(1) {
			t.Fatalf("Quantile(0) > Quantile(1): %v > %v", h.Quantile(0), h.Quantile(1))
		}
	})
}
