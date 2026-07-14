package singleflight_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/singleflight"
)

// TestDoDeduplicates is the core invariant: N goroutines racing on the same key
// cause fn to run EXACTLY ONCE; every caller gets the same result.
func TestDoDeduplicates(t *testing.T) {
	g := singleflight.New[string, int]()
	var runs atomic.Int64
	// Block fn until many callers are in flight, guaranteeing overlap.
	gate := make(chan struct{})

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})
	results := make([]singleflight.Result[int], n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = g.Do("k", func() (int, error) {
				runs.Add(1)
				<-gate // hold the in-flight call open until all callers register
				return 42, nil
			})
		}(i)
	}

	close(start)
	// Give callers time to pile up behind the in-flight call.
	time.Sleep(20 * time.Millisecond)
	close(gate) // release fn
	wg.Wait()

	require.Equal(t, int64(1), runs.Load(), "fn must run exactly once under contention")
	exactlyOneLeader := 0
	for _, r := range results {
		require.NoError(t, r.Err)
		require.Equal(t, 42, r.Value)
		if !r.Shared {
			exactlyOneLeader++
		}
	}
	require.Equal(t, 1, exactlyOneLeader, "exactly one caller is the leader (Shared=false)")
}

func TestDoNoCaching(t *testing.T) {
	g := singleflight.New[string, int]()
	var runs atomic.Int64

	r1 := g.Do("k", func() (int, error) {
		runs.Add(1)
		return 1, nil
	})
	r2 := g.Do("k", func() (int, error) {
		runs.Add(1)
		return 2, nil
	})

	require.False(t, r1.Shared) // first call is the leader
	require.False(t, r2.Shared) // second call runs fn again (no caching)
	require.Equal(t, 1, r1.Value)
	require.Equal(t, 2, r2.Value)
	require.Equal(t, int64(2), runs.Load())
}

func TestDoDifferentKeysSeparate(t *testing.T) {
	g := singleflight.New[string, int]()
	var runs atomic.Int64
	var wg sync.WaitGroup

	for _, k := range []string{"a", "b", "c"} {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			g.Do(k, func() (int, error) {
				runs.Add(1)
				return len(k), nil
			})
		}(k)
	}
	wg.Wait()

	require.Equal(t, int64(3), runs.Load(), "different keys each run fn once")
}

func TestDoPropagatesError(t *testing.T) {
	g := singleflight.New[string, int]()
	wantErr := errors.New("boom")
	gate := make(chan struct{})

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})
	errs := make([]error, n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			<-start
			r := g.Do("k", func() (int, error) {
				<-gate
				return 0, wantErr
			})
			errs[i] = r.Err
		}(i)
	}
	close(start)
	time.Sleep(10 * time.Millisecond)
	close(gate)
	wg.Wait()

	for _, err := range errs {
		require.ErrorIs(t, err, wantErr, "all callers share the leader's error")
	}
}

func TestDoSerial(t *testing.T) {
	g := singleflight.New[int, string]()
	r := g.Do(1, func() (string, error) { return "ok", nil })
	require.Equal(t, "ok", r.Value)
	require.False(t, r.Shared)
}

// TestDoConcurrentNoRace is a -race smoke test across many keys and goroutines.
func TestDoConcurrentNoRace(t *testing.T) {
	g := singleflight.New[int, int]()
	var wg sync.WaitGroup
	for i := range 200 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			g.Do(i%20, func() (int, error) { return i, nil }) //nolint:errcheck // result unused
		}(i)
	}
	wg.Wait()
}

// TestDoPanicDoesNotDeadlock is the regression test for the panic-safety P1: if
// fn panics, waiters must NOT hang (wg.Done must run) and the key must be
// cleaned (no permanent starvation). The panic surfaces as Result.Err.
func TestDoPanicDoesNotDeadlock(t *testing.T) {
	g := singleflight.New[string, int]()
	var runs atomic.Int64

	const n = 5
	results := make([]singleflight.Result[int], n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = g.Do("k", func() (int, error) {
				runs.Add(1)
				panic("boom")
			})
		}(i)
	}
	close(start)

	// Pre-fix: waiters hung forever on a wg.Done that never ran.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Do deadlocked on a panicking fn — waiters hung (P1 regression)")
	}

	require.NotZero(t, runs.Load(), "fn ran at least once (the leader)")
	for _, r := range results {
		require.Error(t, r.Err, "panic surfaced as Result.Err, not a hang")
		require.Contains(t, r.Err.Error(), "boom")
	}

	// The key was cleaned up — a later Do re-runs fn (not starved by a dead entry).
	r2 := g.Do("k", func() (int, error) { return 7, nil })
	require.NoError(t, r2.Err)
	require.Equal(t, 7, r2.Value)
}
