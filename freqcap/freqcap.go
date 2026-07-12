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
// The per-entity cap is a hard invariant: an entity is allowed at most maxEvents
// per window, period. The tracked-key ceiling (WithMaxKeys) is a SOFT cap on
// memory: a key that still has in-window events is never evicted to make room
// for a new one, because that would silently reset the entity's cap and cause
// over-delivery. When the live-key count reaches the cap, a fresh entity's first
// Allow is denied instead (memory full, all active audiences) — preferable to
// under-counting a live audience. Count is read-only and never creates map
// entries, so probing it with untrusted keys cannot grow the map.
//
// Ad-tech / push uses: "show this creative to this user at most 3 times per
// hour", "notify this user at most 5 times per day", "a device may trigger at
// most K events per minute" (bot suppression).
package freqcap

import (
	"sync"
	"time"
)

// DefaultMaxKeys is the key cap applied when WithMaxKeys is omitted. It bounds
// memory in the common "no cap configured" case so an unbounded influx of
// distinct keys cannot grow the map without limit. Pass WithMaxKeys(0) for a
// deliberately unbounded Counter (the Go zero-value convention, matching
// package hotkey); the caller must then bound the key space itself.
const DefaultMaxKeys = 10000

// Counter tracks per-key event counts over a fixed sliding window.
//
// Concurrency: safe for concurrent use. All methods (Allow, Inc, Clear, and the
// accessors) acquire an internal sync.Mutex, so concurrent calls are serialised.
// Per-key state is independent but guarded by the single mutex.
type Counter struct {
	mu        sync.Mutex
	window    time.Duration
	maxEvents int
	maxKeys   int // 0 = unbounded (Go zero value); DefaultMaxKeys applied in New when the option is omitted
	clock     func() time.Time
	keys      map[string][]time.Time
}

// Option configures a Counter.
type Option func(*Counter)

// WithMaxKeys caps the number of tracked keys. The cap is SOFT: idle keys (those
// with no events in the window) are always reclaimed to make room, but a key
// that still has in-window events is NEVER evicted — dropping it would silently
// reset that entity's cap and cause over-delivery. When the live-key count
// reaches the cap, a fresh entity's first Allow is denied (return false) rather
// than dropping a live entity.
//
// Omitting the option applies DefaultMaxKeys as a sane ceiling. Pass
// WithMaxKeys(0) — the Go zero value — for an unbounded map; use this only when
// the caller bounds the key space itself, since nothing else limits memory
// growth. This matches the convention in package hotkey (where the cap, by
// contrast, IS a hard eviction of the coldest key, which is correct for a
// hot-key detector but wrong for a frequency cap).
func WithMaxKeys(n int) Option { return func(c *Counter) { c.maxKeys = n } }

// WithClock injects a clock (for tests). Defaults to time.Now.
func WithClock(f func() time.Time) Option { return func(c *Counter) { c.clock = f } }

// New builds a Counter that allows at most maxEvents per key in any window.
// Panics if maxEvents <= 0 or window <= 0.
//
// Unless WithMaxKeys overrides it, the tracked-key cap is DefaultMaxKeys. An
// explicit WithMaxKeys(0) selects an unbounded map (the Go zero-value
// convention, matching package hotkey).
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
		maxKeys:   DefaultMaxKeys,
		clock:     time.Now,
		keys:      make(map[string][]time.Time),
	}
	for _, opt := range opts {
		opt(c)
	}
	// WithMaxKeys(0) is honored as unbounded (the Go zero-value convention);
	// the DefaultMaxKeys initialiser above only wins when the option is
	// omitted. Negative values are also treated as unbounded for robustness.
	if c.maxKeys < 0 {
		c.maxKeys = 0
	}
	return c
}

// Allow records an event for key if the key is still under the cap within the
// window; returns true when recorded, false when the cap would be exceeded (and
// records nothing).
//
// maxKeys is enforced as a SOFT cap: if admitting a new key would push the live
// key count to maxKeys and every existing key is still in-window, the event is
// DENIED (return false) rather than dropping a live entity. Dropping a live
// entity would silently reset its cap and cause over-delivery, breaking the core
// invariant ("at most maxEvents per entity per window"). An idle key is reclaimed
// to make room when one exists. Recording another event for an already-tracked,
// in-window key never counts toward the cap.
func (c *Counter) Allow(key string) bool {
	now := c.clock()
	cutoff := now.Add(-c.window)
	c.mu.Lock()
	defer c.mu.Unlock()
	ts, ok := c.keys[key]
	trimmed := trimBefore(ts, cutoff)
	if len(trimmed) >= c.maxEvents {
		// At/over cap (maxEvents >= 1, so trimmed is non-empty here): keep the
		// trimmed slice (still active events), reject.
		c.keys[key] = trimmed
		return false
	}
	// Admitting this event adds a key to the live set only when the key is not
	// already a live, in-window entry. Reclaim idle keys first; then, if the key
	// is new-to-live and the live set is already at the soft cap, deny rather
	// than evict a live entity (over-delivery guard).
	if !ok || len(trimmed) == 0 {
		c.evictIdleLocked(now)
		if c.maxKeys > 0 && len(c.keys) >= c.maxKeys {
			// evictIdleLocked leaves only live keys, so len(c.keys) == live count.
			return false
		}
	}
	trimmed = append(trimmed, now)
	c.keys[key] = trimmed
	return true
}

// Count returns the number of events currently within the window for key
// (lazy-trimmed). It does not record an event and is read-only with respect to
// the key set: a key that is absent, or whose window has drained to empty, is
// never created in the map by calling Count. This keeps the read path from
// bypassing the maxKeys cap (an attacker probing Count with distinct untrusted
// keys must not grow the map).
func (c *Counter) Count(key string) int {
	now := c.clock()
	cutoff := now.Add(-c.window)
	c.mu.Lock()
	defer c.mu.Unlock()
	ts, ok := c.keys[key]
	if !ok {
		return 0 // absent key: do NOT create a map entry
	}
	trimmed := trimBefore(ts, cutoff)
	if len(trimmed) == 0 {
		delete(c.keys, key) // drained to empty: reclaim the entry
		return 0
	}
	c.keys[key] = trimmed
	return len(trimmed)
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

// evictIdleLocked prunes keys whose window has drained to empty. It NEVER evicts
// a key that still has in-window events: doing so would silently reset that
// entity's cap and cause over-delivery, breaking the core freqcap invariant
// ("at most maxEvents per entity per window"). Caller holds the lock.
//
// As a consequence maxKeys is a SOFT cap on memory: once the live key count
// reaches the cap, evictIdleLocked cannot free more room (no idle keys remain),
// and Allow denies the new event instead of dropping a live entity. A former
// oldest-start eviction loop lived here; it was removed because under the soft
// cap it could only ever drop live keys — exactly the over-delivery bug.
//
// Cost: this is an O(maxKeys) scan run on the fresh-key Allow path (the
// documented "Allow triggers idle pruning" contract). Under a distinct-key
// flood against a saturated cap (maxKeys live entities, then more distinct
// keys) every denied Allow scans the whole map under c.mu, amplifying lock
// contention and capping lock-throughput. For a high-cardinality adversarial
// key space, shard counters (key%N → N Counters) or use an approximate backend
// (countmin) so the per-instance map stays small.
func (c *Counter) evictIdleLocked(now time.Time) {
	cutoff := now.Add(-c.window)
	for k, ts := range c.keys {
		if len(trimBefore(ts, cutoff)) == 0 {
			delete(c.keys, k)
		}
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
