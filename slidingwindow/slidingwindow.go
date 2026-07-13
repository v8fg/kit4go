// Package slidingwindow provides a generic sliding window that maintains the
// last N elements and supports O(1) aggregate queries (Sum, Min, Max, Avg,
// Count). Elements are evicted FIFO when the window is full.
//
// For time-based eviction (last T duration), use the TimeWindow variant.
//
// Pure standard library. Ad-tech / finance uses: rolling p99 latency, moving
// average price, last-N-impressions click-through rate, rolling volatility.
package slidingwindow

import (
	"math"
	"time"
)

// Window is a fixed-count sliding window over numeric values.
type Window struct {
	buf       []float64
	head      int
	size      int
	cap       int
	sum       float64
	hasMinMax bool
	minVal    float64
	maxVal    float64
}

// New creates a count-based sliding window holding the last `capacity` values.
func New(capacity int) *Window {
	if capacity <= 0 {
		panic("slidingwindow: capacity must be > 0")
	}
	return &Window{
		buf: make([]float64, capacity),
		cap: capacity,
	}
}

// Push adds a value, evicting the oldest if the window is full. Sum is O(1);
// Min/Max are cached and recomputed lazily on query when invalidated by an
// eviction of an extreme value.
func (w *Window) Push(v float64) {
	if w.size < w.cap {
		idx := (w.head + w.size) % w.cap
		w.buf[idx] = v
		w.size++
	} else {
		old := w.buf[w.head]
		w.sum -= old
		w.buf[w.head] = v
		w.head = (w.head + 1) % w.cap
		// Invalidate cached min/max if the evicted element was an extreme.
		if w.hasMinMax && (old == w.minVal || old == w.maxVal) {
			w.hasMinMax = false
		}
	}
	w.sum += v
	if w.hasMinMax {
		if v < w.minVal {
			w.minVal = v
		}
		if v > w.maxVal {
			w.maxVal = v
		}
	}
}

// Sum returns the sum of all values in the window. O(1).
func (w *Window) Sum() float64 { return w.sum }

// Count returns the number of values in the window. O(1).
func (w *Window) Count() int { return w.size }

// Avg returns the average, or NaN if empty. O(1).
func (w *Window) Avg() float64 {
	if w.size == 0 {
		return math.NaN()
	}
	return w.sum / float64(w.size)
}

// Min returns the minimum value. O(n) if the cached min was invalidated by an
// eviction; O(1) otherwise.
func (w *Window) Min() float64 {
	if w.size == 0 {
		return math.NaN()
	}
	w.ensureMinMax()
	return w.minVal
}

// Max returns the maximum value. Same caching behavior as Min.
func (w *Window) Max() float64 {
	if w.size == 0 {
		return math.NaN()
	}
	w.ensureMinMax()
	return w.maxVal
}

func (w *Window) ensureMinMax() {
	if w.hasMinMax {
		return
	}
	w.minVal = w.buf[w.head]
	w.maxVal = w.buf[w.head]
	for i := 1; i < w.size; i++ {
		v := w.buf[(w.head+i)%w.cap]
		if v < w.minVal {
			w.minVal = v
		}
		if v > w.maxVal {
			w.maxVal = v
		}
	}
	w.hasMinMax = true
}

// Len returns the number of elements (alias for Count).
func (w *Window) Len() int { return w.size }

// Cap returns the capacity.
func (w *Window) Cap() int { return w.cap }

// Clear resets the window to empty.
func (w *Window) Clear() {
	w.head = 0
	w.size = 0
	w.sum = 0
	w.hasMinMax = false
}

// --- TimeWindow: time-based eviction ---

// TimeWindow holds values within a time duration. Values older than `ttl` from
// the latest Push are evicted.
type TimeWindow struct {
	ttl  time.Duration
	vals []twEntry
	head int
	size int
	cap  int
	sum  float64
}

type twEntry struct {
	val float64
	ts  time.Time
}

// NewTimeWindow creates a time-based sliding window with the given TTL and a
// max capacity (pre-allocation hint).
func NewTimeWindow(ttl time.Duration, maxCapacity int) *TimeWindow {
	if ttl <= 0 {
		panic("slidingwindow: ttl must be > 0")
	}
	if maxCapacity <= 0 {
		maxCapacity = 1024
	}
	return &TimeWindow{ttl: ttl, vals: make([]twEntry, maxCapacity), cap: maxCapacity}
}

// Push adds a value at time ts, evicting entries older than ttl.
func (w *TimeWindow) Push(v float64, ts time.Time) {
	cutoff := ts.Add(-w.ttl)
	for w.size > 0 {
		idx := w.head
		if w.vals[idx].ts.After(cutoff) {
			break
		}
		w.sum -= w.vals[idx].val
		w.head = (w.head + 1) % w.cap
		w.size--
	}
	if w.size < w.cap {
		idx := (w.head + w.size) % w.cap
		w.vals[idx] = twEntry{val: v, ts: ts}
		w.size++
	} else {
		w.sum -= w.vals[w.head].val
		w.vals[w.head] = twEntry{val: v, ts: ts}
		w.head = (w.head + 1) % w.cap
	}
	w.sum += v
}

// Sum returns the sum of all values in the time window.
func (w *TimeWindow) Sum() float64 { return w.sum }

// Count returns the number of values.
func (w *TimeWindow) Count() int { return w.size }

// Avg returns the average, or NaN if empty.
func (w *TimeWindow) Avg() float64 {
	if w.size == 0 {
		return math.NaN()
	}
	return w.sum / float64(w.size)
}
