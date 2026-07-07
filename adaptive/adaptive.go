// Package adaptive implements a CPU-aware worker pool whose primary goal is to
// NOT saturate the host CPU.
//
// Unlike a fixed-size pool that maximizes throughput, adaptive scales the worker
// count DOWN when host CPU usage exceeds a target (default 0.75) so that
// latency-critical paths keep their headroom — e.g. an RTB bidder with a ~100ms
// timeout must not be starved by background work running in the same pool. When
// there is spare CPU and a queued backlog, it scales back UP, bounded by
// MinWorkers and MaxWorkers.
//
// The do-no-harm contract: this pool treats CPU headroom as a shared resource
// and yields it when contested. It is intentionally NOT a throughput-maximizing
// pool. Use workerpool for fixed-concurrency throughput; use adaptive when the
// workload is opportunistic and must back off the host under load.
//
// Worker add/remove is safe under concurrent Submit. Shrink signals a worker to
// exit after its current job (no hard preemption), so an in-flight job always
// runs to completion.
package adaptive

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

// Sentinel errors. Use errors.Is to test.
var (
	// ErrClosed is returned by Submit after Close.
	ErrClosed = errors.New("adaptive: pool closed")
	// ErrFull is returned by Submit when the job queue is full (backpressure).
	ErrFull = errors.New("adaptive: queue full")
)

// LoadMonitor reports the host CPU load as a fraction in [0,1]. The default
// implementation samples gopsutil cpu.Percent over SampleInterval. Tests inject
// a fake to drive the autoscaler deterministically (real CPU is flaky).
type LoadMonitor interface {
	CPU() (float64, error)
}

// Compile-time interface assertions: guard that the concrete implementations
// stay in sync with the interface contract.
var (
	_ LoadMonitor = (*gopsutilMonitor)(nil)
)

// gopsutilMonitor is the default LoadMonitor. It samples cpu.Percent over the
// given interval (blocking for that interval on each call).
type gopsutilMonitor struct {
	interval time.Duration
}

// cpuPercent is the gopsutil sampling function, kept as a package var so tests
// can swap it for a fake (the real call reads kernel counters and is
// nondeterministic). Defaults to cpu.Percent with perCPU=false (one overall
// sample in [0,100]).
var cpuPercent = cpu.Percent

func (m gopsutilMonitor) CPU() (float64, error) {
	// cpu.Percent blocks for `interval` measuring the delta, then returns the
	// busy fraction as a percentage in [0,100].
	pct, err := cpuPercent(m.interval, false)
	if err != nil {
		return 0, fmt.Errorf("adaptive: cpu sample: %w", err)
	}
	if len(pct) == 0 {
		return 0, nil
	}
	return pct[0] / 100.0, nil
}

// Options holds the pool configuration. Zero values fall back to defaults in
// [New]: MinWorkers=1, MaxWorkers=runtime.NumCPU()*2, TargetCPU=0.75,
// SampleInterval=1s, QueueSize=MaxWorkers.
type Options[Job any] struct {
	MinWorkers     int
	MaxWorkers     int
	TargetCPU      float64
	SampleInterval time.Duration
	QueueSize      int
	Work           func(Job)
	monitor        LoadMonitor // set by WithLoadMonitor; nil => gopsutil default
}

// Pool is a CPU-aware autoscaling worker pool over an arbitrary job type.
//
// Submit enqueues jobs on a bounded queue; a pool of workers drains it. An
// autoscaler goroutine samples host CPU every SampleInterval and adjusts the
// worker count toward the TargetCPU ceiling, clamped to [MinWorkers,MaxWorkers].
type Pool[Job any] struct {
	work  func(Job)
	queue chan Job

	minWorkers int
	maxWorkers int
	targetCPU  float64
	interval   time.Duration

	monitor LoadMonitor

	workers atomic.Int32 // current live worker count

	stopChMu sync.Mutex
	stopChs  []chan struct{} // one signal channel per live worker; closing one shrinks by one

	closed atomic.Bool
	done   chan struct{}  // closed by Close: autoscaler + workers exit, Submit rejects
	wg     sync.WaitGroup // workers
	scaler sync.WaitGroup // autoscaler goroutine

	closeOnce sync.Once
}

// Option configures the Pool.
type Option[Job any] func(*Options[Job])

// WithMinWorkers sets the minimum worker count (default 1). Must be >= 1 and
// <= MaxWorkers.
func WithMinWorkers[Job any](n int) Option[Job] {
	return func(o *Options[Job]) { o.MinWorkers = n }
}

// WithMaxWorkers sets the maximum worker count
// (default runtime.NumCPU()*2). Must be >= MinWorkers.
func WithMaxWorkers[Job any](n int) Option[Job] {
	return func(o *Options[Job]) { o.MaxWorkers = n }
}

