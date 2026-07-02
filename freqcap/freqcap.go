// Package freqcap is a per-entity sliding-window event counter for frequency
// capping: "may this key produce one more event within the window, given it has
// already produced N?".
//
// It is the per-entity *counting* sibling of package limiter: limiter throttles
// an action's global rate to protect the caller; freqcap caps how many times an
// *entity* (user, creative, device) may act per window to protect the audience.
// In-memory and exact (a timestamp per allowed event); idle keys are pruned so
// memory tracks active entities, not historical ones.
//
// Ad-tech / push uses: "show this creative to this user at most 3 times per
// hour", "notify this user at most 5 times per day", "a device may trigger at
// most K events per minute" (bot suppression).
package freqcap

import (
	"sync"
	"time"
)

// Counter tracks per-key event counts over a fixed sliding window.
//
// Concurrency: safe for concurrent use. All methods (Allow, Inc, Clear, and the
// accessors) acquire an internal sync.Mutex, so concurrent calls are serialised.
// Per-key state is independent but guarded by the single mutex.
type Counter struct {
	mu        sync.Mutex
	window    time.Duration
	maxEvents int
	maxKeys   int // 0 = unbounded
	clock     func() time.Time
	keys      map[string][]time.Time
}

// Option configures a Counter.
type Option func(*Counter)

// WithMaxKeys caps the number of tracked keys; when exceeded, idle keys (those
// with no events in the window) are pruned first. 0 (default) = unbounded.
func WithMaxKeys(n int) Option { return func(c *Counter) { c.maxKeys = n } }

// WithClock injects a clock (for tests). Defaults to time.Now.
func WithClock(f func() time.Time) Option { return func(c *Counter) { c.clock = f } }

// New builds a Counter that allows at most maxEvents per key in any window.
// Panics if maxEvents <= 0 or window <= 0.
func New(window time.Duration, maxEvents int, opts ...Option) *Counter {
	if maxEvents <= 0 {
		panic("freqcap: maxEvents must be > 0")
	}
	if window <= 0 {
		panic("freqcap: window must be > 0")
	}
	c := &Counter{
		window:    window,
		maxEvents: maxEvents,
		clock:     time.Now,
		keys:      make(map[string][]time.Time),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Allow records an event for key if the key is still under the cap within the
// window; returns true when recorded, false when the cap would be exceeded (and
// records nothing).
func (c *Counter) Allow(key string) bool {
	now := c.clock()
	cutoff := now.Add(-c.window)
	c.mu.Lock()
	defer c.mu.Unlock()
	ts := trimBefore(c.keys[key], cutoff)
	if len(ts) >= c.maxEvents {
		// At/over cap: keep the trimmed slice (still active events), reject.
		c.keys[key] = ts
		return false
	}
	ts = append(ts, now)
	c.keys[key] = ts
	c.evictIdleLocked(now)
	return true
}

// Count returns the number of events currently within the window for key
// (lazy-trimmed). It does not record an event.
func (c *Counter) Count(key string) int {
	now := c.clock()
	cutoff := now.Add(-c.window)
	c.mu.Lock()
	defer c.mu.Unlock()
	ts := trimBefore(c.keys[key], cutoff)
	c.keys[key] = ts
	return len(ts)
}

// Reset drops all recorded events for key.
func (c *Counter) Reset(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.keys, key)
}

// Len returns the number of tracked keys (active + not-yet-pruned idle).
func (c *Counter) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.keys)
}

// evictIdleLocked prunes keys whose window is empty, and — if a key cap is set
// and still exceeded — drops the oldest-starting keys. Caller holds the lock.
func (c *Counter) evictIdleLocked(now time.Time) {
	// Drop fully-idle keys (no events within the window).
	for k, ts := range c.keys {
		if len(trimBefore(ts, now.Add(-c.window))) == 0 {
			delete(c.keys, k)
		}
	}
	// If still over the key cap, drop the key with the oldest earliest event.
	for c.maxKeys > 0 && len(c.keys) > c.maxKeys {
		victim := ""
		oldest := time.Time{}
		first := true
		for k, ts := range c.keys {
			if len(ts) == 0 { // prefer an empty one
				victim = k
				break
			}
			if first || ts[0].Before(oldest) {
				oldest, victim, first = ts[0], k, false
			}
		}
		if victim == "" {
			return
		}
		delete(c.keys, victim)
	}
}

// trimBefore returns the suffix of ts with elements >= cutoff. ts is ascending.
func trimBefore(ts []time.Time, cutoff time.Time) []time.Time {
	// In-place: find the first index >= cutoff and reslice, reclaiming head.
	i := 0
	for i < len(ts) && ts[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		// Copy down to avoid retaining the prefix (and the time.Time values).
		copy(ts, ts[i:])
		ts = ts[:len(ts)-i]
	}
	return ts
}
