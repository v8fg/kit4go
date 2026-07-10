package limiter

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// slidingWindow is a sliding-window counter with one bucket per second — the
// same design as log4go's RateAlerter. At most rate requests may land inside the
// trailing windowSec seconds.
//
// A fixed ring of per-second counters (counts, len == windowSec) is advanced
// lazily on each call: advance() rolls the ring forward and subtracts expired
// buckets from the running sum, so Allow is O(1) amortised with no per-event
// allocation. Only the sum comparison needs the mutex; the metrics counters are
// kept on atomics so Metrics() stays lock-free.
type slidingWindow struct {
	rate      float64 // max requests per window
	windowSec int     // window size in seconds (len(counts))

	counts []int // ring of per-second counters
	base   int64 // unix second of the newest bucket advanced to
	sum    int   // running sum of live buckets

	mu       sync.Mutex
	allowed  atomic.Uint64
	denied   atomic.Uint64
	acquired atomic.Uint64
	closed   atomic.Bool

	// now is the clock source. It defaults to [time.Now] so production reads
	// wall time; tests inject a fake clock to advance time deterministically
	// instead of sleeping. nil-safe via the nowTime method.
	now func() time.Time
}

// nowTime returns the current clock reading, falling back to [time.Now] when no
// fake clock has been injected.
func (sw *slidingWindow) nowTime() time.Time {
	return sw.now()
}

func newSlidingWindow(rate float64, window time.Duration) *slidingWindow {
	secs := int(window.Seconds())
	if secs < 1 {
		secs = 1
	}
	sw := &slidingWindow{
		rate:      rate,
		windowSec: secs,
		counts:    make([]int, secs),
		now:       time.Now,
	}
	sw.base = sw.nowTime().Unix()
	return sw
}

// Allow records one event and returns true if it fits under the cap.
func (sw *slidingWindow) Allow() bool {
	if sw.closed.Load() {
		return false
	}
	return sw.acquire(1)
}

// TryAcquire records n events atomically (no partial acquisition). n <= 0 is a
// no-op success.
func (sw *slidingWindow) TryAcquire(n int) bool {
	if n <= 0 {
		return true
	}
	if sw.closed.Load() {
		return false
	}
	return sw.acquire(n)
}

func (sw *slidingWindow) acquire(n int) bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	// Read the second UNDER the lock: a value read before acquiring the lock can
	// be older than base (advanced by a concurrent caller while we waited), which
	// would otherwise trip advance's backward path and silently destroy live
	// counts — letting the limiter over-allow.
	sec := sw.nowTime().Unix()
	sw.advance(sec)
	if sec < sw.base {
		sec = sw.base // wall clock regressed: charge the current bucket
	}
	if float64(sw.sum)+float64(n) <= sw.rate {
		sw.counts[int(sec%int64(sw.windowSec))] += n
		sw.sum += n
		sw.allowed.Add(1)
		sw.acquired.Add(uint64(n))
		return true
	}
	sw.denied.Add(1)
	return false
}

// Wait blocks until one token is acquired or ctx is cancelled. It polls with a
// short sleep (sized to the remaining window tail) since the sliding window can
// only free capacity as the oldest second expires. After Close it returns
// promptly (ctx.Err() if ctx is done, else ErrLimiterClosed) rather than
// busy-looping.
func (sw *slidingWindow) Wait(ctx context.Context) error {
	// Closed short-circuit: match Allow/TryAcquire (see tokenBucket.Wait).
	if sw.closed.Load() {
		return closedWaitResult(ctx)
	}
	if sw.Allow() {
		return nil
	}
	for {
		// Re-check each iteration: a Close issued WHILE Wait is blocked at
		// capacity must unblock it within one poll, not poll until ctx expires.
		if sw.closed.Load() {
			return closedWaitResult(ctx)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if sw.Allow() {
			return nil
		}
	}
}

// advance rolls the bucket ring forward to sec, zeroing buckets that have fallen
// out of the window and subtracting them from sum. After it returns, the bucket
// for sec is cleared and ready for a fresh count. Mirrors RateAlerter.advance.
func (sw *slidingWindow) advance(sec int64) {
	n := int64(sw.windowSec)
	if sec <= sw.base {
		// Stale read or wall-clock regression (NTP). Do NOT clear: destroying
		// live counts on a backward timestamp would let the limiter over-allow.
		// The caller clamps its write to base, so leave the window untouched.
		return
	}
	if sec-sw.base >= n {
		// A full window (or more) has elapsed: every bucket is expired.
		for i := range sw.counts {
			sw.sum -= sw.counts[i]
			sw.counts[i] = 0
		}
		sw.base = sec
		return
	}
	for sw.base < sec {
		sw.base++
		i := int(sw.base % n)
		sw.sum -= sw.counts[i]
		sw.counts[i] = 0
	}
}

// Close marks the limiter as closed. Idempotent.
func (sw *slidingWindow) Close() {
	sw.closed.Store(true)
}

// Metrics returns a best-effort snapshot of the counters.
func (sw *slidingWindow) Metrics() LimiterMetrics {
	return LimiterMetrics{
		Allowed:  sw.allowed.Load(),
		Denied:   sw.denied.Load(),
		Acquired: sw.acquired.Load(),
	}
}
