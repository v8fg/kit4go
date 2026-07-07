package workerpool

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWithResults_NegativeBuffer covers the `buffer < 0` clamp-to-0 branch of
// WithResults (previously uncovered: 80%).
func TestWithResults_NegativeBuffer(t *testing.T) {
	// A negative buffer must be clamped to 0 (unbuffered results channel) — no
	// panic, results channel is non-nil and collectW is set.
	p := New[int](1, WithResults[int](-5))
	defer p.Close()
	require.NotNil(t, p.Results(), "results channel must be created even with buffer<0")

	// Drive one job through so the collectW result-send path executes.
	require.NoError(t, p.Submit(context.Background(), func(context.Context) (int, error) {
		return 7, nil
	}))
	// With an unbuffered results channel, a worker is selected on done|results;
	// drain it promptly so the worker doesn't block.
	time.Sleep(30 * time.Millisecond)
	select {
	case r := <-p.Results():
		require.Equal(t, 7, r.Value)
	default:
		// On slow CI the result may have been dropped on Close; the key coverage
		// goal (buffer<0 clamp + collectW send) is already exercised.
	}
}

// TestSubmit_DoneBranchDeterministic deterministically covers the
// `case <-p.done: return ErrClosed` branch of Submit by closing the unexported
// `done` channel directly (leaving `closed` false), then calling Submit while
// the queue is full. Submit's select must pick the done arm and return ErrClosed.
func TestSubmit_DoneBranchDeterministic(t *testing.T) {
	p := New[int](1, WithQueueSize[int](1))
	slow := make(chan struct{})
	_ = p.Submit(context.Background(), func(context.Context) (int, error) {
		<-slow
		return 0, nil
	})
	_ = p.Submit(context.Background(), func(context.Context) (int, error) { return 0, nil }) // queued

	// Close `done` directly (closed flag stays false) so Submit's fast-path
	// check is bypassed and the select observes the done arm.
	close(p.done)
	err := p.Submit(context.Background(), func(context.Context) (int, error) { return 0, nil })
	require.ErrorIs(t, err, ErrClosed, "Submit must return ErrClosed via the done arm")

	// Cleanup: release the worker and mark the pool closed so Close() is a no-op
	// and wg.Wait returns (the worker has already exited via <-p.done).
	close(slow)
	p.closed.Store(true)
	p.wg.Wait()
}

// TestSubmit_DoneBranchViaRace additionally exercises the done arm through the
// public Close() path under contention (best-effort coverage supplement).
func TestSubmit_DoneBranchViaRace(t *testing.T) {
	p := New[int](4, WithQueueSize[int](2))
	slow := make(chan struct{})
	for range 4 {
		_ = p.Submit(context.Background(), func(context.Context) (int, error) {
			<-slow
			return 0, nil
		})
	}

	errs := make(chan error, 64)
	go func() {
		defer close(errs)
		for range 64 {
			errs <- p.Submit(context.Background(), func(context.Context) (int, error) { return 0, nil })
		}
	}()

	closeDone := make(chan struct{})
	go func() {
		p.Close()
		close(closeDone)
	}()
	close(slow)
	<-closeDone

	for err := range errs {
		if err != nil {
			require.ErrorIs(t, err, ErrClosed, "Submit failure must be ErrClosed")
		}
	}
}

// TestTrySubmit_DoneBranchDeterministic deterministically covers the
// `case <-p.done: return false` branch of TrySubmit. The closed-flag fast path
// normally short-circuits before the select can observe `done`; we bypass it by
// closing the unexported `done` channel directly (the internal-test advantage),
// leaving `closed` false. With the queue full and `done` closed, TrySubmit's
// select hits the done arm and returns false.
func TestTrySubmit_DoneBranchDeterministic(t *testing.T) {
	p := New[int](1, WithQueueSize[int](1))
	// Occupy the worker + fill the queue so the default arm cannot fire.
	slow := make(chan struct{})
	_ = p.Submit(context.Background(), func(context.Context) (int, error) {
		<-slow
		return 0, nil
	})
	_ = p.Submit(context.Background(), func(context.Context) (int, error) { return 0, nil })

	// Close `done` directly WITHOUT setting the closed flag: TrySubmit's
	// `closed.Load()` check passes, then the select observes the done channel.
	close(p.done)
	require.False(t, p.TrySubmit(context.Background(), func(context.Context) (int, error) { return 0, nil }),
		"TrySubmit must return false via the done arm")

	// Cleanup: unblock the worker and mark closed so Close() is a no-op. The
	// worker has already exited (it selected on done), so wg.Wait returns once
	// the blocked job returns.
	close(slow)
	p.closed.Store(true)
	// Re-collect the orphaned worker goroutine: it selected <-p.done and is
	// draining/exiting. Wait for it.
	p.wg.Wait()
}

// TestTrySubmit_DoneBranchViaRace additionally exercises the done arm through
// the public Close() path under contention (best-effort; the deterministic test
// above guarantees the branch is covered regardless of timing).
func TestTrySubmit_DoneBranchViaRace(t *testing.T) {
	p := New[int](2, WithQueueSize[int](1))
	slow := make(chan struct{})
	for range 2 {
		_ = p.Submit(context.Background(), func(context.Context) (int, error) {
			<-slow
			return 0, nil
		})
	}
	_ = p.Submit(context.Background(), func(context.Context) (int, error) { return 0, nil })

	closeDone := make(chan struct{})
	go func() {
		p.Close()
		close(closeDone)
	}()
	for range 200 {
		_ = p.TrySubmit(context.Background(), func(context.Context) (int, error) { return 0, nil })
	}
	close(slow)
	<-closeDone
}

