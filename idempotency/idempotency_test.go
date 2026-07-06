package idempotency

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// withClock injects a clock for deterministic TTL tests (test-only).
func withClock[V any](f func() time.Time) Option[V] {
	return func(c *Cache[V]) { c.clock = f }
}

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }

func TestDo_RunsOnceSequentialRepeat(t *testing.T) {
	var calls atomic.Int64
	c := New[int]()
	slow := func(context.Context) (int, error) { calls.Add(1); return 42, nil }

	v, err := c.Do(context.Background(), "k", slow)
	require.NoError(t, err)
	require.Equal(t, 42, v)
	// Repeats within TTL serve from cache (no new call).
	for i := 0; i < 5; i++ {
		v, err = c.Do(context.Background(), "k", slow)
		require.NoError(t, err)
		require.Equal(t, 42, v)
	}
	require.Equal(t, int64(1), calls.Load(), "fn ran only once")
}

// The core singleflight guarantee: concurrent callers for the same key run fn
// exactly once and all observe the same result. Run under -race to verify the
// shared entry is safe.
func TestDo_ConcurrentCoalescing_RunsOnce(t *testing.T) {
	var calls atomic.Int64
	var block atomic.Bool
	c := New[string]()

	work := func(ctx context.Context) (string, error) {
		calls.Add(1)
		// Hold until released so followers pile up behind the leader.
		for block.Load() {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			time.Sleep(time.Millisecond)
		}
		return "ok", nil
	}

	const n = 50
	var wg sync.WaitGroup
	results := make([]string, n)
	errs := make([]error, n)
	start := make(chan struct{})
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = c.Do(context.Background(), "auction-7", work)
		}()
	}
	time.Sleep(20 * time.Millisecond) // let goroutines park on <‑start
	block.Store(true)                 // leader will spin in the block loop
	close(start)
	time.Sleep(30 * time.Millisecond) // followers pile up behind the leader
	block.Store(false)                // release the leader
	wg.Wait()

	require.Equal(t, int64(1), calls.Load(), "fn must run exactly once across %d concurrent callers", n)
	for i := range results {
		require.NoError(t, errs[i])
		require.Equal(t, "ok", results[i])
	}
}

func TestDo_ErrorNotCached_Retries(t *testing.T) {
	var calls atomic.Int64
	c := New[int]()
	flakey := func(context.Context) (int, error) {
		n := calls.Add(1)
		if n < 3 {
			return 0, errors.New("boom")
		}
		return 7, nil
	}
	_, err := c.Do(context.Background(), "k", flakey) // call 1 -> error
	require.Error(t, err)
	_, err = c.Do(context.Background(), "k", flakey) // call 2 -> error (retried, not cached)
	require.Error(t, err)
	v, err := c.Do(context.Background(), "k", flakey) // call 3 -> success
	require.NoError(t, err)
	require.Equal(t, 7, v)
	require.Equal(t, int64(3), calls.Load())
}

func TestDo_CacheErrors_WhenEnabled(t *testing.T) {
	var calls atomic.Int64
	c := New[int](WithCacheErrors[int](true))
	f := func(context.Context) (int, error) { calls.Add(1); return 0, errors.New("hard fail") }
	_, err := c.Do(context.Background(), "k", f)
	require.Error(t, err)
	_, err = c.Do(context.Background(), "k", f) // cached error -> not retried
	require.Error(t, err)
	require.Equal(t, int64(1), calls.Load())
}

func TestTTLExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var calls atomic.Int64
	c := New[int](WithTTL[int](10*time.Second), withClock[int](clk.now))
	c.Do(context.Background(), "k", func(context.Context) (int, error) { calls.Add(1); return 1, nil })
	c.Do(context.Background(), "k", func(context.Context) (int, error) { calls.Add(1); return 2, nil })
	require.Equal(t, int64(1), calls.Load()) // cached

	clk.t = clk.t.Add(20 * time.Second) // expired
	c.Do(context.Background(), "k", func(context.Context) (int, error) { calls.Add(1); return 3, nil })
	require.Equal(t, int64(2), calls.Load()) // re-ran after TTL
}

func TestTTLZero_NoExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(2000, 0)}
	var calls atomic.Int64
	c := New[int](WithTTL[int](0), withClock[int](clk.now))
	c.Do(context.Background(), "k", func(context.Context) (int, error) { calls.Add(1); return 1, nil })
	clk.t = clk.t.Add(100 * 365 * 24 * time.Hour)
	c.Do(context.Background(), "k", func(context.Context) (int, error) { calls.Add(1); return 2, nil })
	require.Equal(t, int64(1), calls.Load())
}

