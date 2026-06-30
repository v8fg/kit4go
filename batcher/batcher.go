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
}

// Option configures a Batcher.
type Option[T any] func(*Batcher[T])

// WithBufferSize sets the input channel capacity (default = maxSize). A larger
// buffer decouples the producer from a momentarily-slow flusher at the cost of
// memory.
func WithBufferSize[T any](n int) Option[T] {
	return func(b *Batcher[T]) { b.bufferCap = n }
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

// Add enqueues item, blocking until accepted (backpressure). Returns false (and
// does not enqueue) after Close.
func (b *Batcher[T]) Add(item T) bool {
	// Fast-path check: once Close has been called, reject without racing the
	// buffered send. (Close fully completes — collector exited — before this
	// flag would be observed, so done is definitively closed.)
	select {
	case <-b.done:
		return false
	default:
	}
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
func (b *Batcher[T]) Close() error {
	b.once.Do(func() { close(b.done) })
	<-b.closed
	return nil
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
		b.flushFn(out)
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
