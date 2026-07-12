// Package batcher is a generic batch coalescer: it collects items and flushes
// them in batches via a caller-supplied callback, triggered by size (N items),
// time (interval), or an explicit Flush/Close.
//
// Add provides backpressure — it blocks until the item is accepted by the
// bounded input buffer, so a slow flusher naturally slows the producer. Shutdown
// is safe: Close signals the collector, which drains in-flight items and flushes
// the remainder; Add after Close returns false without panicking (the input
// channel is never closed).
//
// Pure standard library. Ad-tech / IoT / live-streaming / push uses: pixel and
// beacon batching, sensor aggregation, bulk DB inserts, notification batching.
package batcher

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrClosed is returned by Add/Flush after Close.
var ErrClosed = errors.New("batcher: closed")

// Batcher collects items of type T and flushes batches via FlushFn.
type Batcher[T any] struct {
	flushFn   func([]T)
	maxSize   int
	bufferCap int

	in      chan T
	done    chan struct{}      // closed on Close -> collector drains and exits
	flushCh chan chan struct{} // manual flush requests (each carries its ack)
	closed  chan struct{}      // closed when the collector has fully stopped
	once    sync.Once

	// recovered counts flushFn panics recovered across every flush site
	// (size/timer/Close/Flush/final-straggler). The collector goroutine owns the
	// flush path, so an un-recovered panic would abort the collector — Close then
	// deadlocks forever on <-b.closed. Recovering keeps the collector alive (L1),
	// makes the panic observable (L5), and lets close(b.closed) run so Close stays
	// bounded (L6). onPanic is an optional hook fired on each recovery.
	recovered atomic.Uint64
	onPanic   atomic.Pointer[func(any)]

	// mu serializes Add's send with Close's final drain. Add holds the read lock
	// during the buffered send; Close holds the write lock while draining
	// stragglers, so an Add that raced the collector's exit cannot strand an item.
	mu sync.RWMutex
}

// Option configures a Batcher.
type Option[T any] func(*Batcher[T])

// WithBufferSize sets the input channel capacity (default = maxSize). A larger
// buffer decouples the producer from a momentarily-slow flusher at the cost of
// memory. n is clamped to >= 0 (0 = unbuffered); a negative value is ignored,
// since make(chan, n) would panic on a negative buffer.
func WithBufferSize[T any](n int) Option[T] {
	return func(b *Batcher[T]) {
		if n >= 0 {
			b.bufferCap = n
		}
	}
}

// New builds a batcher that flushes when it accumulates maxSize items, every
// interval, or on Flush/Close. interval <= 0 disables the time trigger (size +
// manual only). The flush callback receives each batch once; it must not retain
// the slice past return unless it copies it. Panics if maxSize <= 0.
func New[T any](maxSize int, interval time.Duration, flush func([]T), opts ...Option[T]) *Batcher[T] {
	if maxSize <= 0 {
		panic("batcher: maxSize must be > 0")
	}
	if flush == nil {
		panic("batcher: flush callback is required")
	}
	b := &Batcher[T]{
		flushFn:   flush,
		maxSize:   maxSize,
		bufferCap: maxSize,
		in:        nil,
		done:      make(chan struct{}),
		flushCh:   make(chan chan struct{}, 1),
		closed:    make(chan struct{}),
	}
	for _, opt := range opts {
		opt(b)
	}
	b.in = make(chan T, b.bufferCap)
	go b.run(interval)
	return b
}

// safeFlush runs flushFn with panic recovery. A panicking flush is counted in
// recovered and surfaced via the onPanic hook (if set), but never re-panicked —
// so a buggy flush cannot kill the collector goroutine (which would deadlock
// Close on <-b.closed) or abort Close's straggler drain. Mirrors workerpool's
// safeCall / signalbus invoke. buf is always cleared by the caller regardless of
// a panic, so no batch is double-flushed.
func (b *Batcher[T]) safeFlush(buf []T) {
	defer func() {
		if r := recover(); r != nil {
			b.recovered.Add(1)
			if hook := b.onPanic.Load(); hook != nil {
				(*hook)(r)
			}
		}
	}()
	b.flushFn(buf)
}