func TestFollowerCtxCancelled(t *testing.T) {
	var calls atomic.Int64
	c := New[string]()
	block := make(chan struct{})
	work := func(ctx context.Context) (string, error) {
		calls.Add(1)
		<-block // leader blocks
		return "ok", nil
	}
	// Leader.
	leaderDone := make(chan struct{})
	go func() {
		c.Do(context.Background(), "k", work)
		close(leaderDone)
	}()
	time.Sleep(20 * time.Millisecond) // let leader become leader

	// Follower with a cancelled ctx: returns ctx.Err() without waiting.
	fctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Do(fctx, "k", work)
	require.ErrorIs(t, err, context.Canceled)

	close(block) // release leader
	<-leaderDone
	require.Equal(t, int64(1), calls.Load()) // follower did not run fn
}

func TestForgetAndClear(t *testing.T) {
	var calls atomic.Int64
	c := New[int]()
	f := func(context.Context) (int, error) { calls.Add(1); return 1, nil }
	c.Do(context.Background(), "k", f)
	c.Forget("k")
	c.Do(context.Background(), "k", f)
	require.Equal(t, int64(2), calls.Load())

	c.Do(context.Background(), "k2", f)
	require.Equal(t, 2, c.Len())
	c.Clear()
	require.Equal(t, 0, c.Len())
}

func TestEviction_ExpiredFirst(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	var calls atomic.Int64
	c := New[int](WithTTL[int](10*time.Second), WithMaxEntries[int](2), withClock[int](clk.now))
	work := func(context.Context) (int, error) { calls.Add(1); return 1, nil }
	c.Do(context.Background(), "a", work)
	clk.t = clk.t.Add(5 * time.Second)
	c.Do(context.Background(), "b", work)
	clk.t = clk.t.Add(6 * time.Second)    // a expired (inserted at t0, ttl 10 -> expired at t>10)
	c.Do(context.Background(), "c", work) // over cap -> evicts expired 'a'
	require.Equal(t, int64(3), calls.Load())
	require.Equal(t, 2, c.Len()) // b and c remain
}

func TestEvictionNeverEvictsInFlight(t *testing.T) {
	// Even with maxEntries=1, an in-flight leader must not be evicted.
	c := New[int](WithMaxEntries[int](1))
	block := make(chan struct{})
	go func() {
		c.Do(context.Background(), "leader", func(context.Context) (int, error) { <-block; return 1, nil })
	}()
	time.Sleep(20 * time.Millisecond)
	c.Do(context.Background(), "other", func(context.Context) (int, error) { return 2, nil })
	require.Equal(t, 2, c.Len()) // both present; in-flight leader not evicted
	close(block)
}

// TestEviction_OldestByExpiry exercises evictLocked's second pass: when the
// cache is over capacity but the expired-pass leaves it over (nothing expired),
// the oldest COMPLETED entry by expiry is dropped. This covers the
// "!found || expiresAt.Before(oldestT)" update and the delete(oldestK) path,
// which the ExpiredFirst test never reaches (there the expired pass alone
// relieves the pressure).
func TestEviction_OldestByExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	var calls atomic.Int64
	c := New[int](WithTTL[int](100*time.Second), WithMaxEntries[int](2), withClock[int](clk.now))
	work := func(context.Context) (int, error) { n := calls.Add(1); return int(n), nil }

	// "a" inserted at t=0 -> expires at 100; "b" at t=10 -> expires at 110.
	// Neither expired at the current clock, so the expired pass cannot fire.
	c.Do(context.Background(), "a", work)
	clk.t = clk.t.Add(10 * time.Second)
	c.Do(context.Background(), "b", work)
	require.Equal(t, int64(2), calls.Load())

	// Insert "c" (still t=10): entries = {a, b (both completed), c (in-flight
	// leader, not completed)}, len 3 > maxEntries 2, nothing expired -> the
	// oldest-completed-by-expiry pass runs and must evict "a" (earliest expiry).
	clk.t = clk.t.Add(1 * time.Second) // t=11; a(100) and b(110) both unexpired
	c.Do(context.Background(), "c", work)

	require.Equal(t, int64(3), calls.Load(), "c ran once")
	require.Equal(t, 2, c.Len(), "back at cap after evicting one completed entry")

	// "a" was evicted (oldest expiry) -> re-running it must execute fn again.
	// "b" must still be cached (served from cache, no new call).
	callsBefore := calls.Load()
	v, err := c.Do(context.Background(), "b", work)
	require.NoError(t, err)
	require.Equal(t, 2, v) // value from original call #2
	require.Equal(t, callsBefore, calls.Load(), "b served from cache, not re-run")

	v, err = c.Do(context.Background(), "a", work)
	require.NoError(t, err)
	require.Equal(t, 4, v) // re-ran as call #4
	require.Equal(t, callsBefore+1, calls.Load(), "a re-ran after being evicted")
}
