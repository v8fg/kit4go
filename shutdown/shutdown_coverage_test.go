package shutdown

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStop_ResolveLockedError covers the resolveLocked branch of Stop: when
// Start was never called and the graph has a cycle, Stop returns the resolve
// error instead of running stop hooks.
func TestStop_ResolveLockedError(t *testing.T) {
	m := New()
	m.Add("a", nil, func(context.Context) error { return errors.New("should-not-run") }, "b")
	m.Add("b", nil, func(context.Context) error { return errors.New("should-not-run") }, "a")
	err := m.Stop(context.Background())
	require.ErrorIs(t, err, ErrCycle)
}

// TestStart_AlreadyResolved covers the resolveLocked branch where the order is
// already cached from a prior Start (m.order != nil): the second Start reuses
// it without re-resolving.
func TestStart_AlreadyResolved(t *testing.T) {
	var rec recorder
	m := New()
	m.Add("a", func(context.Context) error { rec.record("start:a"); return nil }, func(context.Context) error { rec.record("stop:a"); return nil })
	m.Add("b", func(context.Context) error { rec.record("start:b"); return nil }, func(context.Context) error { rec.record("stop:b"); return nil }, "a")
	// First Start resolves + runs hooks.
	require.NoError(t, m.Start(context.Background()))
	require.NoError(t, m.Stop(context.Background()))
	// Second Start reuses the cached order (m.order != nil path).
	require.NoError(t, m.Start(context.Background()))
	require.Equal(t,
		[]string{"start:a", "start:b", "stop:b", "stop:a", "start:a", "start:b"},
		rec.slice(),
	)
	require.NoError(t, m.Stop(context.Background()))
}

// TestStart_ContextCancelledDuringStart covers the ctx.Err() != nil branch
// inside the start loop: after a component starts, if the parent ctx is already
// cancelled, the remaining components are not started and a rollback occurs.
func TestStart_ContextCancelledDuringStart(t *testing.T) {
	var rec recorder
	ctx, cancel := context.WithCancel(context.Background())
	m := New(WithStartTimeout(time.Second))
	m.Add("a", func(context.Context) error {
		rec.record("start:a")
		cancel() // cancel the parent ctx mid-startup
		return nil
	}, func(context.Context) error { rec.record("stop:a"); return nil })
	m.Add("b", func(context.Context) error {
		rec.record("start:b") // must NOT run
		return nil
	}, nil, "a")
	err := m.Start(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, rec.slice(), "start:b")
	require.Contains(t, rec.slice(), "stop:a") // rollback stopped a
}

// TestSafeStop_PanicRecovery covers the panic-recovery branch of safeStop: a
// panicking stop hook is turned into an error and aggregated, so the remaining
// components still stop.
func TestSafeStop_PanicRecovery(t *testing.T) {
	var rec recorder
	m := New(WithStopTimeout(time.Second))
	m.Add("panicker", nil, func(context.Context) error {
		rec.record("panicker:before")
		panic("kaboom")
	})
	m.Add("after", nil, func(context.Context) error {
		rec.record("after:stop")
		return nil
	}, "panicker")
	err := m.Stop(context.Background())
	require.Error(t, err)
	var se *ErrShutdown
	require.ErrorAs(t, err, &se)
	require.Contains(t, se.Errors[0].Err.Error(), "panic recovered")
	// the dependent component still stopped
	require.Contains(t, rec.slice(), "after:stop")
}

// TestRun_StartFails covers the early-return branch of Run when Start returns
// an error (Run propagates the start error and does not run Stop).
func TestRun_StartFails(t *testing.T) {
	startErr := errors.New("startup failure")
	m := New()
	m.Add("svc", func(context.Context) error { return startErr }, func(context.Context) error {
		t.Fatal("Stop must not run when Start failed")
		return nil
	})
	err := m.Run(context.Background())
	require.ErrorIs(t, err, startErr)
}

// TestRunWithSignalAndExplicitSet covers the WithSignal branch with an explicit
// signal set passed in (not the default).
func TestRunWithSignalAndExplicitSet(t *testing.T) {
	m := New(WithSignal(os.Interrupt, os.Kill))
	require.Len(t, m.signals, 2)
}

