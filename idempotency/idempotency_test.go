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
	for range 5 {
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
	for i := range n {
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

// TestDo_LeaderPanic_FollowersWakeAndEntryDoesNotLeak is the R13-F1 regression
// test. A panicking leader fn must NOT escape Do before close(e.done): all
// followers wake with an error (instead of hanging until ctx timeout), the
// entry is removed from the map (default cacheErrors=false -> no leak), and a
// fresh Do(key) after the panic re-runs fn and succeeds (key recovered). The
// old code let the panic propagate out of the leader before close(e.done), so
// followers hung forever and the key was permanently stuck.
//
// The leader fn sleeps first so followers reliably pile up behind it; we then
// assert bounded completion (no hang) via a watchdog, Len()==0, and a
// successful retry. The follower fn is distinct and counts invocations: with
// the fix every follower coalesces behind the leader (follower fn runs 0
// times); on the old code the leader panics before close(e.done), so followers
// either hang on <-e.done or, after the entry is gone, become new leaders and
// re-run — both observable as a hang. Run under -race.
func TestDo_LeaderPanic_FollowersWakeAndEntryDoesNotLeak(t *testing.T) {
	c := New[int]()

	leaderStarted := make(chan struct{})
	// The leader fn closes the start signal exactly once (guarded by the
	// atomic Swap) so the test can deterministically wait for the leader to
	// start before launching followers. Followers coalesce via singleflight and
	// never run fn at all.
	var leaderRan atomic.Bool
	leaderFn := func(context.Context) (int, error) {
		if leaderRan.Swap(true) {
			panic("leader fn ran twice — singleflight broken")
		}
		close(leaderStarted)
		time.Sleep(30 * time.Millisecond) // let followers stack up behind the leader
		panic("boom")
	}

	// The follower fn must NEVER run: with the fix all followers coalesce. If
	// the bug lets a follower become a new leader, this counter would rise and
	// the follower's bounded ctx turns that into a timely failure too.
	var followerRan atomic.Int64
	followerFn := func(context.Context) (int, error) { followerRan.Add(1); return 0, nil }

	// Leader goroutine.
	leaderRes := make(chan error, 1)
	go func() {
		_, err := c.Do(context.Background(), "k", leaderFn)
		leaderRes <- err
	}()
	<-leaderStarted

	// Followers pile up behind the in-flight leader. With the bug they hang on
	// <-e.done forever (the leader panicked before close). Each runs with a
	// bounded ctx so the test fails fast on the old code instead of blocking
	// the whole suite; with the fix they wake immediately.
	const n = 5
	followerErrs := make([]error, n)
	followerDone := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		// NOTE: sync.WaitGroup.Go (Go 1.26+) calls Add(1) internally, so we must
		// NOT pre-Add here — doing so would double-count and block Wait forever.
		for i := range n {
			i := i
			wg.Go(func() {
				fctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_, followerErrs[i] = c.Do(fctx, "k", followerFn)
			})
		}
		wg.Wait()
		close(followerDone)
	}()

	// All followers must wake (with an error) within a bounded time. On the old
	// code this select deadlocks until the 2s follower ctxs time out, which the
	// 500ms timeout below catches as a hang.
	select {
	case <-followerDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("followers hung: leader panic escaped Do before close(e.done)")
	}

	// (1) Leader: the recovered panic must surface as an error (not a re-panic).
	select {
	case err := <-leaderRes:
		require.Error(t, err, "leader must return the recovered panic as an error")
		require.Contains(t, err.Error(), "fn panic recovered")
	default:
		t.Fatal("leader goroutine did not return")
	}

	// (1b) Followers: every follower must wake with an error (the leader's
	// recovered-panic error), not hang and not succeed with a zero value.
	for i, err := range followerErrs {
		require.Error(t, err, "follower %d must wake with the leader's error, not hang", i)
	}

	// Singleflight intact: the follower fn never ran.
	require.Equal(t, int64(0), followerRan.Load(), "followers must coalesce, not re-run fn")

	// (2) No leak: with cacheErrors=false the failed entry must be gone so the
	// next Do re-runs fn.
	require.Equal(t, 0, c.Len(), "failed entry must be removed (default cacheErrors=false)")
	require.Equal(t, uint64(1), c.Recovered(), "exactly one leader panic recovered")

	// (3) Key recovered: a fresh Do(key) re-runs fn and succeeds.
	var calls atomic.Int64
	okFn := func(context.Context) (int, error) { calls.Add(1); return 7, nil }
	v, err := c.Do(context.Background(), "k", okFn)
	require.NoError(t, err, "key must be reusable after a recovered leader panic")
	require.Equal(t, 7, v)
	require.Equal(t, int64(1), calls.Load())
}

// TestDo_LeaderPanic_SetOnPanic_FiredAndCounted covers the L5 observability
// hooks (parity with workerpool/pipeline/signalbus): a leader fn panic is
// counted in Recovered and surfaced via the onPanic hook (non-blocking). The
// hook fires on the leader's goroutine, so the test waits on a signal.
func TestDo_LeaderPanic_SetOnPanic_FiredAndCounted(t *testing.T) {
	c := New[int]()
	require.Equal(t, uint64(0), c.Recovered(), "Recovered starts at 0")

	var hookFired atomic.Bool
	var got atomic.Value // any
	c.SetOnPanic(func(r any) {
		got.Store(r)
		hookFired.Store(true)
	})

	panicFn := func(context.Context) (int, error) { panic("kaboom") }
	_, err := c.Do(context.Background(), "k", panicFn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fn panic recovered")

	require.Eventually(t, hookFired.Load, 200*time.Millisecond, time.Millisecond,
		"onPanic hook must fire")
	require.Equal(t, "kaboom", got.Load(), "hook receives the recovered value")
	require.Equal(t, uint64(1), c.Recovered(), "Recovered counts the panic")

	// SetOnPanic(nil) clears the hook; a second panic must not fire the (now
	// nil) hook but must still be counted.
	c.SetOnPanic(nil)
	hookFired.Store(false)
	_, err = c.Do(context.Background(), "k2", panicFn)
	require.Error(t, err)
	require.False(t, hookFired.Load(), "nil hook must not fire")
	require.Equal(t, uint64(2), c.Recovered(), "second panic counted even with nil hook")
}

// TestDo_LeaderPanic_WithCacheErrors_KeepsEntry covers the cacheErrors=true
// branch in finishLocked: a recovered leader panic is treated as a normal
// failure and CACHED (hard de-dup), so the entry stays and the next Do serves
// the same error WITHOUT re-running fn. Without the fix, the entry would leak
// in a stuck (done-never-closed) state.
func TestDo_LeaderPanic_WithCacheErrors_KeepsEntry(t *testing.T) {
	var calls atomic.Int64
	c := New[int](WithCacheErrors[int](true))
	panicFn := func(context.Context) (int, error) {
		calls.Add(1)
		panic("boom")
	}

	_, err := c.Do(context.Background(), "k", panicFn)
	require.Error(t, err)
	require.Equal(t, int64(1), calls.Load())
	require.Equal(t, 1, c.Len(), "cacheErrors=true keeps the failed entry")

	// Second Do serves the cached panic-error; fn must NOT re-run.
	_, err2 := c.Do(context.Background(), "k", panicFn)
	require.Error(t, err2)
	require.Equal(t, int64(1), calls.Load(), "cached failure not retried")
	require.Equal(t, 1, c.Len(), "entry still present")
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
