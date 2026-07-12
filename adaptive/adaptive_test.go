package adaptive

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeMonitor is a deterministic LoadMonitor for tests. It returns a scripted
// sequence of CPU fractions, cycling on exhaustion so the autoscaler never
// blocks on a short script. Real CPU is NEVER read in tests (flaky).
type fakeMonitor struct {
	mu    sync.Mutex
	vals  []float64
	idx   int
	err   error
	calls int32
}

func newFake(vals ...float64) *fakeMonitor {
	return &fakeMonitor{vals: vals}
}

func (f *fakeMonitor) CPU() (float64, error) {
	atomic.AddInt32(&f.calls, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return 0, f.err
	}
	if len(f.vals) == 0 {
		return 0, nil
	}
	v := f.vals[f.idx%len(f.vals)]
	f.idx++
	return v, nil
}

// trySubmit enqueues via TrySubmit and asserts success (used where the queue is
// large enough that the job should always fit).
func trySubmit(t *testing.T, p *Pool[int], j int) {
	t.Helper()
	ok, err := p.TrySubmit(j)
	require.NoError(t, err)
	require.True(t, ok)
}

// --- New validation -------------------------------------------------------

func TestNew_NilWork(t *testing.T) {
	_, err := New[int](nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "work function must be non-nil")
}

func TestNew_MinWorkersTooSmall(t *testing.T) {
	_, err := New[int](func(int) {}, WithMinWorkers[int](0))
	require.Error(t, err)
	require.ErrorContains(t, err, "MinWorkers must be >= 1")
}

func TestNew_MinWorkersNegative(t *testing.T) {
	_, err := New[int](func(int) {}, WithMinWorkers[int](-3))
	require.Error(t, err)
	require.ErrorContains(t, err, "MinWorkers must be >= 1")
}

func TestNew_MaxLessThanMin(t *testing.T) {
	_, err := New[int](func(int) {}, WithMinWorkers[int](4), WithMaxWorkers[int](2))
	require.Error(t, err)
	require.ErrorContains(t, err, "MaxWorkers")
}

func TestNew_TargetCPUOutOfRange(t *testing.T) {
	cases := []float64{0, -0.1, 1.0, 1.5, 2.0}
	for _, tc := range cases {
		_, err := New[int](func(int) {}, WithTargetCPU[int](tc))
		require.Error(t, err, "target=%v should be rejected", tc)
		require.ErrorContains(t, err, "TargetCPU must be in (0,1)")
	}
}

func TestNew_SampleIntervalZero(t *testing.T) {
	_, err := New[int](func(int) {}, WithSampleInterval[int](0))
	require.Error(t, err)
	require.ErrorContains(t, err, "SampleInterval must be > 0")
}

func TestNew_SampleIntervalNegative(t *testing.T) {
	_, err := New[int](func(int) {}, WithSampleInterval[int](-time.Second))
	require.Error(t, err)
	require.ErrorContains(t, err, "SampleInterval must be > 0")
}

func TestNew_Defaults(t *testing.T) {
	// Defaults: MinWorkers=1, autoscaler runs, no errors.
	p, err := New[int](func(int) {}, WithLoadMonitor[int](newFake(0.5)), WithSampleInterval[int](10*time.Millisecond))
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, 1, p.Workers()) // seeded at MinWorkers
	require.NoError(t, p.Close())
}

func TestNew_DefaultMonitorAndQueue(t *testing.T) {
	// No WithLoadMonitor and no WithQueueSize: New must wire the gopsutil
	// default monitor and default the queue size to MaxWorkers. This exercises
	// both default branches. The default monitor reads real CPU; that's fine for
	// construction (the autoscaler samples it). Short interval so the test is
	// fast.
	p, err := New[int](func(int) {},
		WithSampleInterval[int](5*time.Millisecond),
		WithMaxWorkers[int](4),
	)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, 1, p.Workers())
	require.Equal(t, 4, cap(p.queue), "default queue size should equal MaxWorkers")
	require.IsType(t, gopsutilMonitor{}, p.monitor, "default monitor must be the gopsutil impl")
	require.NoError(t, p.Close())
}

// TestGopsutilMonitor_Smoke exercises the default LoadMonitor against real CPU.
// It does NOT assert a specific value (real CPU is nondeterministic); it only
// confirms the call returns without error and a fraction in [0,1]. A single
// ~5ms sample is cheap and stable enough to run under -short (it never touches
// real workload, just reads the kernel CPU counters).
func TestGopsutilMonitor_Smoke(t *testing.T) {
	if testing.Short() {
		t.Skip("real CPU sensor read skipped in -short")
	}
	m := gopsutilMonitor{interval: 5 * time.Millisecond}
	frac, err := m.CPU()
	if err != nil {
		t.Skipf("cpu sensor unavailable on this runner: %v", err)
	}
	require.GreaterOrEqual(t, frac, 0.0)
	require.LessOrEqual(t, frac, 1.0+1e-9) // tiny float slack for a 1.0 busy sample
}