// TestAdd_AfterNew covers Add when m.byName is initialized via New (the common
// path) — already covered, but this also pins the duplicate-error wrapping.
func TestAdd_DuplicateWrapsError(t *testing.T) {
	m := New()
	require.NoError(t, m.Add("a", nil, nil))
	err := m.Add("a", nil, nil)
	require.ErrorIs(t, err, ErrDuplicate)
	require.Contains(t, err.Error(), "a")
}

// TestStopReverse_NoHooks covers stopReverse with a mix of nil and non-nil stop
// hooks (nil hooks are skipped silently).
func TestStopReverse_NoHooks(t *testing.T) {
	var rec recorder
	m := New(WithStopTimeout(time.Second))
	m.Add("a", nil, nil) // nil stop hook
	m.Add("b", nil, func(context.Context) error { rec.record("stop:b"); return nil }, "a")
	require.NoError(t, m.Stop(context.Background()))
	require.Equal(t, []string{"stop:b"}, rec.slice())
}

// TestComponents_ResolvedByStart covers Components returning the cached order
// after Start has run (m.order != nil).
func TestComponents_ResolvedByStart(t *testing.T) {
	m := New()
	m.Add("a", func(context.Context) error { return nil }, nil)
	m.Add("b", func(context.Context) error { return nil }, nil, "a")
	require.NoError(t, m.Start(context.Background()))
	defer m.Stop(context.Background())
	order, err := m.Components()
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, order)
}

// TestReverseStrings covers reverseStrings directly.
func TestReverseStrings(t *testing.T) {
	in := []string{"a", "b", "c"}
	out := reverseStrings(in)
	require.Equal(t, []string{"c", "b", "a"}, out)
	require.Equal(t, []string{"a", "b", "c"}, in, "input must not be mutated")
	require.Empty(t, reverseStrings(nil))
}

// TestRun_NoSignal covers Run without WithSignal: it blocks on ctx.Done() and
// then runs Stop. (Functionally overlaps TestRunCancelsAndStops but pins the
// no-signal branch explicitly.)
func TestRun_NoSignal(t *testing.T) {
	var rec recorder
	m := New()
	m.Add("svc", func(context.Context) error { rec.record("start"); return nil }, func(context.Context) error { rec.record("stop"); return nil })
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = m.Run(ctx)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	wg.Wait()
	require.Equal(t, []string{"start", "stop"}, rec.slice())
}

// TestSafeStop_NoPanic covers safeStop with a non-panicking stop returning nil.
func TestSafeStop_NoPanic(t *testing.T) {
	called := false
	err := safeStop(func(context.Context) error {
		called = true
		return nil
	}, context.Background())
	require.NoError(t, err)
	require.True(t, called)
}

// TestSafeStop_ReturnsError covers safeStop propagating a returned error
// (without panic).
func TestSafeStop_ReturnsError(t *testing.T) {
	stopErr := errors.New("stop failed")
	err := safeStop(func(context.Context) error { return stopErr }, context.Background())
	require.ErrorIs(t, err, stopErr)
}

// TestRun_StopReturnsError covers the path where Run reaches Stop and Stop
// returns an aggregated *ErrShutdown.
func TestRun_StopReturnsError(t *testing.T) {
	m := New()
	m.Add("svc", func(context.Context) error { return nil }, func(context.Context) error {
		return errors.New("shutdown failed")
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := m.Run(ctx)
	require.Error(t, err)
	var se *ErrShutdown
	require.ErrorAs(t, err, &se)
}

// TestRunWithSignal_ContextCancelNoSignal covers the signal goroutine's
// <-ctx.Done() branch: WithSignal is configured but the context is cancelled
// directly (no OS signal is sent), so the goroutine exits via ctx.Done().
func TestRunWithSignal_ContextCancelNoSignal(t *testing.T) {
	var rec recorder
	m := New(WithSignal(os.Interrupt), WithStopTimeout(time.Second))
	m.Add("svc", func(context.Context) error { rec.record("start"); return nil }, func(context.Context) error { rec.record("stop"); return nil })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	time.Sleep(30 * time.Millisecond) // let Start run and the signal goroutine park
	cancel()                          // cancel without sending a signal -> goroutine exits via ctx.Done()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
	require.Equal(t, []string{"start", "stop"}, rec.slice())
}