// WithTargetCPU sets the CPU fraction ceiling (default 0.75) above which the
// pool shrinks. Must be in (0,1).
func WithTargetCPU[Job any](f float64) Option[Job] {
	return func(o *Options[Job]) { o.TargetCPU = f }
}

// WithSampleInterval sets the autoscaler tick / CPU sample interval
// (default 1s). Must be > 0.
func WithSampleInterval[Job any](d time.Duration) Option[Job] {
	return func(o *Options[Job]) { o.SampleInterval = d }
}

// WithQueueSize sets the job queue capacity (default MaxWorkers). A larger queue
// absorbs bursts at the cost of memory and latency; a full queue returns
// [ErrFull] from Submit (backpressure).
func WithQueueSize[Job any](n int) Option[Job] {
	return func(o *Options[Job]) { o.QueueSize = n }
}

// WithLoadMonitor injects a LoadMonitor (tests pass a fake). Without it, the
// pool samples gopsutil cpu.Percent over SampleInterval.
func WithLoadMonitor[Job any](m LoadMonitor) Option[Job] {
	return func(o *Options[Job]) { o.monitor = m }
}

// New builds an adaptive pool, starts MinWorkers, and launches the autoscaler
// goroutine. work must be non-nil.
//
// Returns an error (rather than panicking) for invalid configuration:
// MinWorkers < 1, MaxWorkers < MinWorkers, TargetCPU not in (0,1),
// SampleInterval <= 0, or a nil work function.
func New[Job any](work func(Job), opts ...Option[Job]) (*Pool[Job], error) {
	o := Options[Job]{
		MinWorkers:     1,
		MaxWorkers:     runtime.NumCPU() * 2,
		TargetCPU:      0.75,
		SampleInterval: time.Second,
		Work:           work,
	}
	for _, opt := range opts {
		opt(&o)
	}

	if o.Work == nil {
		return nil, errors.New("adaptive: work function must be non-nil")
	}
	if o.MinWorkers < 1 {
		return nil, fmt.Errorf("adaptive: MinWorkers must be >= 1, got %d", o.MinWorkers)
	}
	if o.MaxWorkers < o.MinWorkers {
		return nil, fmt.Errorf("adaptive: MaxWorkers (%d) must be >= MinWorkers (%d)", o.MaxWorkers, o.MinWorkers)
	}
	if o.TargetCPU <= 0 || o.TargetCPU >= 1 {
		return nil, fmt.Errorf("adaptive: TargetCPU must be in (0,1), got %v", o.TargetCPU)
	}
	if o.SampleInterval <= 0 {
		return nil, fmt.Errorf("adaptive: SampleInterval must be > 0, got %v", o.SampleInterval)
	}
	if o.QueueSize <= 0 {
		o.QueueSize = o.MaxWorkers
	}

	p := &Pool[Job]{
		work:       o.Work,
		queue:      make(chan Job, o.QueueSize),
		minWorkers: o.MinWorkers,
		maxWorkers: o.MaxWorkers,
		targetCPU:  o.TargetCPU,
		interval:   o.SampleInterval,
		done:       make(chan struct{}),
	}
	if o.monitor != nil {
		p.monitor = o.monitor
	} else {
		p.monitor = gopsutilMonitor{interval: o.SampleInterval}
	}

	// Seed the worker pool at MinWorkers.
	p.grow(o.MinWorkers)

	// Autoscaler.
	p.scaler.Add(1)
	go p.autoscale()

	return p, nil
}

// autoscale ticks every SampleInterval, samples host CPU, and adjusts the worker
// count toward the target ceiling.
//
//   - CPU > TargetCPU: shrink (down to MinWorkers) to free headroom for other
//     paths. This is the do-no-harm behavior: yield CPU when contested.
//   - CPU < TargetCPU and queued backlog: grow (up to MaxWorkers) to drain it.
//   - Otherwise: hold steady.
func (p *Pool[Job]) autoscale() {
	defer p.scaler.Done()
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
		}
		frac, err := p.monitor.CPU()
		if err != nil {
			// A bad sample is not fatal: hold steady and try again next tick.
			continue
		}
		// Re-check done after the (possibly blocking) sample: if Close fired
		// during the sample, exit without acting on a stale decision.
		select {
		case <-p.done:
			return
		default:
		}
		cur := int(p.workers.Load())
		switch {
		case frac > p.targetCPU:
			// Over target: shrink by one toward MinWorkers.
			if cur > p.minWorkers {
				p.shrink()
			}
		case frac < p.targetCPU:
			// Under target: grow only if there is queued backlog (else adding
			// workers wastes goroutines idling on an empty queue).
			if cur < p.maxWorkers && p.hasBacklog() {
				p.grow(1)
			}
		}
	}
}