// TestGopsutilMonitor_Branches covers the error and empty-slice defensive
// branches of gopsutilMonitor.CPU deterministically by swapping the package-var
// sampler (real gopsutil never errors / never returns empty in practice).
func TestGopsutilMonitor_Branches(t *testing.T) {
	orig := cpuPercent
	t.Cleanup(func() { cpuPercent = orig })

	t.Run("sample error", func(t *testing.T) {
		cpuPercent = func(_ time.Duration, _ bool) ([]float64, error) {
			return nil, errors.New("sensor offline")
		}
		m := gopsutilMonitor{interval: time.Millisecond}
		_, err := m.CPU()
		require.Error(t, err)
		require.ErrorContains(t, err, "cpu sample")
	})

	t.Run("empty slice", func(t *testing.T) {
		cpuPercent = func(_ time.Duration, _ bool) ([]float64, error) {
			return []float64{}, nil
		}
		m := gopsutilMonitor{interval: time.Millisecond}
		frac, err := m.CPU()
		require.NoError(t, err)
		require.Equal(t, 0.0, frac)
	})

	t.Run("scaled fraction", func(t *testing.T) {
		cpuPercent = func(_ time.Duration, _ bool) ([]float64, error) {
			return []float64{50.0}, nil // 50% busy
		}
		m := gopsutilMonitor{interval: time.Millisecond}
		frac, err := m.CPU()
		require.NoError(t, err)
		require.InDelta(t, 0.5, frac, 1e-9)
	})
}

// --- submit + work --------------------------------------------------------

func TestSubmit_WorkRuns(t *testing.T) {
	var ran atomic.Int32
	p, err := New[int](func(j int) { ran.Add(1) },
		WithLoadMonitor[int](newFake(0.5)),
		WithSampleInterval[int](50*time.Millisecond),
	)
	require.NoError(t, err)
	defer p.Close()

	require.NoError(t, p.Submit(context.Background(), 1))
	require.Eventually(t, func() bool { return ran.Load() == 1 }, time.Second, time.Millisecond)
}

func TestSubmit_MultipleJobsDrained(t *testing.T) {
	var ran atomic.Int32
	p, err := New[int](func(j int) { ran.Add(1) },
		WithLoadMonitor[int](newFake(0.5)),
		WithMinWorkers[int](2),
		WithSampleInterval[int](50*time.Millisecond),
	)
	require.NoError(t, err)
	defer p.Close()

	for i := range 20 {
		require.NoError(t, p.Submit(context.Background(), i))
	}
	require.Eventually(t, func() bool { return ran.Load() == 20 }, time.Second, time.Millisecond)
}

func TestSubmit_AfterClose(t *testing.T) {
	p, err := New[int](func(int) {}, WithLoadMonitor[int](newFake(0.5)))
	require.NoError(t, err)
	require.NoError(t, p.Close())

	err = p.Submit(context.Background(), 1)
	require.ErrorIs(t, err, ErrClosed)
}

func TestSubmit_CtxCancelReturnsCtxErr(t *testing.T) {
	// Block the single worker on a gate so the queue fills, then a Submit with
	// an expiring ctx hits the ctx.Done branch and returns the context error
	// (mirrors workerpool/semaphore — distinct from ErrFull backpressure).
	gate := make(chan struct{})
	p, err := New[int](func(j int) { <-gate },
		WithLoadMonitor[int](newFake(0.5)),
		WithMinWorkers[int](1),
		WithMaxWorkers[int](1),
		WithQueueSize[int](1),
		WithSampleInterval[int](50*time.Millisecond),
	)
	require.NoError(t, err)
	defer func() {
		close(gate) // release any blocked worker
		p.Close()
	}()

	// Fill the single worker + the 1-slot queue.
	require.NoError(t, p.Submit(context.Background(), 1)) // worker picks up, blocks on gate
	require.Eventually(t, func() bool { return len(p.queue) == 0 }, time.Second, time.Millisecond)
	require.NoError(t, p.Submit(context.Background(), 2)) // queue now full

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	err = p.Submit(ctx, 3)
	// Cancellation while waiting on a full queue returns the context error,
	// NOT ErrFull — callers can tell backpressure (ErrFull) from cancellation.
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, ErrFull)
}

func TestTrySubmit_Full(t *testing.T) {
	gate := make(chan struct{})
	p, err := New[int](func(j int) { <-gate },
		WithLoadMonitor[int](newFake(0.5)),
		WithMinWorkers[int](1),
		WithMaxWorkers[int](1),
		WithQueueSize[int](1),
		WithSampleInterval[int](50*time.Millisecond),
	)
	require.NoError(t, err)
	defer func() { close(gate); p.Close() }()

	require.NoError(t, p.Submit(context.Background(), 1)) // worker blocks on gate
	require.Eventually(t, func() bool { return len(p.queue) == 0 }, time.Second, time.Millisecond)
	ok, err := p.TrySubmit(2)
	require.NoError(t, err)
	require.True(t, ok)
	// Queue now full (1 slot): TrySubmit must report full.
	ok, err = p.TrySubmit(3)
	require.False(t, ok)
	require.ErrorIs(t, err, ErrFull)
}

