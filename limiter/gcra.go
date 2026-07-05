package limiter

import (
	"context"
	"sync/atomic"
	"time"
)

// gcraLimiter is the local (in-process) Generic Cell Rate Algorithm — the same
// algorithm as the distributed GCRA in kit4go/rate, but using a single
// atomic.Int64 instead of a Redis Lua call.
//
// It tracks one value: the "theoretical arrival time" (TAT) — the time at which
// the next token becomes available. On each Allow: TAT is advanced by one
// emission interval (1/rate seconds); if TAT - now exceeds the burst allowance
// (emission × burst), the request is denied. One atomic state variable, lock-free.
type gcraLimiter struct {
	emissionNs    float64 // ns per token = float64(time.Second) / rate
	burstOffsetNs float64 // emissionNs * burst
	tat           atomic.Int64
	allowed       atomic.Uint64
	denied        atomic.Uint64
	acquired      atomic.Uint64
	closed        atomic.Bool

	// now is the clock source. It defaults to [time.Now] so production reads
	// wall time; tests inject a fake clock to advance time deterministically
	// instead of sleeping. nil-safe via the nowTime method.
	now func() time.Time
}

// nowTime returns the current clock reading, falling back to [time.Now] when no
// fake clock has been injected.
func (g *gcraLimiter) nowTime() time.Time {
	return g.now()
}

func newGCRA(rate float64, burst int) *gcraLimiter {
	if burst < 1 {
		burst = 1
	}
	emission := float64(time.Second) / rate
	return &gcraLimiter{
		emissionNs:    emission,
		burstOffsetNs: emission * float64(burst),
		now:           time.Now,
	}
}

func (g *gcraLimiter) Allow() bool {
	if g.closed.Load() {
		return false
	}
	return g.acquire(1)
}

func (g *gcraLimiter) TryAcquire(n int) bool {
	if n <= 0 {
		return true
	}
	if g.closed.Load() {
		return false
	}
	return g.acquire(n)
}

func (g *gcraLimiter) acquire(n int) bool {
	now := g.nowTime().UnixNano()
	cost := float64(n)
	for attempt := 0; attempt < 2; attempt++ {
		loaded := g.tat.Load() // original stored value (for CAS)
		tat := loaded
		if tat < now {
			tat = now
		}
		newTAT := tat + int64(cost*g.emissionNs)
		if (newTAT - now) > int64(g.burstOffsetNs) {
			g.denied.Add(1)
			return false
		}
		if g.tat.CompareAndSwap(loaded, newTAT) {
			g.allowed.Add(1)
			g.acquired.Add(uint64(n))
			return true
		}
		now = g.nowTime().UnixNano()
	}
	g.denied.Add(1)
	return false
}

func (g *gcraLimiter) Wait(ctx context.Context) error {
	if g.Allow() {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		timer := time.NewTimer(g.nextDelay())
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if g.Allow() {
			return nil
		}
	}
}

func (g *gcraLimiter) nextDelay() time.Duration {
	now := g.nowTime().UnixNano()
	tat := g.tat.Load()
	if tat <= now {
		return 0
	}
	deficit := tat - now - int64(g.burstOffsetNs)
	if deficit <= 0 {
		return 0
	}
	d := time.Duration(deficit)
	if d < time.Microsecond {
		d = time.Microsecond
	}
	if d > time.Millisecond {
		d = time.Millisecond
	}
	return d
}

func (g *gcraLimiter) Close() { g.closed.Store(true) }
func (g *gcraLimiter) Metrics() LimiterMetrics {
	return LimiterMetrics{
		Allowed:  g.allowed.Load(),
		Denied:   g.denied.Load(),
		Acquired: g.acquired.Load(),
	}
}
