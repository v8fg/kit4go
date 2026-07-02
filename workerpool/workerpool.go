// Package workerpool is a bounded worker pool: N goroutines process jobs from a
// queue, with backpressure (Submit blocks when full), graceful shutdown (drain),
// and optional result collection. Pure standard library.
//
// Ad-tech uses: bounded request processing (cap concurrent bid evaluations),
// bulk creative loading, batch event ingestion, parallel HTTP fan-out to SSPs,
// and anywhere you need "N workers, M-deep queue" without hand-rolling
// goroutine lifecycle.
package workerpool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// ErrClosed is returned by Submit after Close.
var ErrClosed = errors.New("workerpool: closed")

// Job is a unit of work. The function receives the worker context (cancelled on
// shutdown) and returns a result or error.
type Job[T any] func(ctx context.Context) (T, error)

// Result holds the outcome of a job.
type Result[T any] struct {
	Value T
	Err   error
}

// Pool runs jobs on N workers with a bounded queue.
type Pool[T any] struct {
	queue    chan jobWrap[T]
	done     chan struct{} // closed by Close: workers exit, Submit rejects (queue is never closed → no send-on-closed panic)
	workers  int
	wg       sync.WaitGroup
	closed   atomic.Bool
	results  chan Result[T]
	collectW bool
}

type jobWrap[T any] struct {
	fn  Job[T]
	ctx context.Context
}

// Option configures the Pool.
type Option[T any] func(*Pool[T])

// WithQueueSize sets the queue capacity (default = workers). A larger queue
// decouples producers from slow workers at the cost of memory.
func WithQueueSize[T any](n int) Option[T] {
	return func(p *Pool[T]) {
		if n > 0 {
			p.queue = make(chan jobWrap[T], n)
		}
	}
}

// WithResults enables result collection via the Results channel. Without this,
// jobs run fire-and-forget (errors are silently dropped).
func WithResults[T any](buffer int) Option[T] {
	return func(p *Pool[T]) {
		if buffer < 0 {
			buffer = 0
		}
		p.results = make(chan Result[T], buffer)
		p.collectW = true
	}
}

// New builds a pool with n workers. Panics if n <= 0.
func New[T any](workers int, opts ...Option[T]) *Pool[T] {
	if workers <= 0 {
		panic("workerpool: workers must be > 0")
	}
	p := &Pool[T]{workers: workers, done: make(chan struct{})}
	for _, opt := range opts {
		opt(p)
	}
	if p.queue == nil {
		p.queue = make(chan jobWrap[T], workers)
	}
	p.start()
	return p
}

func (p *Pool[T]) start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

func (p *Pool[T]) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.done:
			// Shutdown: process jobs still queued at this instant, then exit.
			// Jobs submitted after done is closed may be lost — standard
			// close-race semantics. The queue is NEVER closed, so a racing
			// Submit can never panic on send-to-closed-channel.
			p.drainQueue()
			return
		case jw, ok := <-p.queue:
			if !ok {
				return
			}
			p.runJob(jw)
		}
	}
}

// drainQueue processes remaining queued jobs non-blocking, then returns.
func (p *Pool[T]) drainQueue() {
	for {
		select {
		case jw, ok := <-p.queue:
			if !ok {
				return
			}
			p.runJob(jw)
		default:
			return
		}
	}
}

// runJob executes one job and, when result collection is on, forwards its
// result. The result send selects on done so a worker cannot block forever on a
// full, undrained results channel during shutdown — otherwise Close's wg.Wait
// would deadlock.
func (p *Pool[T]) runJob(jw jobWrap[T]) {
	val, err := jw.fn(jw.ctx)
	if !p.collectW {
		return
	}
	select {
	case p.results <- Result[T]{Value: val, Err: err}:
	case <-p.done: // shutdown: drop this result so the worker can exit
	}
}

// Submit enqueues a job. It blocks if the queue is full (backpressure). Returns
// ErrClosed if the pool is shutting down.
func (p *Pool[T]) Submit(ctx context.Context, fn Job[T]) error {
	if p.closed.Load() {
		return ErrClosed
	}
	select {
	case p.queue <- jobWrap[T]{fn: fn, ctx: ctx}:
		return nil
	case <-p.done:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TrySubmit enqueues without blocking. Returns false if the queue is full or
// closed.
func (p *Pool[T]) TrySubmit(ctx context.Context, fn Job[T]) bool {
	if p.closed.Load() {
		return false
	}
	select {
	case p.queue <- jobWrap[T]{fn: fn, ctx: ctx}:
		return true
	case <-p.done:
		return false
	default:
		return false
	}
}

// Results returns the result channel (nil if WithResults was not used).
func (p *Pool[T]) Results() <-chan Result[T] { return p.results }

// Workers returns the configured worker count.
func (p *Pool[T]) Workers() int { return p.workers }

// Close stops accepting new jobs, drains the queue, and waits for all workers
// to finish. Safe to call multiple times.
func (p *Pool[T]) Close() {
	if !p.closed.CompareAndSwap(false, true) {
		return
	}
	close(p.done) // workers exit after draining queued jobs; Submit/TrySubmit reject
	p.wg.Wait()
	if p.collectW {
		close(p.results)
	}
}
