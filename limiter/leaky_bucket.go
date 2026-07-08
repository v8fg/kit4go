package limiter

import (
	"context"
	"math"
	"sync/atomic"
	"time"
)

// leakyBucket models a bucket that "fills" with each request and drains at a
// steady rate. If adding a request would overflow the capacity (Burst), the
// request is denied. This smooths outflow: spikes fill the bucket but the drain
// rate stays constant.
//
// Implementation is lock-free CAS on the water level + last-drain timestamp,
// mirroring tokenBucket's approach but inverted (water drains vs tokens fill).
type leakyBucket struct {
	rate     float64 // drain rate (requests per second)
	capacity float64 // max water level (burst capacity)

	water    atomic.Uint64 // math.Float64bits(float64) — current water level
	lastTime atomic.Int64  // unix nano of last drain
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
func (lb *leakyBucket) nowTime() time.Time {
	return lb.now()
}

func newLeakyBucket(rate float64, burst int) *leakyBucket {
	if burst < 1 {
		burst = 1
	}
	lb := &leakyBucket{rate: rate, capacity: float64(burst), now: time.Now}
	lb.water.Store(0) // start empty (all requests allowed initially)
	lb.lastTime.Store(lb.nowTime().UnixNano())
	return lb
}

func (lb *leakyBucket) Allow() bool {
	if lb.closed.Load() {
		return false
	}
	return lb.acquire(1)
}

func (lb *leakyBucket) TryAcquire(n int) bool {
	if n <= 0 {
		return true
	}
	if lb.closed.Load() {
		return false
	}
	return lb.acquire(n)
}

func (lb *leakyBucket) acquire(n int) bool {
	now := lb.nowTime().UnixNano()
	last := lb.lastTime.Load()
	if now < last {
		now = last // clock moved backward
	}
	for attempt := 0; attempt < 2; attempt++ {
		curBits := lb.water.Load()
		cur := math.Float64frombits(curBits)
		drain := float64(now-last) / 1e9 * lb.rate
		if drain < 0 {
			drain = 0
		}
		level := cur - drain
		if level < 0 {
			level = 0
		}
		if level+float64(n) > lb.capacity {
			lb.denied.Add(1)
			return false
		}
		next := level + float64(n)
		if lb.water.CompareAndSwap(curBits, math.Float64bits(next)) {
			lb.lastTime.CompareAndSwap(last, now)
			lb.allowed.Add(1)
			lb.acquired.Add(uint64(n))
			return true
		}
		last = lb.lastTime.Load()
	}
	lb.denied.Add(1)
	return false
}

func (lb *leakyBucket) Wait(ctx context.Context) error {
	// Closed short-circuit: match Allow/TryAcquire (see tokenBucket.Wait).
	if lb.closed.Load() {
		if err := ctx.Err(); err != nil {
			return err
		}
		return ErrLimiterClosed
	}
	if lb.Allow() {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		timer := time.NewTimer(lb.nextDrainDelay())
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if lb.Allow() {
			return nil
		}
	}
}

func (lb *leakyBucket) nextDrainDelay() time.Duration {
	now := lb.nowTime().UnixNano()
	last := lb.lastTime.Load()
	if now < last {
		now = last
	}
	cur := math.Float64frombits(lb.water.Load())
	drain := float64(now-last) / 1e9 * lb.rate
	if drain < 0 {
		drain = 0
	}
	level := cur - drain
	if level < 0 {
		level = 0
	}
	if level+1 <= lb.capacity {
		return 0
	}
	deficit := (level + 1) - lb.capacity
	secs := deficit / lb.rate
	d := time.Duration(secs * float64(time.Second))
	if d < time.Microsecond {
		d = time.Microsecond
	}
	if d > time.Millisecond {
		d = time.Millisecond
	}
	return d
}

func (lb *leakyBucket) Close() { lb.closed.Store(true) }
func (lb *leakyBucket) Metrics() LimiterMetrics {
	return LimiterMetrics{
		Allowed:  lb.allowed.Load(),
		Denied:   lb.denied.Load(),
		Acquired: lb.acquired.Load(),
	}
}
