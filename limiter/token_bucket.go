package limiter

import (
	"context"
	"math"
	"sync/atomic"
	"time"
)

// tokenBucket is a classic token bucket with lazy (on-demand) refill.
//
// Tokens accrue continuously at rate per second, capped at burst. The token
// count and last-refill timestamp are stored as atomics so the hot path (Allow)
// is lock-free: it reads both, computes the refill, and CASes the new count. On
// CAS contention it retries once, then conservatively denies — favouring the
// rate cap over throughput under heavy contention.
//
// The float64 token count is stored in an atomic.Uint64 via
// [math.Float64bits]/[math.Float64frombits], because sync/atomic has no native
// float64 type.
type tokenBucket struct {
	rate  float64 // tokens per second
	burst float64 // capacity (max tokens)

	tokens   atomic.Uint64 // math.Float64bits(float64)
	lastTime atomic.Int64  // unix nano of last refill point
	allowed  atomic.Uint64
	denied   atomic.Uint64
	acquired atomic.Uint64
	closed   atomic.Bool

	// now is the clock source. It defaults to [time.Now] so production reads
	// wall time; tests inject a fake clock to advance time deterministically
	// instead of sleeping. nil-safe via the now method.
	now func() time.Time
}

// nowTime returns the current clock reading, falling back to [time.Now] when no
// fake clock has been injected. Kept inline (no allocation) on the hot path.
func (tb *tokenBucket) nowTime() time.Time {
	return tb.now()
}

func newTokenBucket(rate float64, burst int) *tokenBucket {
	if burst < 1 {
		burst = 1
	}
	tb := &tokenBucket{
		rate:  rate,
		burst: float64(burst),
		now:   time.Now,
	}
	// Start full so the very first burst is absorbable.
	tb.tokens.Store(math.Float64bits(float64(burst)))
	tb.lastTime.Store(tb.nowTime().UnixNano())
	return tb
}

// Allow attempts to acquire one token without blocking. Lock-free; on CAS
// contention it retries once, then denies.
func (tb *tokenBucket) Allow() bool {
	if tb.closed.Load() {
		return false
	}
	return tb.acquire(1)
}

// TryAcquire attempts to acquire n tokens at once. n <= 0 is a no-op success.
// On contention it retries once, then denies (no partial acquisition).
func (tb *tokenBucket) TryAcquire(n int) bool {
	if n <= 0 {
		return true
	}
	if tb.closed.Load() {
		return false
	}
	return tb.acquire(n)
}

// acquire is the shared CAS loop for Allow and TryAcquire. It retries once on
// CAS failure (concurrent mutation) and then returns false, so under heavy
// contention the limiter stays faithful to its configured cap.
func (tb *tokenBucket) acquire(n int) bool {
	now := tb.nowTime().UnixNano()
	last := tb.lastTime.Load()
	if now < last {
		// clock moved backward: don't refill with a negative delta, just probe
		// the current bucket.
		now = last
	}

	for attempt := 0; attempt < 2; attempt++ {
		curBits := tb.tokens.Load()
		cur := math.Float64frombits(curBits)
		refill := float64(now-last) / 1e9 * tb.rate
		if refill < 0 {
			refill = 0
		}
		avail := cur + refill
		if avail > tb.burst {
			avail = tb.burst
		}
		if avail < float64(n) {
			// Not enough tokens even after refill. Do not update lastTime — the
			// unrefilled delta carries forward to the next call (lazy refill).
			tb.denied.Add(1)
			return false
		}
		next := avail - float64(n)
		// Advance lastTime only when we actually consumed tokens. CAS both the
		// token count and the timestamp; if either lost a race, retry once.
		if tb.tokens.CompareAndSwap(curBits, math.Float64bits(next)) {
			tb.lastTime.CompareAndSwap(last, now)
			tb.allowed.Add(1)
			tb.acquired.Add(uint64(n))
			return true
		}
		// CAS failed: reload and retry once.
		last = tb.lastTime.Load()
	}
	// Lost the race twice: conservatively deny to protect the cap.
	tb.denied.Add(1)
	return false
}

// Wait blocks until one token is acquired or ctx is cancelled. It polls Allow
// with a short sleep sized to the current deficit so a near-available token is
// grabbed promptly without busy-spinning. After Close it returns promptly
// (ctx.Err() if ctx is done, else ErrLimiterClosed) rather than busy-looping.
func (tb *tokenBucket) Wait(ctx context.Context) error {
	// Closed short-circuit: match Allow/TryAcquire, which already bail out on
	// close. Without this, Wait would spin on 1ms timers (Allow keeps returning
	// false) until ctx expires — violating the "no-op after Close" contract.
	if tb.closed.Load() {
		return closedWaitResult(ctx)
	}
	// Fast path: try once before setting up the timer.
	if tb.Allow() {
		return nil
	}
	for {
		// Re-check each iteration: a Close issued WHILE Wait is blocked at
		// capacity must unblock it within one poll, not poll until ctx expires.
		if tb.closed.Load() {
			return closedWaitResult(ctx)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		// Estimate how long until one token is available, capped at 1ms to keep
		// the loop responsive under clock jitter / concurrent acquirers.
		wait := tb.nextAvailableDelay()
		if wait <= 0 {
			wait = time.Millisecond
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if tb.Allow() {
			return nil
		}
	}
}

// nextAvailableDelay estimates how long until one token refills. Returns 0 if a
// token is (nominally) already available.
func (tb *tokenBucket) nextAvailableDelay() time.Duration {
	now := tb.nowTime().UnixNano()
	last := tb.lastTime.Load()
	if now < last {
		now = last
	}
	cur := math.Float64frombits(tb.tokens.Load())
	refill := float64(now-last) / 1e9 * tb.rate
	if refill < 0 {
		refill = 0
	}
	avail := cur + refill
	if avail > tb.burst {
		avail = tb.burst
	}
	if avail >= 1 {
		return 0
	}
	deficit := 1 - avail
	secs := deficit / tb.rate
	d := time.Duration(secs * float64(time.Second))
	if d < time.Microsecond {
		d = time.Microsecond
	}
	if d > time.Millisecond {
		d = time.Millisecond
	}
	return d
}

// Close marks the limiter as closed. Idempotent.
func (tb *tokenBucket) Close() {
	tb.closed.Store(true)
}

// Metrics returns a best-effort snapshot of the counters.
func (tb *tokenBucket) Metrics() LimiterMetrics {
	return LimiterMetrics{
		Allowed:  tb.allowed.Load(),
		Denied:   tb.denied.Load(),
		Acquired: tb.acquired.Load(),
	}
}