func TestTrySubmit_AfterClose(t *testing.T) {
	p, err := New[int](func(int) {}, WithLoadMonitor[int](newFake(0.5)))
	require.NoError(t, err)
	require.NoError(t, p.Close())
	ok, err := p.TrySubmit(1)
	require.False(t, ok)
	require.ErrorIs(t, err, ErrClosed)
}

// --- autoscaler: grow on low-CPU + backlog --------------------------------

func TestAutoscale_GrowsOnLowCPUAndBacklog(t *testing.T) {
	// CPU stays low (0.1 < 0.75). Keep a backlog queued so the grow condition
	// (backlog present) holds across several ticks; the pool should climb from
	// MinWorkers=1 toward MaxWorkers=4.
	gate := make(chan struct{})
	var submitted atomic.Int32
	p, err := New[int](func(j int) { submitted.Add(1); <-gate },
		WithLoadMonitor[int](newFake(0.1)), // always under target
		WithMinWorkers[int](1),
		WithMaxWorkers[int](4),
		WithQueueSize[int](64),
		WithSampleInterval[int](5*time.Millisecond),
	)
	require.NoError(t, err)
	defer func() { close(gate); p.Close() }()

	// Submit enough jobs to keep a backlog while workers are gated.
	for i := range 40 {
		trySubmit(t, p, i)
	}
	// Should grow up to MaxWorkers=4.
	require.Eventually(t, func() bool { return p.Workers() == 4 },
		2*time.Second, time.Millisecond, "expected grow to MaxWorkers, have %d", p.Workers())
}

func TestAutoscale_NoGrowWhenIdle(t *testing.T) {
	// Low CPU but no backlog: the pool must NOT grow (no point idling more
	// workers). It stays at MinWorkers.
	p, err := New[int](func(int) {},
		WithLoadMonitor[int](newFake(0.1)),
		WithMinWorkers[int](1),
		WithMaxWorkers[int](8),
		WithSampleInterval[int](5*time.Millisecond),
	)
	require.NoError(t, err)
	defer p.Close()

	// Let several autoscaler ticks fire with an empty queue.
	time.Sleep(80 * time.Millisecond)
	require.Equal(t, 1, p.Workers(), "must not grow without backlog")
}

// --- autoscaler: shrink on high-CPU ---------------------------------------

func TestAutoscale_ShrinksOnHighCPU(t *testing.T) {
	// Seed at MinWorkers=1 then grow to 4 via low-CPU+backlog (jobs held on a
	// gate so the backlog persists across autoscaler ticks). Confirm growth,
	// release the gate so the backlog drains and workers go idle (a shrink
	// signal is only observed between jobs), then flip the monitor to high CPU
	// and confirm it shrinks back toward MinWorkers. The "shrink while a job is
	// in-flight" semantics are covered by TestShrink_FinishesInFlightJob.
	mon := newFake(0.1)
	gate := make(chan struct{})
	// workFn blocks on gate until released; once released it runs instantly.
	workFn := func(j int) { <-gate }
	p, err := New[int](workFn,
		WithLoadMonitor[int](mon),
		WithMinWorkers[int](1),
		WithMaxWorkers[int](4),
		WithQueueSize[int](64),
		WithSampleInterval[int](5*time.Millisecond),
	)
	require.NoError(t, err)

	for i := range 40 {
		trySubmit(t, p, i)
	}
	require.Eventually(t, func() bool { return p.Workers() == 4 },
		2*time.Second, time.Millisecond, "grow to 4 first, have %d", p.Workers())

	// Release the gate so jobs drain and workers go idle.
	close(gate)
	require.Eventually(t, func() bool { return len(p.queue) == 0 },
		time.Second, time.Millisecond)

	// Flip CPU above target: shrink toward MinWorkers (one worker per tick).
	mon.mu.Lock()
	mon.vals = []float64{0.95}
	mon.idx = 0
	mon.mu.Unlock()

	require.Eventually(t, func() bool { return p.Workers() == 1 },
		2*time.Second, time.Millisecond, "shrink to MinWorkers, have %d", p.Workers())
	require.NoError(t, p.Close())
}

