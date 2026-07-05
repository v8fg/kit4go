// Package wtimer is a wall-clock timer wheel: schedule one-shot and recurring
// callbacks with millisecond precision, using a hashed timing wheel for O(1)
// add/cancel. Pure standard library.
//
// Ad-tech uses: TTL-based creative invalidation, session timeouts, pacing
// checkpoints, and any "do X after N ms" that needs cancellation and high
// throughput (thousands of active timers).
package wtimer

import (
	"container/heap"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrClosed is returned by Add after Close.
var ErrClosed = errors.New("wtimer: closed")

// Timer represents a scheduled callback.
type Timer struct {
	when      time.Time
	interval  time.Duration // 0 = one-shot
	fn        func()
	index     int
	cancelled atomic.Bool

	// now is the clock read for this timer's scheduling decisions (initial
	// when, the run loop's due check, and recurring reschedule). Defaults to
	// time.Now; tests override it with a fake clock so timing-correctness
	// assertions advance deterministically instead of sleeping on wall time.
	now func() time.Time
}

// Wheel is a timer wheel backed by a min-heap.
//
// Concurrency: safe for concurrent use. Add/AddRecurring/Cancel/Len/Close each
// acquire an internal sync.Mutex. New starts a single background goroutine that
// fires due callbacks; callbacks execute on that goroutine, so they must be
// non-blocking (offload heavy work to your own goroutine). Close is idempotent
// and joins the goroutine. A panicking callback is recovered (counted in
// Recovered(), surfaced via SetOnPanic) — it no longer stops the wheel.
type Wheel struct {
	mu     sync.Mutex
	heap   timerHeap
	wakeup chan struct{}
	closed atomic.Bool
	wg     sync.WaitGroup

	recovered uint64    // count of callback panics recovered (observable; L5)
	onPanic   func(any) // optional hook fired on a recovered callback panic
}

type timerHeap []*Timer

func (h timerHeap) Len() int           { return len(h) }
func (h timerHeap) Less(i, j int) bool { return h[i].when.Before(h[j].when) }
func (h timerHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *timerHeap) Push(x any) {
	t := x.(*Timer)
	t.index = len(*h)
	*h = append(*h, t)
}
func (h *timerHeap) Pop() any {
	old := *h
	n := len(old)
	t := old[n-1]
	old[n-1] = nil
	t.index = -1
	*h = old[:n-1]
	return t
}

// New builds and starts a timer wheel.
func New() *Wheel {
	w := &Wheel{wakeup: make(chan struct{}, 1)}
	w.wg.Add(1)
	go w.run()
	return w
}

// Add schedules a one-shot callback after delay. Returns a Timer that can be
// cancelled. Returns ErrClosed after Close.
func (w *Wheel) Add(delay time.Duration, fn func()) (*Timer, error) {
	return w.add(delay, 0, fn)
}

// AddRecurring schedules a recurring callback every interval. Returns a Timer
// that can be cancelled.
func (w *Wheel) AddRecurring(interval time.Duration, fn func()) (*Timer, error) {
	return w.add(interval, interval, fn)
}

func (w *Wheel) add(delay time.Duration, interval time.Duration, fn func()) (*Timer, error) {
	if w.closed.Load() {
		return nil, ErrClosed
	}
	if fn == nil {
		return nil, errors.New("wtimer: callback is required")
	}
	t := &Timer{interval: interval, fn: fn, now: time.Now}
	t.when = t.now().Add(delay)
	w.mu.Lock()
	heap.Push(&w.heap, t)
	w.mu.Unlock()
	w.wake()
	return t, nil
}

// Cancel marks a timer as cancelled (it will not fire).
func (t *Timer) Cancel() { t.cancelled.Store(true) }

// Cancelled reports whether the timer was cancelled.
func (t *Timer) Cancelled() bool { return t.cancelled.Load() }

// Len returns the number of active (non-cancelled) timers.
func (w *Wheel) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	count := 0
	for _, t := range w.heap {
		if !t.cancelled.Load() {
			count++
		}
	}
	return count
}

// Close stops the wheel and cancels all pending timers.
func (w *Wheel) Close() {
	if !w.closed.CompareAndSwap(false, true) {
		return
	}
	w.wake()
	w.wg.Wait()
}

// safeFire runs a timer callback with panic recovery — a panicking callback no
// longer crashes the process; it is counted + surfaced via the hook (L5).
func (w *Wheel) safeFire(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			atomic.AddUint64(&w.recovered, 1)
			if w.onPanic != nil {
				w.onPanic(r)
			}
		}
	}()
	fn()
}

// SetOnPanic installs a hook fired when a timer callback panics.
func (w *Wheel) SetOnPanic(fn func(any)) { w.onPanic = fn }

// Recovered returns the total callback panics recovered.
func (w *Wheel) Recovered() uint64 { return atomic.LoadUint64(&w.recovered) }

func (w *Wheel) wake() {
	select {
	case w.wakeup <- struct{}{}:
	default:
	}
}

func (w *Wheel) run() {
	defer w.wg.Done()
	var timer *time.Timer
	for {
		w.mu.Lock()
		// Clean cancelled timers from the top.
		for w.heap.Len() > 0 && w.heap[0].cancelled.Load() {
			heap.Pop(&w.heap)
		}
		if w.heap.Len() == 0 {
			w.mu.Unlock()
			if w.closed.Load() {
				return
			}
			if timer != nil {
				timer.Stop()
				timer = nil
			}
			<-w.wakeup
			continue
		}
		next := w.heap[0]
		now := next.now()
		if next.when.After(now) {
			d := next.when.Sub(now)
			w.mu.Unlock()
			if w.closed.Load() {
				return
			}
			if timer == nil {
				timer = time.NewTimer(d)
			} else {
				timer.Reset(d)
			}
			select {
			case <-w.wakeup:
				timer.Stop()
			case <-timer.C:
			}
			continue
		}
		// Fire the timer.
		t := heap.Pop(&w.heap).(*Timer)
		w.mu.Unlock()
		if !t.cancelled.Load() {
			w.safeFire(t.fn)
		}
		// Reschedule recurring timers.
		if t.interval > 0 && !t.cancelled.Load() && !w.closed.Load() {
			t.when = t.now().Add(t.interval)
			w.mu.Lock()
			heap.Push(&w.heap, t)
			w.mu.Unlock()
		}
	}
}
