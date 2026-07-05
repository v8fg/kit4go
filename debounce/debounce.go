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
//
// Concurrency: safe for concurrent use. Call/CallWith/Flush/Cancel/Pending/Close
// serialise via an internal sync.Mutex; the last-argument pointer uses atomic.
// The debounced fn fires on time.AfterFunc's goroutine, and Flush spawns a fresh
// goroutine, so fn must be re-entrant and non-blocking. A panic in fn is
// recovered (counted in Recovered(), surfaced via SetOnPanic) — it no longer
// crashes the process.
// Close cancels the pending timer; calls after Close are no-ops.
type Debounce struct {
	mu       sync.Mutex
	after    time.Duration
	fn       func()
	timer    *time.Timer
	lastArgs atomic.Pointer[any]
	closed   atomic.Bool

	recovered uint64    // count of fn panics recovered (observable; L5)
	onPanic   func(any) // optional hook fired on a recovered fn panic
}

// New builds a debounce that calls fn after `after` of inactivity following the
// last Call. The last argument passed to Call is used when fn fires.
func New(after time.Duration, fn func()) *Debounce {
	if fn == nil {
		panic("debounce: fn is required")
	}
	d := &Debounce{after: after}
	// Wrap fn so the AfterFunc goroutine never invokes the user fn after Close
	// (Stop() returns false for an already-dispatched timer; without this guard
	// fn would fire once after Close).
	d.fn = func() {
		if d.closed.Load() {
			return
		}
		defer func() {
			if r := recover(); r != nil {
				atomic.AddUint64(&d.recovered, 1)
				if d.onPanic != nil {
					d.onPanic(r)
				}
			}
		}()
		fn()
	}
	return d
}

// SetOnPanic installs a hook fired when fn panics. Also counted in Recovered().
func (d *Debounce) SetOnPanic(fn func(any)) { d.onPanic = fn }

// Recovered returns the total fn panics recovered.
func (d *Debounce) Recovered() uint64 { return atomic.LoadUint64(&d.recovered) }

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
	// Stop() == true means the timer was still pending (had not fired), so the
	// AfterFunc goroutine will NOT run — fire it ourselves. Stop() == false
	// means it already fired (or was stopped), so fn already ran via AfterFunc;
	// firing again here would double-execute. This guard closes that race.
	if timer != nil && timer.Stop() {
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

	now func() time.Time // injectable clock seam; defaults to time.Now (E5)

	recovered uint64    // count of fn panics recovered in Call's goroutine (L5)
	onPanic   func(any) // optional hook fired on a recovered fn panic
}

// NewThrottle builds a throttle that calls fn at most once per interval.
func NewThrottle(interval time.Duration, fn func()) *Throttle {
	if fn == nil {
		panic("debounce: fn is required")
	}
	return &Throttle{interval: interval, fn: fn, now: time.Now}
}

// Call executes fn if at least `interval` has elapsed since the last execution.
// Returns true if fn was called, false if throttled.
func (t *Throttle) Call() bool {
	if t.closed.Load() {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if now.Sub(t.last) >= t.interval {
		t.last = now
		t.callCount.Add(1)
		go t.safeFire()
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
	now := t.now()
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

// safeFire runs fn in the Call goroutine with panic recovery — a panicking fn
// no longer crashes the process; counted + surfaced via the hook (L5).
func (t *Throttle) safeFire() {
	defer func() {
		if r := recover(); r != nil {
			atomic.AddUint64(&t.recovered, 1)
			if t.onPanic != nil {
				t.onPanic(r)
			}
		}
	}()
	t.fn()
}

// SetOnPanic installs a hook fired when Call's fn panics.
func (t *Throttle) SetOnPanic(fn func(any)) { t.onPanic = fn }

// Recovered returns the total fn panics recovered.
func (t *Throttle) Recovered() uint64 { return atomic.LoadUint64(&t.recovered) }
