package latency

import (
	"runtime"
	"sync/atomic"
	"time"
)

// AutoShardCount returns the default shard count for [NewShardHistogram] when
// the caller does not specify one: max(2, GOMAXPROCS/2). This mirrors log4go's
// ShardLogger sizing — enough shards to keep per-shard lock contention low on
// high-core machines without over-allocating.
func AutoShardCount() int {
	n := runtime.GOMAXPROCS(0) / 2
	if n < 2 {
		n = 2
	}
	return n
}

// ShardHistogram is a sharded [Histogram] for very high write concurrency
// (millions of observations/sec on a single instance). Writes are distributed
// round-robin across N independent histograms, so write contention is divided
// by N. Reads (Quantile/Snapshot) merge all shards, so they are O(N × window ×
// buckets) — more expensive than a single Histogram, but reads are rare (a
// Prometheus scrape every ~15s).
//
// All methods are safe for concurrent use.
type ShardHistogram struct {
	shards []*Histogram
	n      uint64
	rr     atomic.Uint64 // round-robin write cursor
}

// NewShardHistogram constructs a sharded histogram with n shards (n <= 0
// selects [AutoShardCount]) sharing the same options. It returns nil if the
// options are invalid (same rules as [NewHistogram]).
func NewShardHistogram(n int, opts Options) *ShardHistogram {
	if n <= 0 {
		n = AutoShardCount()
	}
	sh := &ShardHistogram{
		shards: make([]*Histogram, n),
		n:      uint64(n),
	}
	for i := range sh.shards {
		h := NewHistogram(opts)
		if h == nil {
			return nil
		}
		sh.shards[i] = h
	}
	return sh
}

// Observe records a sample on the next shard (round-robin). From the caller's
// perspective it is near-lock-free: an atomic add selects a shard, and only
// that one shard's mutex is taken.
func (s *ShardHistogram) Observe(d time.Duration) {
	idx := s.rr.Add(1) % s.n
	s.shards[idx].Observe(d)
}

// Quantile returns the q-quantile over all shards' merged windows.
func (s *ShardHistogram) Quantile(q float64) time.Duration {
	if q < 0 {
		q = 0
	}
	if q > 1 {
		q = 1
	}
	bounds := s.shards[0].boundaries
	merged := make([]int, len(bounds))
	total, _, _, _, _ := s.foldAll(merged)
	return quantileFromMerged(bounds, merged, total, q)
}

// Snapshot returns the merged window summary across all shards.
func (s *ShardHistogram) Snapshot() Stats {
	bounds := s.shards[0].boundaries
	merged := make([]int, len(bounds))
	total, sum, mn, mx, hasMin := s.foldAll(merged)
	st := Stats{Count: uint64(total)}
	if total == 0 {
		return st
	}
	if hasMin {
		st.Min = time.Duration(mn)
	}
	st.Max = time.Duration(mx)
	if sum > 0 {
		st.Mean = time.Duration(sum / uint64(total))
	}
	st.P50 = quantileFromMerged(bounds, merged, total, 0.50)
	st.P90 = quantileFromMerged(bounds, merged, total, 0.90)
	st.P99 = quantileFromMerged(bounds, merged, total, 0.99)
	st.P999 = quantileFromMerged(bounds, merged, total, 0.999)
	return st
}

// foldAll advances every shard to the current second and accumulates each
// shard's folded window into merged (which must be zeroed by the caller and be
// of length >= numBuckets). Each shard is locked only for the duration of its
// own fold, so no two shard locks are ever held at once.
func (s *ShardHistogram) foldAll(merged []int) (total int, sum uint64, min, max uint64, hasMin bool) {
	sec := time.Now().Unix()
	for _, sh := range s.shards {
		sh.mu.Lock()
		t, su, mn, mx, hm := sh.foldLocked(sec, merged)
		sh.mu.Unlock()
		total += t
		sum += su
		if hm && (!hasMin || mn < min) {
			min = mn
			hasMin = true
		}
		if mx > max {
			max = mx
		}
	}
	return total, sum, min, max, hasMin
}