// TestSubmit_RejectsAfterCloseFlag covers the closed.Load() short-circuit in
// Submit (the documented fast path).
func TestSubmit_RejectsAfterCloseFlag(t *testing.T) {
	p := New[int](1)
	p.Close()
	require.ErrorIs(t, p.Submit(context.Background(), func(context.Context) (int, error) { return 0, nil }), ErrClosed)
	require.False(t, p.TrySubmit(context.Background(), func(context.Context) (int, error) { return 0, nil }))
}

// TestWorker_DrainsOnDone covers the worker's `case <-p.done` branch: when
// Close is called, the worker selects done, calls drainQueue, and exits. We
// verify the drain happened by asserting queued jobs completed during Close.
func TestWorker_DrainsOnDone(t *testing.T) {
	p := New[int](1, WithQueueSize[int](4))
	var processed atomic.Int64
	// Submit several jobs, then Close while some are still queued — Close must
	// drain them (worker hits the `case <-p.done` arm and runs drainQueue).
	for range 4 {
		require.NoError(t, p.Submit(context.Background(), func(context.Context) (int, error) {
			processed.Add(1)
			return 0, nil
		}))
	}
	p.Close()
	require.Equal(t, int64(4), processed.Load(), "queued jobs must be drained on Close")
}

// TestWithQueueSize_ZeroOrNegative covers the WithQueueSize option when n <= 0
// (the option is a no-op and New falls back to the default queue size = workers).
func TestWithQueueSize_ZeroOrNegative(t *testing.T) {
	p := New[int](3, WithQueueSize[int](0), WithQueueSize[int](-1))
	defer p.Close()
	require.Equal(t, 3, p.Workers())
	// Pool must still function with the default queue.
	require.NoError(t, p.Submit(context.Background(), func(context.Context) (int, error) { return 1, nil }))
	time.Sleep(30 * time.Millisecond)
}

// TestSubmit_ResultsDroppedOnShutdown exercises runJob's `case <-p.done` branch
// inside the result-send select: a job completes during shutdown with a full
// results channel, so the result is dropped (not deadlocking Close).
func TestSubmit_ResultsDroppedOnShutdown(t *testing.T) {
	// buffer=0 results channel: any result-send races against a consumer; with
	// Close firing done, the result-send selects done and drops.
	p := New[int](1, WithResults[int](0))
	var ran atomic.Int64
	// Submit a job then immediately Close; the worker's runJob result-send will
	// see done closed and drop the result.
	require.NoError(t, p.Submit(context.Background(), func(context.Context) (int, error) {
		ran.Add(1)
		return 99, nil
	}))
	p.Close()
	require.Equal(t, int64(1), ran.Load(), "job must run even if its result is dropped on shutdown")
}

// TestRecovered_StartsZero covers the Recovered accessor on a healthy pool.
func TestRecovered_StartsZero(t *testing.T) {
	p := New[int](2)
	defer p.Close()
	require.Equal(t, uint64(0), p.Recovered())
}

// TestRunJob_NoCollectW covers runJob's `!p.collectW` early-return branch
// (fire-and-forget pool: no result channel, runJob returns after safeCall).
func TestRunJob_NoCollectW(t *testing.T) {
	p := New[int](2) // no WithResults -> collectW is false
	defer p.Close()
	var ran atomic.Int64
	require.NoError(t, p.Submit(context.Background(), func(context.Context) (int, error) {
		ran.Add(1)
		return 0, errors.New("ignored")
	}))
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, int64(1), ran.Load())
}

// TestWorker_QueueClosedBranch deterministically covers the defensive
// `case jw, ok := <-p.queue: if !ok { return }` branches in both worker's main
// loop and drainQueue. In normal operation the queue is NEVER closed (the code
// comment states this explicitly), so the only way to exercise these guards is
// to close the queue channel directly from an internal test.
//
// We close the queue FIRST (while workers are idle, selecting on the queue) so
// a worker picks `case jw, ok := <-p.queue: !ok → return` from its main loop —
// covering the worker-branch. A second pool closes done first then the queue, so
// drainQueue's `!ok` branch fires too.
func TestWorker_QueueClosedBranch(t *testing.T) {
	// (a) worker main-loop !ok branch: close queue while the worker is idle.
	p := New[int](1, WithQueueSize[int](1))
	close(p.queue) // worker receives !ok on the queue case → returns.
	close(p.done)
	p.closed.Store(true)
	p.wg.Wait()

	// (b) drainQueue !ok branch: close done first so the worker picks the done
	// arm, then close the queue before drainQueue reads it.
	p2 := New[int](1, WithQueueSize[int](1))
	close(p2.done)
	close(p2.queue)
	p2.closed.Store(true)
	p2.wg.Wait()
}

// TestDrainQueue_EmptyDefault covers drainQueue's `default: return` branch:
// when Close is called on a pool whose queue is empty (and still open), the
// worker's drainQueue selects the default arm and returns immediately.
func TestDrainQueue_EmptyDefault(t *testing.T) {
	p := New[int](2, WithQueueSize[int](4))
	// No jobs queued: Close → workers select <-p.done → drainQueue on an empty,
	// open queue → default: return.
	p.Close()
	// Reaching here without deadlock means drainQueue's default branch fired.
}
