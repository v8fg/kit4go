// Package debounce provides debounce and throttle utilities for function calls.
// Pure standard library.
//
// Debounce: wait for a quiet period before executing; if called again during
// the wait, reset the timer. Useful for "save after user stops typing" or
// "reload config after changes settle".
//
// Throttle: execute at most once per interval, dropping intermediate calls.
// Useful for rate-limiting expensive operations (metrics flush, log rotation).
//
// Ad-tech uses: debounce config-reload callbacks (avoid thrashing on rapid
// flag changes), throttle metrics flush (don't flush more than once per second),
// debounce creative-cache refresh (batch rapid invalidation signals).
package debounce

import (
	"sync"
	"sync/atomic"
	"time"
)

// Debounce coalesces rapid calls into a single execution after a quiet period.
type Debounce struct {
	mu       sync.Mutex
	after    time.Duration
	fn       func()
	timer    *time.Timer
	lastArgs atomic.Pointer[any]
	closed   atomic.Bool
}

// New builds a debounce that calls fn after `after` of inactivity following the
// last Call. The last argument passed to Call is used when fn fires.
func New(after time.Duration, fn func()) *Debounce {
	if fn == nil {
		panic("debounce: fn is required")
	}
	return &Debounce{after: after, fn: fn}
}

// Call schedules (or reschedules) the debounced execution. The arg is stored
// and available via LastArg when fn fires (use a closure to capture it).
func (d *Debounce) Call() {
	if d.closed.Load() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.after, d.fn)
}

// CallWith stores arg as the last argument and schedules execution.
func (d *Debounce) CallWith(arg any) {
	d.lastArgs.Store(&arg)
	d.Call()
}

// LastArg returns the last argument passed to CallWith (nil if none).
func (d *Debounce) LastArg() any {
	p := d.lastArgs.Load()
	if p == nil {
		return nil
	}
	return *p
}

// Flush executes immediately if a call is pending (cancels the timer).
func (d *Debounce) Flush() {
	d.mu.Lock()
	timer := d.timer
	d.timer = nil
	d.mu.Unlock()
	if timer != nil {
		timer.Stop()
		go d.fn()
	}
}

// Cancel discards the pending call (if any).
func (d *Debounce) Cancel() {
	d.mu.Lock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	d.mu.Unlock()
}

// Close stops the debouncer; subsequent Call/Flush are no-ops.
func (d *Debounce) Close() {
	d.Cancel()
	d.closed.Store(true)
}

// Pending reports whether a call is scheduled but not yet fired.
func (d *Debounce) Pending() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.timer != nil
}

// Throttle allows at most one execution per interval; subsequent calls within
// the interval are dropped. The last dropped call's argument is retained for the
// trailing edge option.
type Throttle struct {
	mu        sync.Mutex
	interval  time.Duration
	fn        func()
	last      time.Time
	closed    atomic.Bool
	callCount atomic.Int64
}

// NewThrottle builds a throttle that calls fn at most once per interval.
func NewThrottle(interval time.Duration, fn func()) *Throttle {
	if fn == nil {
		panic("debounce: fn is required")
	}
	return &Throttle{interval: interval, fn: fn}
}

// Call executes fn if at least `interval` has elapsed since the last execution.
// Returns true if fn was called, false if throttled.
func (t *Throttle) Call() bool {
	if t.closed.Load() {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if now.Sub(t.last) >= t.interval {
		t.last = now
		t.callCount.Add(1)
		go t.fn()
		return true
	}
	return false
}

// CallBlocking is like Call but executes fn synchronously (blocks until done).
func (t *Throttle) CallBlocking() bool {
	if t.closed.Load() {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if now.Sub(t.last) >= t.interval {
		t.last = now
		t.callCount.Add(1)
		t.fn()
		return true
	}
	return false
}

// Calls returns the total number of successful (non-throttled) calls.
func (t *Throttle) Calls() int64 { return t.callCount.Load() }

// Close stops the throttler; subsequent Call returns false.
func (t *Throttle) Close() { t.closed.Store(true) }
