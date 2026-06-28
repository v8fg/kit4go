package log4go

import (
	"sync"
	"time"
)

// RateAlerter suppresses alert storms with a sliding, second-bucket window.
// Each Allow() call records one event; Allow() returns true (and arms the
// cooldown) only when the in-window event count reaches the threshold AND the
// cooldown since the last fire has elapsed.
//
// Use it to turn a per-record alert into a throttled one — e.g. fire a webhook
// at most once per minute once errors exceed 100/min, instead of once per
// record. As a WebhookWriter gate it implements the "threshold summary" trigger
// mode; standalone callers can also poll Count() to render a summary payload.
//
// Implementation: a fixed ring of per-second counters (one slot per second of
// the window). advance() rolls the ring forward and subtracts expired slots
// from the running sum, so Allow() is O(1) amortized with no per-event
// allocation — safe to call on a hot path.
type RateAlerter struct {
	window    time.Duration
	threshold int
	cooldown  time.Duration

	mu      sync.Mutex
	counts  []int   // ring of per-second counters; len == window seconds
	base    int64   // unix second of the newest bucket advanced to
	sum     int     // sum of all live buckets (the current in-window count)
	lastFire time.Time
}

// NewRateAlerter builds a gate that fires after at least threshold events land
// inside window. window is rounded down to whole seconds (min 1s); threshold is
// clamped to >= 1. The default cooldown equals the window (one fire per window
// at most); override with SetCooldown.
func NewRateAlerter(window time.Duration, threshold int) *RateAlerter {
	secs := int(window.Seconds())
	if secs < 1 {
		secs = 1
	}
	if threshold < 1 {
		threshold = 1
	}
	return &RateAlerter{
		window:    time.Duration(secs) * time.Second,
		threshold: threshold,
		cooldown:  time.Duration(secs) * time.Second,
		counts:    make([]int, secs),
		base:      time.Now().Unix(),
	}
}

// SetCooldown sets the minimum interval between fires. Defaults to the window.
// Set below the window to allow multiple fires within one window once the
// threshold is sustained.
func (a *RateAlerter) SetCooldown(d time.Duration) { a.cooldown = d }

// Count returns the current in-window event count (best-effort snapshot, for
// formatters that want to report "N events in the last minute").
func (a *RateAlerter) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.advance(time.Now().Unix())
	return a.sum
}

// Allow records one event and reports whether the alert should fire now.
// Thread-safe.
func (a *RateAlerter) Allow() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Read now UNDER the lock: a value read before acquiring the lock can be
	// older than base (advanced by a concurrent caller while we waited), which
	// would otherwise trip advance's backward path and silently destroy live
	// counts — the alert could then fail to fire when it should.
	now := time.Now()
	sec := now.Unix()
	a.advance(sec)
	if sec < a.base {
		sec = a.base // wall clock regressed: charge the current bucket
	}
	n := int64(len(a.counts))
	a.counts[sec%n]++
	a.sum++
	if a.sum >= a.threshold && now.Sub(a.lastFire) >= a.cooldown {
		a.lastFire = now
		return true
	}
	return false
}

// advance rolls the bucket ring forward to sec, zeroing buckets that have fallen
// out of the window and subtracting them from sum. After it returns, the bucket
// at index sec%n is cleared and ready for a fresh count.
func (a *RateAlerter) advance(sec int64) {
	n := int64(len(a.counts))
	if sec <= a.base {
		// Stale read or wall-clock regression (NTP). Do NOT clear: destroying
		// live counts on a backward timestamp would under-count events (the
		// alert might fail to fire when it should). The caller clamps its write
		// to base, so leave the window untouched.
		return
	}
	if sec-a.base >= n {
		// a full window (or more) has elapsed: every bucket is expired.
		for i := range a.counts {
			a.sum -= a.counts[i]
			a.counts[i] = 0
		}
		a.base = sec
		return
	}
	for a.base < sec {
		a.base++
		i := a.base % n
		a.sum -= a.counts[i]
		a.counts[i] = 0
	}
}
