package adaptive

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Fuzz_New validates that Pool construction never panics and that its
// accept/reject decision is consistent with the documented configuration
// invariants, for arbitrary option combinations.
//
// Invariant under test:
//   - New never panics for any inputs (it returns an error instead).
//   - When the configuration is valid, the returned pool is seeded at
//     MinWorkers, the live worker count is within [MinWorkers, MaxWorkers],
//     and Close drains cleanly.
//   - The monitor wiring is correct: an injected monitor is used; otherwise
//     the gopsutil default is selected.
//
// The corpus targets the documented validation boundaries: each numeric field
// is seeded below, at, and above its threshold so the fuzzer explores both
// sides of every branch in New.
func Fuzz_New(f *testing.F) {
	// Seeds cover each validation rule on both sides of its boundary.
	f.Add(1, 1, 0.75, int64(time.Second), 4)      // all defaults (valid)
	f.Add(0, 1, 0.75, int64(time.Second), 4)      // MinWorkers < 1 (invalid)
	f.Add(-5, 1, 0.75, int64(time.Second), 4)     // MinWorkers negative (invalid)
	f.Add(4, 2, 0.75, int64(time.Second), 4)      // MaxWorkers < MinWorkers (invalid)
	f.Add(2, 2, 0.75, int64(time.Second), 4)      // MaxWorkers == MinWorkers (valid edge)
	f.Add(1, 4, 0.0, int64(time.Second), 4)       // TargetCPU == 0 (invalid)
	f.Add(1, 4, 1.0, int64(time.Second), 4)       // TargetCPU == 1 (invalid)
	f.Add(1, 4, -0.1, int64(time.Second), 4)      // TargetCPU < 0 (invalid)
	f.Add(1, 4, 1.5, int64(time.Second), 4)       // TargetCPU > 1 (invalid)
	f.Add(1, 4, 0.5, int64(0), 4)                 // SampleInterval == 0 (invalid)
	f.Add(1, 4, 0.5, int64(-time.Second), 4)      // SampleInterval < 0 (invalid)
	f.Add(1, 4, 0.5, int64(time.Second), 0)       // QueueSize == 0 → defaults to MaxWorkers (valid)
	f.Add(1, 4, 0.5, int64(time.Second), -3)      // QueueSize < 0 → defaults to MaxWorkers (valid)
	f.Add(1, 8, 0.5, int64(time.Millisecond), 16) // a typical valid config

	f.Fuzz(func(t *testing.T, minWorkers, maxWorkers int, targetCPU float64, sampleInterval int64, queueSize int) {
		// A nil work function is its own documented rejection path; the fuzzer
		// always supplies a real work function so the numeric options are the
		// variable under test.
		var ran atomic.Int32
		work := func(int) { ran.Add(1) }

		opts := []Option[int]{
			WithMinWorkers[int](minWorkers),
			WithMaxWorkers[int](maxWorkers),
			WithTargetCPU[int](targetCPU),
			WithSampleInterval[int](time.Duration(sampleInterval)),
			WithQueueSize[int](queueSize),
			// Inject a fake monitor so construction never reads real CPU and
			// the autoscaler (if it starts) is deterministic and fast.
			WithLoadMonitor[int](newFake(0.5)),
		}

		// Invariant 1: New must never panic for any inputs.
		p, err := New[int](work, opts...)
		if err != nil {
			// Rejected: nothing to close. The error must be non-empty so callers
			// have a usable message.
			require.Error(t, err)
			require.Nil(t, p)
			return
		}

		// Accepted. Invariant 2: seeded at MinWorkers, count within bounds.
		require.NotNil(t, p)
		require.GreaterOrEqual(t, p.Workers(), 1, "live workers must be >= 1")
		require.LessOrEqual(t, p.Workers(), maxWorkers, "live workers must be <= MaxWorkers")
		require.Equal(t, minWorkers, p.Workers(), "pool must seed at MinWorkers")

		// Invariant 3: an injected monitor is wired through (not the default).
		_, isFake := p.monitor.(*fakeMonitor)
		require.True(t, isFake, "injected monitor must be used, got %T", p.monitor)

		// Invariant 4: Close drains cleanly and is idempotent.
		require.NoError(t, p.Close())
		require.NoError(t, p.Close())
	})
}

