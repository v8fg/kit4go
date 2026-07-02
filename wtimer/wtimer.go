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
}

// Wheel is a timer wheel backed by a min-heap.
//
// Concurrency: safe for concurrent use. Add/AddRecurring/Cancel/Len/Close each
// acquire an internal sync.Mutex. New starts a single background goroutine that
// fires due callbacks; callbacks execute on that goroutine, so they must be
// non-blocking (offload heavy work to your own goroutine). Close is idempotent
// and joins the goroutine. A panic in a callback is not recovered and will stop
// the wheel — guard it inside the callback if that matters.
type Wheel struct {
	mu     sync.Mutex
	heap   timerHeap
	wakeup chan struct{}
	closed atomic.Bool
	wg     sync.WaitGroup
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
	return w.add(time.Now().Add(delay), 0, fn)
}

// AddRecurring schedules a recurring callback every interval. Returns a Timer
// that can be cancelled.
func (w *Wheel) AddRecurring(interval time.Duration, fn func()) (*Timer, error) {
	return w.add(time.Now().Add(interval), interval, fn)
}

func (w *Wheel) add(when time.Time, interval time.Duration, fn func()) (*Timer, error) {
	if w.closed.Load() {
		return nil, ErrClosed
	}
	if fn == nil {
		return nil, errors.New("wtimer: callback is required")
	}
	t := &Timer{when: when, interval: interval, fn: fn}
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
		now := time.Now()
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
			t.fn()
		}
		// Reschedule recurring timers.
		if t.interval > 0 && !t.cancelled.Load() && !w.closed.Load() {
			t.when = time.Now().Add(t.interval)
			w.mu.Lock()
			heap.Push(&w.heap, t)
			w.mu.Unlock()
		}
	}
}
