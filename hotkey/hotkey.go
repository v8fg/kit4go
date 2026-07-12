// Package hotkey detects heavy-hitter (hot) keys in a sliding time window — the
// keys receiving disproportionate traffic in the last N seconds.
//
// It tracks per-key hit timestamps in a sliding window and returns the top-K by
// count on demand. Idle keys (no hits in the window) are pruned so memory tracks
// active keys only. Pure standard library.
//
// By default a Detector tracks at most DefaultMaxKeys keys; once that ceiling is
// reached the key with the fewest hits (ties broken by oldest last-touch) is
// dropped. Pass WithMaxKeys(0) — the Go zero value — to disable the cap entirely
// (unbounded), or WithMaxKeys(n) for a custom ceiling. Package freqcap shares
// the same convention.
//
// Ad-tech uses: detect hot SSP endpoints, hot creatives, hot user segments, or
// hot auction IDs that are skewing load — then route them to a local cache, a
// dedicated shard, or throttle them individually. Pair with countmin for an
// approximate backend when the key space is too large for exact per-key tracking.
package hotkey

import (
	"cmp"
	"slices"
	"sync"
	"time"
)

// DefaultMaxKeys is the default ceiling on the number of tracked keys applied
// when WithMaxKeys is omitted. It is large enough for typical hot-key detection
// (a few thousand active keys) while bounding memory if the key space runs away.
// Use WithMaxKeys(0) — the Go zero value — to disable the cap (unbounded), or
// WithMaxKeys(n) for a custom ceiling. This matches the convention in package
// freqcap.
const DefaultMaxKeys = 10000

// HotKey is a key with its current window count.
type HotKey struct {
	Key   string
	Count int
}

// Detector tracks heavy-hitter keys in a sliding window.
//
// Concurrency: safe for concurrent use. Touch, TopN, Reset, and metrics each
// acquire an internal sync.Mutex (serialised). The window advances on the wall
// clock under the lock; Touch is the hot path and may contend under very high
// key-cardinality — shard detectors if that becomes a bottleneck.
type Detector struct {
	mu            sync.Mutex
	window        time.Duration
	topK          int
	maxKeys       int
	maxHitsPerKey int
	clock         func() time.Time
	keys          map[string][]time.Time // ascending timestamps
}

// Option configures a Detector.
type Option func(*Detector)

// WithMaxKeys caps the number of tracked keys; idle keys are pruned first, then
// the key with the fewest hits is dropped (ties broken by oldest last-touch).
//
// WithMaxKeys(0) — the Go zero value — disables the cap (unbounded). Negative
// values are treated the same way. When this option is omitted, New applies
// DefaultMaxKeys as a sane ceiling so a runaway key space cannot grow the map
// without bound. Pass WithMaxKeys(0) explicitly to restore fully unbounded
// tracking; this matches the convention in package freqcap.
func WithMaxKeys(n int) Option { return func(d *Detector) { d.maxKeys = n } }

// WithMaxHitsPerKey caps the number of timestamps retained for a single key.
// A sustained heavy hitter would otherwise store every hit in the window, so the
// hotter a key the more memory it burns — the detector's own target workload.
// The cap bounds per-key memory at maxHitsPerKey timestamps; once reached the
// oldest hits are dropped, so Count/Top slightly under-count extreme heavy
// hitters (still rank-correct: a key over the cap is still reported as hot).
//
// 0 (the zero value, also the default) disables the cap. That keeps Count exact,
// which is fine when per-key hit volume is bounded by the window duration; for
// unbounded-rate heavy hitters where memory is a concern, set an explicit cap or
// pair with countmin for an approximate backend.
func WithMaxHitsPerKey(n int) Option { return func(d *Detector) { d.maxHitsPerKey = n } }

// WithClock injects a clock for tests.
func WithClock(f func() time.Time) Option { return func(d *Detector) { d.clock = f } }

