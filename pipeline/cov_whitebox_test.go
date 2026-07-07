package pipeline

// White-box coverage tests that exercise internal branches that are racy or
// scheduling-dependent via the public API alone. Package-internal so they can
// observe p.done timing. No production code is changed here.
//
// Uncovered-but-unreachable branch, deliberately NOT tested (documented):
//
//   pipeline.go worker(): `case req, ok := <-p.in: if !ok { return }`
//   (cover profile block 100.11,102.5). This fires only when p.in is closed.
//   Production code never closes p.in — Close() closes p.done, then drain()
//   empties the buffered input non-blocking; workers exit via the <-p.done
//   arm. Closing p.in would race with concurrent Senders (Send writes
//   p.in <- ... and would panic on a closed channel), so the branch is a
//   defensive guard against a hypothetical future API that closes the input
//   channel, not a path reachable through the current public surface. We
//   leave it uncovered rather than add an artificial, race-prone closure.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Covers pipeline.go:145.20 — `case <-p.done:` in process()'s second select.
//
// Reach condition: the worker must be parked in the *second* select of
// process() — i.e. the first select took its `default` arm (p.out full), and
// now neither `p.out <- out` nor `<-p.done` is ready — at the instant Close()
// closes p.done. We make this deterministic with a coordination barrier: the
// stage signals each time it is about to return a result. After we stop
// consuming, the next result wedges the worker in the second select; we wait
// for that signal, give the worker a beat to park, then Close().
func TestProcess_DoneBranchWhileBlockedOnOut(t *testing.T) {
	const workers = 1
	stageAboutToReturn := make(chan struct{}, 16) // stage signals before returning a result

	p := New[int, int](workers, func(ctx context.Context, n int) (int, bool, error) {
		select {
		case stageAboutToReturn <- struct{}{}:
		case <-ctx.Done():
			return 0, false, ctx.Err()
		}
		return n, true, nil
	}, WithInputBuffer[int, int](8), WithOutputBuffer[int, int](1))

	// Enqueue several items; senders block on the small input buffer until the
	// worker drains them, but the output buffer (size 1) fills immediately.
	for i := range 8 {
		go func(i int) { _ = p.Send(context.Background(), i) }(i)
	}

	// Let one result flow through (drain the single-slot output buffer) so we
	// know the pipeline is live, then STOP consuming.
	<-stageAboutToReturn // first result produced
	<-p.Out()            // drain it; buffer is now empty

	// The worker keeps pulling from p.in and producing. The next result
	// re-fills the size-1 out buffer; the one after that finds the buffer
	// full, takes the first select's `default`, and parks in the second
	// select. Wait for two more produced-results so the park has happened.
	<-stageAboutToReturn // refilled the buffer
	<-stageAboutToReturn // this one is parked in the second select
	time.Sleep(40 * time.Millisecond)

	// Close() closes p.done; the parked worker must select the <-p.done arm
	// (line 145) to exit. If it instead waits forever on p.out, Close's
	// wg.Wait deadlocks here.
	closeDone := make(chan struct{})
	go func() { p.Close(); close(closeDone) }()
	select {
	case <-closeDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Close deadlocked: worker did not take <-p.done arm in process()")
	}
	for range p.Out() {
	}
}

// Covers pipeline.go:164.20,165.19 — `case <-ctx.Done(): return ctx.Err()` in
// Send(). Requires a context that is cancelled while Send is blocked waiting
// for room in a full input buffer.
func TestSend_ReturnsCtxErrWhenCancelledWhileBlocked(t *testing.T) {
	// Single worker with a slow stage and a tiny input buffer, so the buffer
	// fills and a subsequent Send blocks.
	p := New[int, int](1, func(ctx context.Context, n int) (int, bool, error) {
		time.Sleep(100 * time.Millisecond)
		return n, true, nil
	}, WithInputBuffer[int, int](1), WithOutputBuffer[int, int](4))
	defer p.Close()

	// Fill the input buffer: first Send is picked up by the worker (slot
	// frees, then re-fills), second Send fills the buffer. A third Send will
	// block.
	require.NoError(t, p.Send(context.Background(), 1))
	require.NoError(t, p.Send(context.Background(), 2))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- p.Send(ctx, 3) }()

	// Ensure the goroutine is parked in Send's select on a full p.in, then
	// cancel the context — Send must wake via ctx.Done() and return ctx.Err().
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled),
			"Send should return ctx.Err() (context.Canceled), got %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("blocked Send did not return after context cancellation")
	}
}

// Strengthens the ctx.Err() coverage against scheduling jitter: a
// pre-cancelled context takes the same ctx.Done() arm immediately when the
// buffer is full (no parked goroutine needed).
func TestSend_PreCancelledCtxOnFullBuffer(t *testing.T) {
	p := New[int, int](1, func(ctx context.Context, n int) (int, bool, error) {
		time.Sleep(80 * time.Millisecond)
		return n, true, nil
	}, WithInputBuffer[int, int](1), WithOutputBuffer[int, int](1))
	defer p.Close()

	require.NoError(t, p.Send(context.Background(), 1))
	require.NoError(t, p.Send(context.Background(), 2)) // buffer now full

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.Send(ctx, 3)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
}