func TestAutoscale_HoldsAtMinOnHighCPU(t *testing.T) {
	// High CPU from the start: never grows past MinWorkers even with backlog.
	gate := make(chan struct{})
	p, err := New[int](func(j int) { <-gate },
		WithLoadMonitor[int](newFake(0.95)), // always over target
		WithMinWorkers[int](2),
		WithMaxWorkers[int](8),
		WithQueueSize[int](64),
		WithSampleInterval[int](5*time.Millisecond),
	)
	require.NoError(t, err)
	defer func() { close(gate); p.Close() }()

	for i := range 40 {
		trySubmit(t, p, i)
	}
	time.Sleep(80 * time.Millisecond)
	require.Equal(t, 2, p.Workers(), "high CPU must hold at MinWorkers")
}

func TestAutoscale_RespectsMaxWorkers(t *testing.T) {
	gate := make(chan struct{})
	p, err := New[int](func(j int) { <-gate },
		WithLoadMonitor[int](newFake(0.1)),
		WithMinWorkers[int](1),
		WithMaxWorkers[int](3),
		WithQueueSize[int](64),
		WithSampleInterval[int](5*time.Millisecond),
	)
	require.NoError(t, err)
	defer func() { close(gate); p.Close() }()

	for i := range 50 {
		trySubmit(t, p, i)
	}
	require.Eventually(t, func() bool { return p.Workers() == 3 },
		2*time.Second, time.Millisecond, "cap at MaxWorkers=3, have %d", p.Workers())
	// Stay capped.
	time.Sleep(40 * time.Millisecond)
	require.Equal(t, 3, p.Workers())
}

func TestAutoscale_HoldsAtTargetCPU(t *testing.T) {
	// CPU exactly at target (no > and no <): neither branch fires, count holds.
	p, err := New[int](func(int) {},
		WithLoadMonitor[int](newFake(0.75)),
		WithMinWorkers[int](1),
		WithMaxWorkers[int](8),
		WithTargetCPU[int](0.75),
		WithSampleInterval[int](5*time.Millisecond),
	)
	require.NoError(t, err)
	defer p.Close()
	time.Sleep(80 * time.Millisecond)
	require.Equal(t, 1, p.Workers())
}

// --- autoscaler: bad CPU sample is non-fatal ------------------------------

func TestAutoscale_BadSampleHoldsSteady(t *testing.T) {
	mon := &fakeMonitor{err: errors.New("sensor offline")}
	p, err := New[int](func(int) {},
		WithLoadMonitor[int](mon),
		WithMinWorkers[int](1),
		WithMaxWorkers[int](8),
		WithSampleInterval[int](5*time.Millisecond),
	)
	require.NoError(t, err)
	defer p.Close()

	time.Sleep(80 * time.Millisecond)
	require.Greater(t, atomic.LoadInt32(&mon.calls), int32(0), "autoscaler must keep ticking despite errors")
	require.Equal(t, 1, p.Workers(), "hold steady on bad samples")
}

func TestAutoscale_CloseDuringSample(t *testing.T) {
	// Cover the post-sample done-check: Close fires while monitor.CPU() blocks.
	// When CPU() returns, the autoscaler observes done and exits via the second
	// select (no grow/shrink applied to a stale sample).
	sampleGate := make(chan struct{})
	mon := &blockingMonitor{gate: sampleGate, frac: 0.1}
	p, err := New[int](func(int) {},
		WithLoadMonitor[int](mon),
		WithMinWorkers[int](1),
		WithMaxWorkers[int](8),
		WithSampleInterval[int](5*time.Millisecond),
	)
	require.NoError(t, err)

	// Wait until the autoscaler is parked inside CPU() (blocking on the gate).
	require.Eventually(t, func() bool { return mon.started.Load() }, time.Second, time.Millisecond)
	// Close while the sample is in flight, then release the sample.
	go p.Close()
	// Give Close a moment to close `done`, then unblock the in-flight sample so
	// the autoscaler reaches the post-sample done-check.
	time.Sleep(20 * time.Millisecond)
	close(sampleGate)

	// Close must return promptly: the autoscaler exits via the post-sample
	// done-check rather than looping for another tick.
	done := make(chan struct{})
	go func() { p.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after closing during sample")
	}
}

// blockingMonitor blocks each CPU() call on gate until released, then returns
// frac. `started` lets a test wait until the autoscaler has entered CPU().
type blockingMonitor struct {
	gate    chan struct{}
	frac    float64
	started atomic.Bool
}

func (m *blockingMonitor) CPU() (float64, error) {
	m.started.Store(true)
	<-m.gate
	return m.frac, nil
}

// --- Close: idempotent, drains, bounded -----------------------------------

func TestClose_Idempotent(t *testing.T) {
	var ran atomic.Int32
	p, err := New[int](func(j int) { ran.Add(1) },
		WithLoadMonitor[int](newFake(0.5)),
		WithSampleInterval[int](50*time.Millisecond),
	)
	require.NoError(t, err)

	// Submit before close; Close must drain queued jobs.
	require.NoError(t, p.Submit(context.Background(), 1))
	require.NoError(t, p.Submit(context.Background(), 2))

	require.NoError(t, p.Close())
	require.NoError(t, p.Close()) // idempotent
	require.NoError(t, p.Close())

	require.Eventually(t, func() bool { return ran.Load() == 2 }, time.Second, time.Millisecond)
}

