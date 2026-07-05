// Package featureflag provides simple, in-process feature toggles with
// percentage rollout, allowlist, and time-gate. Pure standard library.
//
// Ad-tech / finance uses: canary rollout of a new bid strategy, gradual %
// enable of a new payment provider, allowlist internal testing, time-gated
// campaign features.
package featureflag

import (
	"hash/fnv"
	"sync"
	"time"
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
	f.mu.RLock()
	defer f.mu.RUnlock()
	if !f.enabled {
		return false
	}
	if !f.notBefore.IsZero() && time.Now().Before(f.notBefore) {
		return false
	}
	if _, ok := f.allowlist[key]; ok {
		return true
	}
	// percentage=0 means nobody (except allowlist); 100 means everyone.
	if f.percentage >= 100 {
		return true
	}
	if f.percentage == 0 {
		return false
	}
	return hashPercent(key) < f.percentage
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

// hashPercent returns a 0-99 value for the key (deterministic, uniform).
func hashPercent(key string) uint {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return uint(h.Sum32() % 100)
}
