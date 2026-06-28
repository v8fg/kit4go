package latency

import (
	"math"
	"sync"
	"time"
)

// windowBucket is one per-second slot of the sliding ring. counts holds the
// per-bucket observation counts; total is sum(counts), cached so advance can
// expire a whole bucket in O(1) and a reader can short-circuit on an empty
// window. min/max/sum mirror those totals so a Snapshot is fully window-scoped
// without any lifetime state.
type windowBucket struct {
	counts []int
	total  int
	min    uint64 // ns; meaningful only when total > 0
	max    uint64 // ns
	sum    uint64 // ns, for the window mean
}

// Stats is a point-in-time summary of a histogram's trailing window. Every
// field is window-scoped: a Snapshot reflects only samples observed within the
// last [Options.Window], so it tracks current latency rather than lifetime.
type Stats struct {
	// Count is the number of samples currently inside the trailing window.
	Count uint64

	// Min and Max are the smallest and largest sample in the window.
	Min time.Duration
	Max time.Duration

	// Mean is the arithmetic mean of the window's samples.
	Mean time.Duration

	// P50/P90/P99/P999 are the median, 90th, 99th and 99.9th percentile
	// latencies, linearly interpolated within their bucket.
	P50  time.Duration
	P90  time.Duration
	P99  time.Duration
	P999 time.Duration
}

// Histogram is a fixed-bucket latency histogram with a trailing sliding
// window. It reports percentiles (p50/p99/p999) over the samples that landed
// within the last [Options.Window]; older samples are expired lazily on the
// next Observe or read.
//
// Observe is the hot path: a binary bucket lookup (lock-free, the boundaries
// are read-only) plus a short mutex critical section (advance + increment).
// It is allocation-free, matching the shape of limiter's sliding window.
//
// Within-bucket percentile values are linearly interpolated between the
// bucket's lower and upper bounds — a conservative estimate that tends to
// slightly over-state the tail (the same assumption HdrHistogram makes).
//
// All methods are safe for concurrent use by multiple goroutines.
type Histogram struct {
	boundaries []time.Duration // frozen upper bounds; read-only after New
	numBuckets int
	windowSec  int

	buckets []windowBucket // ring, len == windowSec
	base    int64          // unix second of the newest bucket advanced to

	mu       sync.Mutex // guards buckets/base/mergeBuf
	mergeBuf []int      // reused by Quantile/Snapshot -> 0 alloc
}

// NewHistogram constructs a [Histogram] from opts, filling zero fields with
// defaults. It returns nil if opts.Boundaries is non-empty but invalid
// (non-monotonic, or containing a value <= 0) — a configuration error the
// caller should treat as fatal. A nil/empty Boundaries selects
// [DefaultBoundaries].
func NewHistogram(opts Options) *Histogram {
	opts = opts.withDefaults()
	if !validBoundaries(opts.Boundaries) {
		return nil
	}
	// withDefaults (called above) guarantees Window >= 1s, so secs >= 1.
	secs := int(opts.Window / time.Second)
	n := len(opts.Boundaries)
	boundaries := make([]time.Duration, n)
	copy(boundaries, opts.Boundaries)

	h := &Histogram{
		boundaries: boundaries,
		numBuckets: n,
		windowSec:  secs,
		buckets:    make([]windowBucket, secs),
		base:       time.Now().Unix(),
		mergeBuf:   make([]int, n),
	}
	for i := range h.buckets {
		h.buckets[i].counts = make([]int, n)
	}
	return h
}

// Observe records a single latency sample. d <= 0 is recorded as 0 (bucket 0)
// so the count stays accurate. Allocation-free on the hot path.
func (h *Histogram) Observe(d time.Duration) {
	if d < 0 {
		d = 0
	}
	ns := uint64(d)
	idx := bucketIndex(h.boundaries, d) // lock-free; boundaries is read-only

	h.mu.Lock()
	// Read the second UNDER the lock. A value read before acquiring the lock can
	// be older than base (advanced by a concurrent caller while we waited), which
	// would otherwise trip advance's backward path and destroy live samples.
	sec := time.Now().Unix()
	h.advance(sec)
	if sec < h.base {
		sec = h.base // wall clock regressed (NTP): attribute the sample to the current bucket
	}
	b := &h.buckets[int(sec%int64(h.windowSec))]
	if b.total == 0 {
		// First sample in this (possibly just-cleared) bucket: seed min/max so
		// a genuine 0 sample is not confused with the unset state.
		b.min = ns
		b.max = ns
	} else {
		if ns < b.min {
			b.min = ns
		}
		if ns > b.max {
			b.max = ns
		}
	}
	b.counts[idx]++
	b.total++
	b.sum += ns
	h.mu.Unlock()
}