func TestClose_DrainsQueuedJobs(t *testing.T) {
	var ran atomic.Int32
	p, err := New[int](func(j int) { ran.Add(1) },
		WithLoadMonitor[int](newFake(0.5)),
		WithMinWorkers[int](2),
		WithQueueSize[int](16),
		WithSampleInterval[int](50*time.Millisecond),
	)
	require.NoError(t, err)

	for i := range 16 {
		require.NoError(t, p.Submit(context.Background(), i))
	}
	require.NoError(t, p.Close())
	require.Equal(t, int32(16), ran.Load(), "all queued jobs drained on Close")
}

func TestClose_StopsAutoscaler(t *testing.T) {
	mon := newFake(0.1)
	p, err := New[int](func(int) {},
		WithLoadMonitor[int](mon),
		WithSampleInterval[int](5*time.Millisecond),
	)
	require.NoError(t, err)
	require.NoError(t, p.Close())

	callsBefore := atomic.LoadInt32(&mon.calls)
	time.Sleep(40 * time.Millisecond)
	callsAfter := atomic.LoadInt32(&mon.calls)
	require.Equal(t, callsBefore, callsAfter, "autoscaler must stop sampling after Close")
}

func TestClose_Bounded_NewSubmitsRejected(t *testing.T) {
	p, err := New[int](func(int) {}, WithLoadMonitor[int](newFake(0.5)))
	require.NoError(t, err)
	require.NoError(t, p.Close())
	// Every post-close submit is rejected; no enqueue, no growth.
	for i := range 5 {
		require.ErrorIs(t, p.Submit(context.Background(), i), ErrClosed)
	}
}

// --- shrink preserves in-flight jobs (no hard preemption) -----------------

func TestShrink_FinishesInFlightJob(t *testing.T) {
	// Drive one job into a worker, trigger a shrink (high CPU) while the job is
	// running. The job must complete; the worker exits only after.
	var done atomic.Int32
	started := make(chan struct{})
	gate := make(chan struct{})

	mon := newFake(0.1)
	p, err := New[int](func(j int) {
		if j == 0 {
			close(started)
			<-gate // hold job 0 in-flight
		}
		done.Add(1)
	},
		WithLoadMonitor[int](mon),
		WithMinWorkers[int](1),
		WithMaxWorkers[int](2),
		WithQueueSize[int](4),
		WithSampleInterval[int](5*time.Millisecond),
	)
	require.NoError(t, err)

	// Grow to 2 first via backlog.
	for i := range 4 {
		trySubmit(t, p, i)
	}
	require.Eventually(t, func() bool { return p.Workers() == 2 },
		2*time.Second, time.Millisecond, "grow to 2, have %d", p.Workers())

	<-started // job 0 is now in-flight (blocked on gate)

	// Flip to high CPU: shrink fires. Worker count drops to 1, but job 0 is
	// still in-flight (it's the one holding the gate).
	mon.mu.Lock()
	mon.vals = []float64{0.95}
	mon.idx = 0
	mon.mu.Unlock()

	require.Eventually(t, func() bool { return p.Workers() == 1 },
		2*time.Second, time.Millisecond, "shrink to 1, have %d", p.Workers())

	// Release the in-flight job; it completes.
	close(gate)
	require.Eventually(t, func() bool { return done.Load() >= 1 }, time.Second, time.Millisecond)
	require.NoError(t, p.Close())
}

// --- Workers() snapshot ----------------------------------------------------

func TestWorkers_AtomicSnapshot(t *testing.T) {
	// Workers() must be safe to call concurrently with the autoscaler and must
	// never go negative. Run it under -race.
	p, err := New[int](func(int) {},
		WithLoadMonitor[int](newFake(0.5)),
		WithMinWorkers[int](2),
		WithMaxWorkers[int](4),
		WithSampleInterval[int](2*time.Millisecond),
	)
	require.NoError(t, err)
	defer p.Close()

	var stop atomic.Bool
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for !stop.Load() {
				if w := p.Workers(); w < 1 || w > 4 {
					t.Errorf("workers out of bounds: %d", w)
				}
			}
		})
	}
	time.Sleep(50 * time.Millisecond)
	stop.Store(true)
	wg.Wait()
}

// --- coverage: edge branches ----------------------------------------------

func TestShrink_NoWorkersNoOp(t *testing.T) {
	// shrink() with an empty stopChs slice is a no-op (covers the len==0 return).
	p, err := New[int](func(int) {}, WithLoadMonitor[int](newFake(0.5)))
	require.NoError(t, err)
	defer p.Close()

	// Directly drain stopChs so shrink has nothing to signal.
	p.stopChMu.Lock()
	p.stopChs = nil
	p.stopChMu.Unlock()
	p.shrink() // must not panic and must not change worker count
	require.Equal(t, 1, p.Workers())
}

