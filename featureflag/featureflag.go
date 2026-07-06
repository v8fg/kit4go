// Package featureflag provides simple, in-process feature toggles with
// percentage rollout, allowlist, and time-gate. Pure standard library.
//
// Ad-tech / finance uses: canary rollout of a new bid strategy, gradual %
// enable of a new payment provider, allowlist internal testing, time-gated
// campaign features.
package featureflag

import (
	"sync"
	"time"
)

// FNV-1a 32-bit constants (identical to hash/fnv.New32a), inlined so the
// rollout hash is computed over the key with zero allocations. hash/fnv's
// hash.Hash32.Write goes through an interface and forces the []byte(key)
// conversion to escape; the inline form avoids both that escape and the
// per-call hasher object, removing every allocation from the hot path that
// previously sat under the read lock (D6). A golden test pins this to the
// stdlib implementation.
const (
	fnv32aOffsetBasis uint32 = 2166136261
	fnv32aPrime       uint32 = 16777619
)

// Flag is a single feature toggle with rollout controls. Safe for concurrent use.
type Flag struct {
	mu         sync.RWMutex
	enabled    bool
	percentage uint // default 100 (full rollout); 0 = nobody except allowlist
	allowlist  map[string]struct{}
	notBefore  time.Time // zero value = no time gate
}

// Option configures a Flag.
type Option func(*Flag)

// WithPercentage sets the rollout percentage (0-100). Users are assigned
// deterministically by hashing their key, so the same key always gets the same
// answer for a given percentage.
func WithPercentage(p uint) Option {
	return func(f *Flag) {
		if p > 100 {
			p = 100
		}
		f.percentage = p
	}
}

// WithAllowlist adds keys that are always enabled (regardless of percentage).
func WithAllowlist(keys ...string) Option {
	return func(f *Flag) {
		for _, k := range keys {
			f.allowlist[k] = struct{}{}
		}
	}
}

// WithStartTime gates the flag: it returns false before this time (even if
// enabled + percentage=100). Zero value = no time gate.
func WithStartTime(t time.Time) Option {
	return func(f *Flag) { f.notBefore = t }
}

// WithEnabled sets the initial enabled state (default false).
func WithEnabled(on bool) Option {
	return func(f *Flag) { f.enabled = on }
}

// New builds a Flag. Default: disabled, 100% (full rollout when enabled), no
// allowlist, no time gate.
func New(opts ...Option) *Flag {
	f := &Flag{
		percentage: 100,
		allowlist:  make(map[string]struct{}),
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Enabled reports whether the flag is on for the given key. The key is a stable
// user/request identifier (user ID, session ID) used for consistent percentage
// rollout. Evaluation order:
//  1. Flag globally disabled → false.
//  2. Before start time → false.
//  3. Key in allowlist → true.
//  4. Percentage rollout (hash(key) % 100 < percentage).
func (f *Flag) Enabled(key string) bool {
	// Snapshot all guarded state under the read lock, then release it before
	// hashing. This keeps the FNV-1a computation (and any allocation it could
	// cause) strictly outside the critical section, so the read lock is held
	// only for three field reads (D6 fix).
	f.mu.RLock()
	if !f.enabled {
		f.mu.RUnlock()
		return false
	}
	if !f.notBefore.IsZero() && time.Now().Before(f.notBefore) {
		f.mu.RUnlock()
		return false
	}
	_, allowlisted := f.allowlist[key]
	percentage := f.percentage
	f.mu.RUnlock()

	if allowlisted {
		return true
	}
	// percentage=0 means nobody (except allowlist); 100 means everyone.
	if percentage >= 100 {
		return true
	}
	if percentage == 0 {
		return false
	}
	return hashPercent(key) < percentage
}

// Enable turns the flag on globally.
func (f *Flag) Enable() { f.mu.Lock(); f.enabled = true; f.mu.Unlock() }

// Disable turns the flag off globally.
func (f *Flag) Disable() { f.mu.Lock(); f.enabled = false; f.mu.Unlock() }

// SetPercentage adjusts the rollout percentage at runtime.
func (f *Flag) SetPercentage(p uint) {
	if p > 100 {
		p = 100
	}
	f.mu.Lock()
	f.percentage = p
	f.mu.Unlock()
}

// Allow adds keys to the allowlist at runtime.
func (f *Flag) Allow(keys ...string) {
	f.mu.Lock()
	for _, k := range keys {
		f.allowlist[k] = struct{}{}
	}
	f.mu.Unlock()
}

// Revoke removes keys from the allowlist.
func (f *Flag) Revoke(keys ...string) {
	f.mu.Lock()
	for _, k := range keys {
		delete(f.allowlist, k)
	}
	f.mu.Unlock()
}

// hashPercent returns a 0-99 value for the key (deterministic, uniform). It
// uses FNV-1a 32-bit inlined over the key bytes, so the call allocates nothing
// and never holds the read lock (D6 fix).
func hashPercent(key string) uint {
	h := fnv32aOffsetBasis
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= fnv32aPrime
	}
	return uint(h % 100)
}
