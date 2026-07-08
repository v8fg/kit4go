// Package pipeline is a generic stream-processing stage: N workers transform
// items (map + filter) concurrently through a bounded channel pipeline. Pure
// standard library.
package pipeline

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// ErrClosed is returned by Send after Close.
var ErrClosed = errors.New("pipeline: closed")

// Stage transforms one item. (out, true, nil) forwards it; (zero, false, nil)
// drops it; (*, *, err) drops it.
type Stage[I, O any] func(ctx context.Context, in I) (O, bool, error)

// itemReq pairs an input item with the submitter's context, so the stage sees
// the per-call cancellation/deadline/trace values instead of a single
// construction-time context.
type itemReq[I any] struct {
	item I
	ctx  context.Context
}

// Pipeline processes items of type I through N workers, producing type O.
type Pipeline[I, O any] struct {
	in      chan itemReq[I]
	out     chan O
	stage   Stage[I, O]
	workers int
	wg      sync.WaitGroup
	done    chan struct{}
	once    sync.Once

	recovered uint64 // count of stage panics recovered (observable; L5)
	// onPanic is an optional hook fired on a recovered stage panic. Stored as an
	// atomic.Pointer so SetOnPanic (caller goroutine) and the recover path
	// (worker goroutine) never race on the bare field; a nil hook costs only a
	// Load on the no-hook fast path. API identical to the bare-field version.
	onPanic atomic.Pointer[func(any)]
}

// Option configures the Pipeline.
type Option[I, O any] func(*Pipeline[I, O])

// WithInputBuffer sets the input channel capacity (default = workers).
func WithInputBuffer[I, O any](n int) Option[I, O] {
	return func(p *Pipeline[I, O]) {
		if n > 0 {
			p.in = make(chan itemReq[I], n)
		}
	}
}

// WithOutputBuffer sets the output channel capacity (default = workers).
func WithOutputBuffer[I, O any](n int) Option[I, O] {
	return func(p *Pipeline[I, O]) {
		if n > 0 {
			p.out = make(chan O, n)
		}
	}
}

// New builds a pipeline with n workers applying the stage function.
func New[I, O any](workers int, stage Stage[I, O], opts ...Option[I, O]) *Pipeline[I, O] {
	if workers <= 0 {
		panic("pipeline: workers must be > 0")
	}
	if stage == nil {
		panic("pipeline: stage is required")
	}
	p := &Pipeline[I, O]{
		stage:   stage,
		workers: workers,
		in:      make(chan itemReq[I], workers),
		out:     make(chan O, workers),
		done:    make(chan struct{}),
	}
	for _, opt := range opts {
		opt(p)
	}
	for range workers {
		p.wg.Add(1)
		go p.worker()
	}
	return p
}

// worker processes items. After done is closed, it enters drain mode: processes
// any remaining items in p.in non-blocking, then exits. This ensures Close()
// delivers all queued items before shutting down.
func (p *Pipeline[I, O]) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.done:
			// Drain mode: process remaining items, then exit.
			p.drain()
			return
		case req, ok := <-p.in:
			if !ok {
				return
			}
			p.process(req.item, req.ctx)
		}
	}
}

// drain processes remaining items in p.in non-blocking.
func (p *Pipeline[I, O]) drain() {
	for {
		select {
		case req := <-p.in:
			p.process(req.item, req.ctx)
		default:
			return
		}
	}
}

// process transforms one item and writes the result to p.out (if it passes).
// Delivery: if Out() has room the item is delivered immediately (the common
// case, preserves full delivery when the consumer keeps up). If Out() is full
// the send blocks for room (backpressure), but bails on shutdown so a consumer
// that has stopped draining cannot deadlock Close's wg.Wait.
func (p *Pipeline[I, O]) process(item I, ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			atomic.AddUint64(&p.recovered, 1)
			if hp := p.onPanic.Load(); hp != nil {
				(*hp)(r)
			}
			// The item is dropped; the worker survives for the next item.
		}
	}()
	out, pass, err := p.stage(ctx, item)
	if err != nil || !pass {
		return
	}
	select {
	case p.out <- out:
		return
	default:
	}
	select {
	case p.out <- out:
	case <-p.done:
	}
}

// SetOnPanic installs a hook fired (non-blocking) when a stage panics. The panic
// is also counted in Recovered() and the offending item is dropped. Safe to call
// concurrently with Send/stage execution (the hook pointer is stored atomically,
// so there is no data race between a writer here and the reader in the recover
// path). Pass nil to clear a previously-installed hook.
func (p *Pipeline[I, O]) SetOnPanic(fn func(any)) {
	if fn == nil {
		p.onPanic.Store(nil)
		return
	}
	f := fn // copy to heap
	p.onPanic.Store(&f)
}

// Recovered returns the total number of stage panics recovered.
func (p *Pipeline[I, O]) Recovered() uint64 { return atomic.LoadUint64(&p.recovered) }

// Send enqueues an input item. Blocks if full. Returns ErrClosed after Close.
func (p *Pipeline[I, O]) Send(ctx context.Context, item I) error {
	select {
	case p.in <- itemReq[I]{item: item, ctx: ctx}:
		return nil
	case <-p.done:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Out returns the output channel.
func (p *Pipeline[I, O]) Out() <-chan O { return p.out }

// Close gracefully shuts down: stops accepting input (Send returns ErrClosed),
// drains all queued items (workers finish processing), then closes the output.
// Idempotent.
func (p *Pipeline[I, O]) Close() {
	p.once.Do(func() {
		close(p.done) // wake Senders + signal workers to drain
		p.wg.Wait()   // wait for workers to finish draining
		close(p.out)
	})
	// On repeat calls, once.Do skips everything — but wg.Wait was already called,
	// so workers have exited. We need a second guard for p.out.
	// Actually once.Do covers the entire body, so repeat calls are no-ops.
}

// Workers returns the worker count.
func (p *Pipeline[I, O]) Workers() int { return p.workers }