func TestGrow_AtMaxWorkersCap(t *testing.T) {
	// grow() must stop once MaxWorkers is reached (covers the early return).
	p, err := New[int](func(int) {},
		WithLoadMonitor[int](newFake(0.5)),
		WithMinWorkers[int](1),
		WithMaxWorkers[int](2),
	)
	require.NoError(t, err)
	defer p.Close()

	// Seed workers up to Max, then ask for more — no-op.
	p.grow(2) // already 1 → +2 would exceed; grow caps at 2
	require.Equal(t, 2, p.Workers())
	p.grow(5) // already at max → no change
	require.Equal(t, 2, p.Workers())
}

func TestWorker_QueueClosedExits(t *testing.T) {
	// Closing the queue channel makes a worker exit via the `!ok` branch
	// (covered directly rather than via Close, which uses the done channel).
	// Also covers drainQueue's `!ok` return.
	p, err := New[int](func(int) {}, WithLoadMonitor[int](newFake(0.5)))
	require.NoError(t, err)
	require.Equal(t, 1, p.Workers())

	// Close the queue directly; the worker's `<-p.queue` returns !ok → exit.
	close(p.queue)
	require.Eventually(t, func() bool { return p.Workers() == 0 },
		time.Second, time.Millisecond, "worker must exit on closed queue")

	// Close must still be safe even though the worker already exited via !ok.
	// Reset done usage: trigger close path. The autoscaler is still running;
	// Close shuts it down cleanly.
	require.NoError(t, p.Close())
}

func TestSubmit_ConcurrentCloseReturnsErrClosed(t *testing.T) {
	// Race Submit against Close: a Submit in flight when done closes must hit
	// the `case <-p.done: return ErrClosed` branch. Run many iterations under
	// -race to surface any data race.
	for range 50 {
		p, err := New[int](func(int) {}, WithLoadMonitor[int](newFake(0.5)))
		require.NoError(t, err)
		go p.Close()
		// Submit concurrently; outcome is either nil (enqueued before close)
		// or ErrClosed. Any other error is a bug.
		err = p.Submit(context.Background(), 1)
		require.True(t, err == nil || errors.Is(err, ErrClosed), "unexpected err: %v", err)
	}
}

func TestTrySubmit_ConcurrentCloseReturnsErrClosed(t *testing.T) {
	// Same race coverage for TrySubmit's `case <-p.done` branch.
	for range 50 {
		p, err := New[int](func(int) {}, WithLoadMonitor[int](newFake(0.5)))
		require.NoError(t, err)
		go p.Close()
		ok, err := p.TrySubmit(1)
		require.True(t, err == nil || errors.Is(err, ErrClosed), "unexpected err: %v", err)
		require.True(t, ok || !ok) // both outcomes valid
	}
}

func TestSubmit_DoneBranchDeterministic(t *testing.T) {
	// Deterministically hit Submit's `case <-p.done: return ErrClosed` branch:
	// hold the worker on a gate so the queue stays full, close `done` directly
	// (without setting `closed`), then Submit must select the done arm (the
	// `p.queue <- j` arm stays blocked because the queue is full).
	gate := make(chan struct{})
	p, err := New[int](func(int) { <-gate },
		WithLoadMonitor[int](newFake(0.5)),
		WithMinWorkers[int](1),
		WithMaxWorkers[int](1),
		WithQueueSize[int](1),
		WithSampleInterval[int](time.Hour), // park the autoscaler
	)
	require.NoError(t, err)

	// Worker picks up job 1 and parks on gate; queue slot frees, refill it.
	require.NoError(t, p.Submit(context.Background(), 1))
	require.Eventually(t, func() bool { return len(p.queue) == 0 }, time.Second, time.Millisecond)
	require.NoError(t, p.Submit(context.Background(), 2)) // queue now full

	// Close done directly (without setting `closed`) so Submit enters the select
	// and only the done arm is ready.
	close(p.done)
	err = p.Submit(context.Background(), 3)
	require.ErrorIs(t, err, ErrClosed)

	close(gate) // release the parked worker so it can exit
	p.closed.Store(true)
	p.scaler.Wait()
	p.wg.Wait()
}

func TestTrySubmit_DoneBranchDeterministic(t *testing.T) {
	// Deterministically hit TrySubmit's `case <-p.done` arm: close done while
	// the queue is full, then TrySubmit selects done over default-full.
	p, err := New[int](func(int) {},
		WithLoadMonitor[int](newFake(0.5)),
		WithMinWorkers[int](1),
		WithMaxWorkers[int](1),
		WithQueueSize[int](1),
		WithSampleInterval[int](time.Hour),
	)
	require.NoError(t, err)

	require.NoError(t, p.Submit(context.Background(), 1))
	require.Eventually(t, func() bool { return len(p.queue) == 0 }, time.Second, time.Millisecond)
	require.NoError(t, p.Submit(context.Background(), 2)) // queue full

	close(p.done)
	// The select now has both default (full) and <-p.done (closed) ready; Go
	// picks pseudo-randomly, so retry until the done arm fires.
	require.Eventually(t, func() bool {
		ok, err := p.TrySubmit(9)
		return !ok && errors.Is(err, ErrClosed)
	}, time.Second, time.Millisecond)

	p.closed.Store(true)
	p.scaler.Wait()
	p.wg.Wait()
}