// Quantile returns the q-quantile latency (0 <= q <= 1) over the trailing
// window. q <= 0 returns the minimum, q >= 1 the maximum. Returns 0 when no
// samples are in the window.
func (h *Histogram) Quantile(q float64) time.Duration {
	if q < 0 {
		q = 0
	}
	if q > 1 {
		q = 1
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	merged := h.mergeBuf
	for j := range merged {
		merged[j] = 0
	}
	total, _, _, _, _ := h.foldLocked(time.Now().Unix(), merged)
	return quantileFromMerged(h.boundaries, merged, total, q)
}

// Snapshot returns a point-in-time summary over the trailing window. Every
// field is window-scoped (see [Stats]); the returned struct is a value, safe
// to keep.
func (h *Histogram) Snapshot() Stats {
	h.mu.Lock()
	defer h.mu.Unlock()
	merged := h.mergeBuf
	for j := range merged {
		merged[j] = 0
	}
	total, sum, mn, mx, hasMin := h.foldLocked(time.Now().Unix(), merged)
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
	st.P50 = quantileFromMerged(h.boundaries, merged, total, 0.50)
	st.P90 = quantileFromMerged(h.boundaries, merged, total, 0.90)
	st.P99 = quantileFromMerged(h.boundaries, merged, total, 0.99)
	st.P999 = quantileFromMerged(h.boundaries, merged, total, 0.999)
	return st
}

// foldLocked advances the window to sec, then folds every live bucket into out
// (a caller-provided buffer of length >= numBuckets), returning the window
// total, sum, min and max. Caller must hold h.mu. out is mutated in place.
func (h *Histogram) foldLocked(sec int64, out []int) (total int, sum uint64, min, max uint64, hasMin bool) {
	h.advance(sec)
	for i := range h.buckets {
		b := &h.buckets[i]
		if b.total > 0 {
			if !hasMin || b.min < min {
				min = b.min
				hasMin = true
			}
			if b.max > max {
				max = b.max
			}
		}
		total += b.total
		sum += b.sum
		for j := 0; j < h.numBuckets; j++ {
			out[j] += b.counts[j]
		}
	}
	return total, sum, min, max, hasMin
}

// advance rolls the bucket ring forward to sec, zeroing buckets that have
// fallen out of the window. After it returns, the bucket for sec is cleared
// and ready for a fresh count. Mirrors limiter/sliding_window.go advance.
func (h *Histogram) advance(sec int64) {
	n := int64(h.windowSec)
	if sec <= h.base {
		// Stale read or wall-clock regression (NTP). Do NOT clear: destroying
		// live data on a backward timestamp would silently drop samples. The
		// caller (Observe) clamps its write bucket to base, so leave the window
		// untouched here.
		return
	}
	if sec-h.base >= n { // a full window (or more) elapsed: every bucket expired
		for i := range h.buckets {
			h.zeroBucket(i)
		}
		h.base = sec
		return
	}
	for h.base < sec { // roll forward one bucket at a time
		h.base++
		h.zeroBucket(int(h.base % n))
	}
}

// zeroBucket clears bucket i in place. Counts are zeroed element-wise (we must
// not swap in a shared zero slice: a later counts[idx]++ would mutate the
// shared backing array and corrupt every bucket).
func (h *Histogram) zeroBucket(i int) {
	b := &h.buckets[i]
	for j := range b.counts {
		b.counts[j] = 0
	}
	b.total = 0
	b.min = 0
	b.max = 0
	b.sum = 0
}

// quantileFromMerged computes the q-quantile from a folded count array via a
// prefix sum and within-bucket linear interpolation. Pure function (no
// receiver state) so Snapshot can reuse one merged array for four quantiles.
func quantileFromMerged(boundaries []time.Duration, merged []int, total int, q float64) time.Duration {
	if total == 0 {
		return 0
	}
	target := int(math.Ceil(q * float64(total)))
	if target < 1 {
		target = 1
	}
	if target > total {
		target = total
	}
	running := 0
	for i, c := range merged {
		running += c
		if c > 0 && running >= target {
			// The target-th sample is the (target-(running-c))-th sample in
			// bucket i (1-based). Interpolate across [lo, hi] at its midpoint
			// position — the least-biased estimate of where within the bucket
			// the sample sits.
			lo := lowerBound(boundaries, i)
			hi := boundaries[i]
			inBucket := target - (running - c)
			// inBucket is in [1, c] (the target-th sample is somewhere in this
			// bucket), so frac lands in (0, 1) — interpolate at its midpoint.
			frac := (float64(inBucket) - 0.5) / float64(c)
			return lo + time.Duration(float64(hi-lo)*frac)
		}
	}
	return boundaries[len(boundaries)-1]
}