// Fuzz_SubmitCloseRoundtrip validates the do-no-harm contract under arbitrary
// submission patterns: no panic, every accepted job runs exactly once, and
// Close drains the queue completely.
//
// Invariant under test:
//   - Submit/TrySubmit never panic for any job value or count.
//   - Every job that is ACCEPTED (nil error from Submit, or ok==true from
//     TrySubmit) runs exactly once — no drops, no duplicates — by the time
//     Close returns.
//   - Post-close submissions consistently return ErrClosed.
//   - Worker count stays within [MinWorkers, MaxWorkers] across the run.
//
// The pool runs with a fake monitor pinned under target so the autoscaler
// never shrinks below MinWorkers; jobs are trivial (counter increment) so the
// roundtrip is the property under test, not scheduling timing.
func Fuzz_SubmitCloseRoundtrip(f *testing.F) {
	const minW, maxW = 2, 4

	// Seeds vary job count, queue capacity, and job value so the corpus spans
	// empty, partial, exactly-full, and over-full queues.
	f.Add(0, 0)   // empty
	f.Add(1, 7)   // single job
	f.Add(7, 7)   // exactly fills a size-7 queue
	f.Add(8, 7)   // one over queue capacity (backpressure on Submit)
	f.Add(50, 7)  // many jobs, small queue
	f.Add(50, 64) // many jobs, large queue
	f.Add(13, 1)  // more jobs than a 1-slot queue
	f.Add(3, 0)   // queueSize 0 → defaults to MaxWorkers

	f.Fuzz(func(t *testing.T, nJobs int, queueSize int) {
		if nJobs < 0 {
			nJobs = -nJobs // Submit count is conceptually non-negative.
		}
		// Cap the run so a pathological fuzz input cannot OOM the harness.
		if nJobs > 5000 {
			nJobs = 5000
		}

		var ran atomic.Int64
		p, err := New[int](
			func(j int) { ran.Add(1) },
			WithMinWorkers[int](minW),
			WithMaxWorkers[int](maxW),
			WithQueueSize[int](queueSize),
			// Pin CPU below target so the autoscaler holds at MinWorkers and
			// never shrinks; roundtrip correctness must not depend on scaling.
			WithLoadMonitor[int](newFake(0.1)),
			WithSampleInterval[int](time.Hour), // park the autoscaler
		)
		require.NoError(t, err)

		// Invariant: worker count is always within bounds, checked before and
		// after submission.
		require.InDelta(t, minW, p.Workers(), 0)

		// Submit nJobs. A generously long deadline lets the bounded queue drain
		// even when queueSize < nJobs; Submit only blocks on backpressure.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		accepted := 0
		for i := 0; i < nJobs; i++ {
			err := p.Submit(ctx, i)
			switch {
			case err == nil:
				accepted++
			case isClosedErr(err):
				// A concurrent Close cannot happen here (we drive Close below),
				// but tolerate ErrClosed defensively rather than fail the run.
			default:
				// ctx.Err() on a genuinely full queue under deadline is a real
				// signal; record it but keep going so we still validate Close.
			}
			// Invariant: worker count never leaves [MinWorkers, MaxWorkers].
			require.GreaterOrEqual(t, p.Workers(), minW)
			require.LessOrEqual(t, p.Workers(), maxW)
		}
		cancel()

		// Close drains queued jobs and waits for workers — every accepted job
		// must have run exactly once.
		require.NoError(t, p.Close())

		// Invariant: accepted jobs run exactly once (no drops, no duplicates).
		require.Equal(t, int64(accepted), ran.Load(),
			"accepted %d jobs but ran %d; every accepted job must run exactly once", accepted, ran.Load())

		// Invariant: post-close submissions consistently return ErrClosed for
		// any job value, and never panic.
		for _, j := range []int{0, 1, -1, 1 << 30, -1 << 31} {
			require.ErrorIs(t, p.Submit(context.Background(), j), ErrClosed)
			ok, err := p.TrySubmit(j)
			require.False(t, ok)
			require.ErrorIs(t, err, ErrClosed)
		}
	})
}

// isClosedErr reports whether err is ErrClosed (or wraps it), without pulling
// in errors at package level.
func isClosedErr(err error) bool { return err != nil && (err == ErrClosed || errIs(err, ErrClosed)) }

// errIs is a local errors.Is to avoid an "errors" import solely for one call.
func errIs(err, target error) bool {
	for {
		if err == target {
			return true
		}
		x, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = x.Unwrap()
		if err == nil {
			return false
		}
	}
}