// hasBacklog reports whether jobs are waiting in the queue.
func (p *Pool[Job]) hasBacklog() bool {
	return len(p.queue) > 0
}

// grow launches n new workers. Caller validates bounds.
func (p *Pool[Job]) grow(n int) {
	for range n {
		if int(p.workers.Load()) >= p.maxWorkers {
			return
		}
		ch := make(chan struct{})
		p.stopChMu.Lock()
		p.stopChs = append(p.stopChs, ch)
		p.stopChMu.Unlock()
		p.workers.Add(1)
		p.wg.Add(1)
		go p.worker(ch)
	}
}

// shrink signals one worker to exit after its current job (no hard preemption).
func (p *Pool[Job]) shrink() {
	p.stopChMu.Lock()
	defer p.stopChMu.Unlock()
	if len(p.stopChs) == 0 {
		return
	}
	// Pop the last registered worker's stop channel and close it. The worker
	// observes the close after finishing its in-flight job and exits cleanly.
	ch := p.stopChs[len(p.stopChs)-1]
	p.stopChs = p.stopChs[:len(p.stopChs)-1]
	close(ch)
}

// worker drains the queue until either the pool shuts down (done) or it is
// signaled to shrink (stopCh). A signaled worker finishes its current job, then
// exits — it does NOT abandon an in-flight job.
func (p *Pool[Job]) worker(stopCh chan struct{}) {
	defer p.wg.Done()
	defer p.workers.Add(-1)
	for {
		select {
		case <-p.done:
			// Pool shutting down: drain remaining queued jobs (non-blocking),
			// then exit. Submit after done is closed is rejected, so the queue
			// is bounded and drains in finite time.
			p.drainQueue()
			return
		case <-stopCh:
			// Shrink signal: exit WITHOUT draining — the autoscaler asked us to
			// back off; remaining jobs stay queued for the surviving workers.
			return
		case j, ok := <-p.queue:
			if !ok {
				return
			}
			p.work(j)
		}
	}
}

// drainQueue processes remaining queued jobs non-blocking, then returns.
func (p *Pool[Job]) drainQueue() {
	for {
		select {
		case j, ok := <-p.queue:
			if !ok {
				return
			}
			p.work(j)
		default:
			return
		}
	}
}

// Submit enqueues a job. It returns [ErrClosed] if the pool is shutting down,
// [ErrFull] if the queue is full (backpressure), or blocks up to ctx deadline.
//
// Backpressure model: Submit blocks while the queue is full (bounded by ctx).
// For non-blocking submit, use [Pool.TrySubmit].
func (p *Pool[Job]) Submit(ctx context.Context, j Job) error {
	if p.closed.Load() {
		return ErrClosed
	}
	select {
	case p.queue <- j:
		return nil
	case <-p.done:
		return ErrClosed
	case <-ctx.Done():
		// Context cancelled/expired while waiting on a full queue: return the
		// context error (mirrors workerpool/semaphore) so callers can tell
		// cancellation apart from genuine backpressure via errors.Is(err, ErrFull).
		return ctx.Err()
	}
}

// TrySubmit enqueues without blocking. Returns (true, nil) on success; false
// (with ErrFull or ErrClosed) if the queue is full or the pool is closed.
func (p *Pool[Job]) TrySubmit(j Job) (bool, error) {
	if p.closed.Load() {
		return false, ErrClosed
	}
	select {
	case p.queue <- j:
		return true, nil
	case <-p.done:
		return false, ErrClosed
	default:
		return false, ErrFull
	}
}

// Workers returns the current live worker count (atomic snapshot). It may lag
// the autoscaler's most recent decision by up to SampleInterval.
func (p *Pool[Job]) Workers() int { return int(p.workers.Load()) }

// Close stops the autoscaler, rejects new submissions, and drains remaining
// queued jobs with the surviving workers, then waits for them to exit. Bounded:
// drains only jobs already queued at close time (Submit after Close is
// rejected). Idempotent and safe to call concurrently.
//
// Close waits for in-flight jobs to finish, so a Work func that blocks
// indefinitely (stuck network call, held lock, infinite loop) will hang Close.
// Work MUST be non-blocking or honor a context/deadline internally — Go cannot
// preempt it. Close's wait bound is: (in-flight jobs) x max(Work duration).
func (p *Pool[Job]) Close() error {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		close(p.done) // autoscaler exits; workers drain+exit; Submit rejects
	})
	p.scaler.Wait() // autoscaler is down before workers, so no more grow/shrink races
	p.wg.Wait()     // workers finish draining and exit
	return nil
}