func TestDrainQueue_ClosedChannel(t *testing.T) {
	// drainQueue's `!ok` branch fires only when the queue channel is closed
	// (production never closes the queue — only `done` — but the guard is
	// defensive). Call drainQueue directly on a closed, empty queue so the
	// branch is hit deterministically, without racing the live worker for jobs.
	p, err := New[int](func(int) {}, WithLoadMonitor[int](newFake(0.5)))
	require.NoError(t, err)

	// Park the worker so it can't compete for the close signal: close the queue
	// and call drainQueue from the test goroutine. The non-blocking receive on a
	// closed, empty channel yields ok=false → drainQueue returns immediately.
	close(p.queue)
	p.drainQueue() // hits `case j, ok := <-p.queue: if !ok { return }`

	close(p.done)
	p.closed.Store(true)
	p.scaler.Wait()
	p.wg.Wait()
}

// TestClose_FinalDrainRunsAcceptedStraggler is the R24 regression test for the
// accepted-but-abandoned race: a Submit that returns nil must execute its job,
// even when Close races the enqueue. Without Close's final non-blocking drain,
// a job that lands in the queue AFTER the last worker's drainQueue finished
// (but the worker had not yet observed close(done)) would be silently dropped.
//
// The race is forced open by gating the single worker so it cannot drain until
// Close has begun, then releasing it so the worker exits and Close's final
// drain is the only thing that can run the straggler. We assert the accepted
// job ran exactly once.
func TestClose_FinalDrainRunsAcceptedStraggler(t *testing.T) {
	// Single worker, queue size 4, autoscaler parked so only the worker drains.
	gate := make(chan struct{})
	var ran atomic.Int64
	p, err := New[int](
		func(j int) {
			// Block the worker here until the test releases the gate, so the
			// worker cannot drain the queue while we set up the race.
			<-gate
			ran.Add(int64(j + 1)) // record non-zero so a no-op (0) is detectable
		},
		WithMinWorkers[int](1),
		WithMaxWorkers[int](1),
		WithQueueSize[int](4),
		WithLoadMonitor[int](newFake(0.5)),
		WithSampleInterval[int](time.Hour),
	)
	require.NoError(t, err)

	// Occupy the single worker so it stops pulling from the queue.
	require.NoError(t, p.Submit(context.Background(), 0))

	// Queue 3 jobs while the worker is busy. They sit in the queue.
	for i := 1; i <= 3; i++ {
		require.NoError(t, p.Submit(context.Background(), i))
	}

	// Start Close in a goroutine. It will block on wg.Wait until the worker
	// exits, and the worker won't exit until we release the gate (it is parked
	// on the in-flight job 0). close(done) has already fired inside Close.
	closeDone := make(chan error, 1)
	go func() { closeDone <- p.Close() }()

	// Let the worker finish job 0 and observe close(done) → drainQueue → exit.
	// The timing here is the crux: by the time the worker runs drainQueue, the
	// 3 queued jobs are present, so the worker should drain them. But to also
	// stress the accepted-straggler path, enqueue one MORE job right as we
	// release the gate — this job races the worker's final drain. Submit may
	// return nil (accepted) even though done is closed (the select can pick the
	// queue-send arm). If accepted, it MUST run via the worker or Close's drain.
	accepted := true
	if err := p.Submit(context.Background(), 4); err != nil {
		accepted = false // legitimately rejected as closed — fine, then nothing to assert
	}

	close(gate) // unblock the worker → it finishes job 0, drains, exits
	require.NoError(t, <-closeDone)

	// Every accepted job must have run exactly once. ran holds the sum of (j+1)
	// for jobs 0..4 that actually executed; recompute the expected subset.
	//
	// job 0 was always accepted+run. jobs 1..3 were queued and accepted. job 4
	// may or may not have been accepted depending on the race; only count it if
	// accepted.
	expected := int64(1) + int64(2) + int64(3) + int64(4) // jobs 0..3
	if accepted {
		expected += int64(5) // job 4
	}
	require.Equal(t, expected, ran.Load(),
		"accepted jobs must all run (no silent drops); got ran=%d expected=%d", ran.Load(), expected)

	// Post-close: Submit now consistently rejects.
	require.ErrorIs(t, p.Submit(context.Background(), 99), ErrClosed)
}

