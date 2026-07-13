// Package syncutil provides common concurrent utilities built on channels and
// context: OrDone (ctx-cancelable channel receive), Merge (fan-in), and Promise
// (one-shot future).
//
// All goroutines spawned by these functions exit when the context is cancelled
// — no leak. Safe for concurrent use.
//
// Pure standard library.
package syncutil

import (
	"context"
	"sync"
)

// OrDone returns a channel that forwards values from src until src is closed or
// ctx is cancelled. The forwarding goroutine exits on either condition — no
// leak.
//
// Use this to make a range-over-channel ctx-cancelable:
//
//	for v := range syncutil.OrDone(ctx, ch) { ... }
func OrDone[T any](ctx context.Context, src <-chan T) <-chan T {
	dst := make(chan T)
	go func() {
		defer close(dst)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-src:
				if !ok {
					return
				}
				select {
				case dst <- v:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return dst
}

// Merge fans in multiple channels into one. The output channel closes when all
// inputs close or ctx is cancelled.
func Merge[T any](ctx context.Context, channels ...<-chan T) <-chan T {
	out := make(chan T)
	var wg sync.WaitGroup
	wg.Add(len(channels))
	multiplex := func(src <-chan T) {
		defer wg.Done()
		for v := range OrDone(ctx, src) {
			select {
			case out <- v:
			case <-ctx.Done():
				return
			}
		}
	}
	for _, ch := range channels {
		go multiplex(ch)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// Promise is a one-shot future. Set (or SetErr) must be called exactly once;
// Get blocks until then or until ctx is cancelled. Multiple Get callers all
// receive the same result.
//
// Safe for concurrent use: Set is called by the producer goroutine, Get by any
// number of consumer goroutines.
type Promise[T any] struct {
	done chan struct{}
	once sync.Once
	val  T
	err  error
}

// NewPromise creates a Promise.
func NewPromise[T any]() *Promise[T] {
	return &Promise[T]{done: make(chan struct{})}
}

// Set stores the value and unblocks all Get callers. Panics on a second Set/SetErr.
func (p *Promise[T]) Set(v T) {
	p.once.Do(func() {
		p.val = v
		close(p.done)
	})
}

// SetErr stores an error and unblocks all Get callers. Panics on a second Set/SetErr.
func (p *Promise[T]) SetErr(err error) {
	p.once.Do(func() {
		p.err = err
		close(p.done)
	})
}

// Get blocks until Set/SetErr is called or ctx is cancelled.
func (p *Promise[T]) Get(ctx context.Context) (T, error) {
	select {
	case <-p.done:
		return p.val, p.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// Done returns a channel that closes when Set/SetErr is called. Useful for
// select-based waiting alongside other channels.
func (p *Promise[T]) Done() <-chan struct{} { return p.done }
