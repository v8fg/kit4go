package log4go

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test_RateAlerter_BelowThreshold: events under the threshold never fire.
func Test_RateAlerter_BelowThreshold(t *testing.T) {
	a := NewRateAlerter(time.Minute, 5)
	for i := 0; i < 4; i++ {
		if a.Allow() {
			t.Fatalf("fired at event %d below threshold 5", i+1)
		}
	}
	if got := a.Count(); got != 4 {
		t.Errorf("Count=%d want 4", got)
	}
}

// Test_RateAlerter_AtThreshold: the threshold-th event fires exactly once; the
// cooldown (== window) suppresses subsequent fires within the window.
func Test_RateAlerter_AtThreshold(t *testing.T) {
	a := NewRateAlerter(time.Minute, 3)
	var fires int
	for i := 0; i < 10; i++ {
		if a.Allow() {
			fires++
		}
	}
	if fires != 1 {
		t.Errorf("fires=%d want 1 (cooldown should suppress after first fire)", fires)
	}
}

// Test_RateAlerter_CooldownAllowsRefire: with a short cooldown, sustained
// over-threshold traffic fires repeatedly.
func Test_RateAlerter_CooldownAllowsRefire(t *testing.T) {
	a := NewRateAlerter(time.Minute, 3)
	a.SetCooldown(0) // no cooldown: every qualifying event fires
	var fires int
	for i := 0; i < 6; i++ {
		if a.Allow() {
			fires++
		}
	}
	// threshold is 3: events 3,4,5,6 (4 of them, 0-indexed 2..5) fire.
	if fires != 4 {
		t.Errorf("fires=%d want 4", fires)
	}
}

// Test_RateAlerter_WindowExpiry: after a full window elapses with no traffic, a
// new burst must count from zero.
func Test_RateAlerter_WindowExpiry(t *testing.T) {
	a := newRateAlerterAt(time.Minute, 3, time.Unix(1_000_000, 0))
	// fill to threshold at t0
	for i := 0; i < 3; i++ {
		a.allowAt(time.Unix(1_000_000, 0))
	}
	// jump forward past the window; advance+allow should see an empty window
	got := a.allowAt(time.Unix(1_000_000+120, 0)) // 120s > 60s window
	if got {
		t.Errorf("fired after window expiry on a single fresh event (sum should be 1 < 3)")
	}
	if c := a.countAt(time.Unix(1_000_000+120, 0)); c != 1 {
		t.Errorf("Count after expiry=%d want 1", c)
	}
}

// Test_RateAlerter_Concurrent: many goroutines hammering Allow must not race and
// must never panic (run under -race).
func Test_RateAlerter_Concurrent(t *testing.T) {
	a := NewRateAlerter(2*time.Second, 100)
	var wg sync.WaitGroup
	var fires int64
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var local int64
			for i := 0; i < 1000; i++ {
				if a.Allow() {
					local++
				}
			}
			atomic.AddInt64(&fires, local)
		}()
	}
	wg.Wait()
	// 16*1000 = 16000 events over a 2s window; threshold 100 with 2s cooldown
	// => at least a few fires. The exact count is timing-dependent; assert sane.
	if fires < 1 {
		t.Errorf("expected some fires under sustained load, got %d", fires)
	}
}

// allowAt / countAt / newRateAlerterAt are deterministic-time test seams for
// RateAlerter (production uses time.Now via Allow/Count).
func newRateAlerterAt(window time.Duration, threshold int, base time.Time) *RateAlerter {
	secs := int(window.Seconds())
	if secs < 1 {
		secs = 1
	}
	return &RateAlerter{
		window:    time.Duration(secs) * time.Second,
		threshold: threshold,
		cooldown:  time.Duration(secs) * time.Second,
		counts:    make([]int, secs),
		base:      base.Unix(),
	}
}

func (a *RateAlerter) allowAt(now time.Time) bool {
	sec := now.Unix()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.advance(sec)
	n := int64(len(a.counts))
	a.counts[sec%n]++
	a.sum++
	if a.sum >= a.threshold && now.Sub(a.lastFire) >= a.cooldown {
		a.lastFire = now
		return true
	}
	return false
}

func (a *RateAlerter) countAt(now time.Time) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.advance(now.Unix())
	return a.sum
}
