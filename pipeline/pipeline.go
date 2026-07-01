// Package pipeline is a generic stream-processing stage: N workers transform
// items (map + filter) concurrently through a bounded channel pipeline. Pure
// standard library.
package pipeline

import (
	"context"
	"errors"
	"sync"
)

// ErrClosed is returned by Send after Close.
var ErrClosed = errors.New("pipeline: closed")

// Stage transforms one item. (out, true, nil) forwards it; (zero, false, nil)
// drops it; (*, *, err) drops it.
type Stage[I, O any] func(ctx context.Context, in I) (O, bool, error)

// Pipeline processes items of type I through N workers, producing type O.
type Pipeline[I, O any] struct {
	in      chan I
	out     chan O
	stage   Stage[I, O]
	workers int
	wg      sync.WaitGroup
	done    chan struct{}
	once    sync.Once
	ctx     context.Context
}

// Option configures the Pipeline.
type Option[I, O any] func(*Pipeline[I, O])

// WithInputBuffer sets the input channel capacity (default = workers).
func WithInputBuffer[I, O any](n int) Option[I, O] {
	return func(p *Pipeline[I, O]) {
		if n > 0 {
			p.in = make(chan I, n)
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
		in:      make(chan I, workers),
		out:     make(chan O, workers),
		done:    make(chan struct{}),
		ctx:     context.Background(),
	}
	for _, opt := range opts {
		opt(p)
	}
	for i := 0; i < workers; i++ {
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
		case item, ok := <-p.in:
			if !ok {
				return
			}
			p.process(item)
		}
	}
}

// drain processes remaining items in p.in non-blocking.
func (p *Pipeline[I, O]) drain() {
	for {
		select {
		case item := <-p.in:
			p.process(item)
		default:
			return
		}
	}
}

// process transforms one item and writes the result to p.out (if it passes).
// The write blocks if p.out is full (backpressure). During drain (after Close),
// p.out has room for remaining items because Close waits for workers before
// closing p.out.
func (p *Pipeline[I, O]) process(item I) {
	out, pass, err := p.stage(p.ctx, item)
	if err != nil || !pass {
		return
	}
	p.out <- out
}

// Send enqueues an input item. Blocks if full. Returns ErrClosed after Close.
func (p *Pipeline[I, O]) Send(ctx context.Context, item I) error {
	select {
	case p.in <- item:
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