// New builds a Detector that reports the top topK keys by hit count in the last
// window. Panics if window <= 0 or topK <= 0.
func New(window time.Duration, topK int, opts ...Option) *Detector {
	if window <= 0 {
		panic("hotkey: window must be > 0")
	}
	if topK <= 0 {
		panic("hotkey: topK must be > 0")
	}
	d := &Detector{
		window:  window,
		topK:    topK,
		maxKeys: DefaultMaxKeys,
		clock:   time.Now,
		keys:    make(map[string][]time.Time),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Touch records a hit for key at the current time.
func (d *Detector) Touch(key string) {
	now := d.clock()
	d.mu.Lock()
	defer d.mu.Unlock()
	ts := d.keys[key]
	ts = trimBefore(ts, now.Add(-d.window))
	ts = append(ts, now)
	if d.maxHitsPerKey > 0 && len(ts) > d.maxHitsPerKey {
		// Bound per-key storage (D5): a sustained heavy hitter cannot grow its
		// slice past the cap. Drop the oldest hits; the cap is on retained volume.
		ts = ts[len(ts)-d.maxHitsPerKey:]
	}
	d.keys[key] = ts
	d.evictIdleLocked(now)
}

// Count returns the number of hits for key in the current window. It is
// read-only with respect to the key set: an absent key, or one whose window has
// drained to empty, is never created in the map by calling Count. This keeps
// the read path from bypassing the maxKeys cap (an attacker probing Count with
// distinct untrusted keys must not grow the map).
func (d *Detector) Count(key string) int {
	now := d.clock()
	d.mu.Lock()
	defer d.mu.Unlock()
	ts, ok := d.keys[key]
	if !ok {
		return 0 // absent key: do NOT create a map entry
	}
	trimmed := trimBefore(ts, now.Add(-d.window))
	if len(trimmed) == 0 {
		delete(d.keys, key) // drained to empty: reclaim the entry
		return 0
	}
	d.keys[key] = trimmed
	return len(trimmed)
}

// Top returns the top-K keys by hit count in the current window, sorted by count
// descending. Keys with zero hits are excluded.
func (d *Detector) Top() []HotKey {
	now := d.clock()
	cutoff := now.Add(-d.window)
	d.mu.Lock()
	defer d.mu.Unlock()
	// Trim + collect counts; prune idle keys.
	results := make([]HotKey, 0, len(d.keys))
	for k, ts := range d.keys {
		trimmed := trimBefore(ts, cutoff)
		if len(trimmed) == 0 {
			delete(d.keys, k)
			continue
		}
		d.keys[k] = trimmed
		results = append(results, HotKey{Key: k, Count: len(trimmed)})
	}
	// Sort by count desc, take top-K.
	slices.SortFunc(results, func(a, b HotKey) int { return cmp.Compare(b.Count, a.Count) })
	if len(results) > d.topK {
		results = results[:d.topK]
	}
	return results
}

// Reset clears all tracked keys.
func (d *Detector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.keys = make(map[string][]time.Time)
}

// Len returns the number of tracked (non-idle) keys.
func (d *Detector) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.keys)
}

// evictIdleLocked prunes fully-idle keys, then enforces maxKeys by dropping the
// key with the fewest hits. Caller holds the lock.
func (d *Detector) evictIdleLocked(now time.Time) {
	cutoff := now.Add(-d.window)
	for k, ts := range d.keys {
		if len(trimBefore(ts, cutoff)) == 0 {
			delete(d.keys, k)
		}
	}
	for d.maxKeys > 0 && len(d.keys) > d.maxKeys {
		// Drop the key with the fewest (trimmed) timestamps; ties broken by
		// oldest last-touch (LRU) so eviction is deterministic, not random map
		// order.
		victim := ""
		minCount := -1
		var victimLast time.Time
		for k, ts := range d.keys {
			trimmed := trimBefore(ts, cutoff)
			c := len(trimmed)
			last := time.Time{}
			if len(trimmed) > 0 {
				last = trimmed[len(trimmed)-1]
			}
			if minCount < 0 || c < minCount || (c == minCount && last.Before(victimLast)) {
				minCount = c
				victim = k
				victimLast = last
			}
		}
		if victim == "" {
			return
		}
		delete(d.keys, victim)
	}
}

// trimBefore returns the suffix of ts with elements >= cutoff. ts is ascending.
func trimBefore(ts []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(ts) && ts[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		copy(ts, ts[i:])
		ts = ts[:len(ts)-i]
	}
	return ts
}