// SetOnPanic installs a hook fired (non-blocking) whenever flushFn panics and is
// recovered. The hook runs on the collector or Close goroutine — the flushing
// goroutine — so it must not block. Set to nil to disable.
func (b *Batcher[T]) SetOnPanic(fn func(any)) {
	if fn == nil {
		b.onPanic.Store(nil)
		return
	}
	b.onPanic.Store(&fn)
}

// Recovered returns the total number of flushFn panics recovered since the
// batcher was created.
func (b *Batcher[T]) Recovered() uint64 { return b.recovered.Load() }

// Add enqueues item, blocking until accepted (backpressure). Returns false (and
// does not enqueue) after Close.
func (b *Batcher[T]) Add(item T) bool {
	// Fast-path check: once Close has been called, reject without racing the
	// buffered send.
	select {
	case <-b.done:
		return false
	default:
	}
	// Hold the read lock during the send so Close's final drain (write lock)
	// cannot miss this item: it waits for in-flight Adds to resolve, then drains.
	b.mu.RLock()
	defer b.mu.RUnlock()
	select {
	case b.in <- item:
		return true
	case <-b.done:
		return false
	}
}

// Flush triggers an immediate flush of the currently-buffered items and returns
// once that flush has run. No-op (returns) after Close.
func (b *Batcher[T]) Flush() {
	ack := make(chan struct{})
	select {
	case b.flushCh <- ack:
	case <-b.done:
		return
	}
	select {
	case <-ack:
	case <-b.done:
	}
}

// Close stops accepting items, drains the in-flight buffer, flushes the
// remainder, and waits for the collector to exit. Safe to call multiple times.
//
// Close is bounded only if flushFn is. The collector flushes the remainder
// synchronously before exiting, so a flushFn that blocks on a stalled
// downstream (a wedged broker/DB) blocks Close in turn — Close waits on the
// collector via <-b.closed. Keep flushFn non-blocking and give it its own
// deadline/context so graceful shutdown cannot hang (L6).
func (b *Batcher[T]) Close() error {
	b.once.Do(func() { close(b.done) })
	<-b.closed
	// Final drain: an Add that raced the collector's exit (its select picked the
	// buffered send over <-done) left items in b.in that the collector's drain
	// missed. Take the write lock so any in-flight Add finishes first — after
	// this no further Add can send (done is closed, rejected on the fast-path) —
	// then drain and flush the stragglers so Add's contract (true => flushed) holds.
	b.mu.Lock()
	defer b.mu.Unlock()
	buf := make([]T, 0, b.maxSize)
	for {
		select {
		case item := <-b.in:
			buf = append(buf, item)
			if len(buf) >= b.maxSize {
				b.safeFlush(buf)
				buf = make([]T, 0, b.maxSize)
			}
		default:
			if len(buf) > 0 {
				b.safeFlush(buf)
			}
			return nil
		}
	}
}

// run is the collector goroutine.
func (b *Batcher[T]) run(interval time.Duration) {
	defer close(b.closed)
	buf := make([]T, 0, b.maxSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		out := buf
		b.safeFlush(out)
		buf = make([]T, 0, b.maxSize)
	}
	addAndMaybeFlush := func(item T) {
		buf = append(buf, item)
		if len(buf) >= b.maxSize {
			flush()
		}
	}

	var tickerC <-chan time.Time
	if interval > 0 {
		t := time.NewTicker(interval)
		defer t.Stop()
		tickerC = t.C
	}

	for {
		select {
		case <-b.done:
			// Drain anything already accepted, then flush the remainder.
			for {
				select {
				case item := <-b.in:
					addAndMaybeFlush(item)
				default:
					flush()
					return
				}
			}
		case item := <-b.in:
			addAndMaybeFlush(item)
		case <-tickerC:
			flush()
		case ack := <-b.flushCh:
			// Drain any items still queued in the input channel first, so Flush
			// captures everything added before this call (Add returns as soon as
			// the item is in the buffered channel, not once the collector read it).
			drain := true
			for drain {
				select {
				case item := <-b.in:
					addAndMaybeFlush(item)
				default:
					drain = false
				}
			}
			flush()
			close(ack)
		}
	}
}
