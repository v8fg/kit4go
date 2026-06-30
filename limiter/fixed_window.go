package limiter

import (
	"context"
	"sync/atomic"
	"time"
)

// fixedWindow allows at most `rate` requests per `window`, resetting the counter
// at each window boundary. It is the simplest algorithm: O(1) lock-free, but
// allows bursts at window edges (up to 2× rate across a boundary).
type fixedWindow struct {
	rate        int64 // max requests per window
	windowNs    int64 // window duration in nanoseconds
	count       atomic.Int64
	windowStart atomic.Int64 // unix nano of current window start
	allowed     atomic.Uint64
	denied      atomic.Uint64
	acquired    atomic.Uint64
	closed      atomic.Bool
}

func newFixedWindow(rate int64, window time.Duration) *fixedWindow {
	if rate < 1 {
		rate = 1
	}
	if window < time.Second {
		window = time.Second
	}
	now := time.Now().UnixNano()
	fw := &fixedWindow{rate: rate, windowNs: int64(window)}
	fw.windowStart.Store(now)
	return fw
}

func (fw *fixedWindow) Allow() bool {
	if fw.closed.Load() {
		return false
	}
	return fw.acquire(1)
}

func (fw *fixedWindow) TryAcquire(n int) bool {
	if n <= 0 {
		return true
	}
	if fw.closed.Load() {
		return false
	}
	return fw.acquire(n)
}

func (fw *fixedWindow) acquire(n int) bool {
	now := time.Now().UnixNano()
	for attempt := 0; attempt < 2; attempt++ {
		start := fw.windowStart.Load()
		// Check if the window has expired.
		if now >= start+fw.windowNs {
			// Try to advance the window. If CAS succeeds, we "own" the reset.
			if fw.windowStart.CompareAndSwap(start, now) {
				fw.count.Store(0)
			} else {
				continue // someone else advanced; reload and retry
			}
		}
		cur := fw.count.Load()
		if cur+int64(n) > fw.rate {
			fw.denied.Add(1)
			return false
		}
		if fw.count.CompareAndSwap(cur, cur+int64(n)) {
			fw.allowed.Add(1)
			fw.acquired.Add(uint64(n))
			return true
		}
		// CAS lost: retry once.
	}
	fw.denied.Add(1)
	return false
}

func (fw *fixedWindow) Wait(ctx context.Context) error {
	if fw.Allow() {
		return nil
	}
	for {
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
		if fw.Allow() {
			return nil
		}
	}
}

func (fw *fixedWindow) Close() { fw.closed.Store(true) }
func (fw *fixedWindow) Metrics() LimiterMetrics {
	return LimiterMetrics{
		Allowed:  fw.allowed.Load(),
		Denied:   fw.denied.Load(),
		Acquired: fw.acquired.Load(),
	}
}