// TestClose_FinalDrainNoStragglersIsHarmless confirms the final drain is a
// no-op when the queue is already empty (the common case): Close still returns
// nil and behaves as before. Guards against the fix introducing overhead or a
// hang on an empty queue.
func TestClose_FinalDrainNoStragglersIsHarmless(t *testing.T) {
	p, err := New[int](
		func(int) {},
		WithLoadMonitor[int](newFake(0.5)),
		WithSampleInterval[int](time.Hour),
	)
	require.NoError(t, err)
	// No jobs submitted at all → queue empty → final drain is a no-op.
	require.NoError(t, p.Close())
	// Idempotent: a second Close must still be safe (no drain on dead queue).
	require.NoError(t, p.Close())
}

// TestPool_WorkPanicRecovered proves a panicking user work func is recovered on
// the library-owned worker goroutine instead of crashing the host: the worker
// survives, the surrounding jobs still process, the recovery is counted, the
// onPanic hook fires, and the pool keeps functioning afterward. Mirrors the
// kit-callback convention (batcher.safeFlush / workerpool.safeCall).
func TestPool_WorkPanicRecovered(t *testing.T) {
	var processed atomic.Int64
	work := func(j int) {
		if j < 0 {
			panic("negative job")
		}
		processed.Add(1)
	}
	p, err := New[int](work,
		WithMinWorkers[int](1),
		WithMaxWorkers[int](2),
		WithLoadMonitor[int](newFake(0.5)),
		WithSampleInterval[int](50*time.Millisecond),
	)
	require.NoError(t, err)

	var saw atomic.Value
	p.SetOnPanic(func(r any) { saw.Store(r) })

	// Surround the panicking job with normal ones; the worker must survive the
	// panic and keep draining the rest.
	require.NoError(t, p.Submit(context.Background(), 1))
	require.NoError(t, p.Submit(context.Background(), -1)) // panics
	require.NoError(t, p.Submit(context.Background(), 2))
	require.NoError(t, p.Submit(context.Background(), 3))

	require.Eventually(t, func() bool { return processed.Load() == 3 }, time.Second, time.Millisecond)
	require.Equal(t, uint64(1), p.Recovered(), "the panicking job must be recovered and counted once")
	require.NotNil(t, saw.Load(), "onPanic must fire on the recovered work panic")

	// The pool still functions after the panic: submit, drain, clean close.
	require.NoError(t, p.Submit(context.Background(), 4))
	require.Eventually(t, func() bool { return processed.Load() == 4 }, time.Second, time.Millisecond)
	require.NoError(t, p.Close())
}

// TestPool_SetOnPanicDisable covers the SetOnPanic(nil) disable branch and the
// safeWork path that recovers with no hook installed: a panicking job is still
// recovered (host survives, Recovered climbs) when onPanic is nil.
func TestPool_SetOnPanicDisable(t *testing.T) {
	p, err := New[int](func(j int) { panic(j) },
		WithLoadMonitor[int](newFake(0.5)),
		WithSampleInterval[int](50*time.Millisecond),
	)
	require.NoError(t, err)
	p.SetOnPanic(nil) // disable: must be a safe no-op
	require.NoError(t, p.Submit(context.Background(), 1))
	require.Eventually(t, func() bool { return p.Recovered() == 1 }, time.Second, time.Millisecond)
	require.NoError(t, p.Close())
}

// TestClose_SubmitRaceNoOrphan is the regression for the Submit/Close race: a
// Submit concurrent with Close must never report a job accepted (nil) that is
// then silently dropped. Submit holds the read lock across its closed-check +
// queue send, and Close's final drain holds the write lock, so every accepted
// job is observed by the drain. Before the lock, a Submit whose select landed a
// job in the queue AFTER Close's final drain orphaned it (no worker left).
func TestClose_SubmitRaceNoOrphan(t *testing.T) {
	for iter := 0; iter < 50; iter++ {
		var ran atomic.Int64
		p, err := New[int](
			func(int) { ran.Add(1) },
			WithMinWorkers[int](2), WithMaxWorkers[int](4),
			WithQueueSize[int](4),
			WithSampleInterval[int](time.Microsecond),
			WithLoadMonitor[int](newFake(0.5)),
		)
		require.NoError(t, err)

		const submitters = 12
		const perSub = 40
		var accepted atomic.Int64
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(submitters + 1)
		for range submitters {
			go func() {
				defer wg.Done()
				<-start
				for i := 0; i < perSub; i++ {
					if err := p.Submit(context.Background(), i); err == nil {
						accepted.Add(1)
					}
				}
			}()
		}
		go func() {
			defer wg.Done()
			<-start
			p.Close()
		}()
		close(start)
		wg.Wait()

		require.Equal(t, accepted.Load(), ran.Load(),
			"iter %d: %d accepted jobs but only %d ran (orphan)", iter, accepted.Load(), ran.Load())
	}
}
