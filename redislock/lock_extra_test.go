package redislock_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/redislock"
)

// closedClient returns a Cmdable whose backing miniredis is already closed,
// so every command errors. Used to exercise the error branches in tryLock /
// Lock / Refresh / Release. MaxRetries=1 + small dial timeout keeps failures
// fast (go-redis' default retry-with-backoff would otherwise take ~1.5s).
func closedClient(t *testing.T) goredis.Cmdable {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	addr := mr.Addr()
	mr.Close() // shut down now
	c := goredis.NewClient(&goredis.Options{
		Addr:        addr,
		DialTimeout: 20 * time.Millisecond,
		MaxRetries:  -1, // no retries — surface the dial failure immediately
	})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// TestTryLock_SetNXError exercises the l.client.SetNX error branch in tryLock.
func TestTryLock_SetNXError(t *testing.T) {
	lk := redislock.New(closedClient(t))
	_, err := lk.TryLock(context.Background(), "k")
	require.Error(t, err)
	require.False(t, errors.Is(err, redislock.ErrLockNotAcquired),
		"a transport error must not be reported as ErrLockNotAcquired")
}

// TestLock_NonNotAcquiredError covers the `if !errors.Is(err, ErrLockNotAcquired)`
// branch in Lock: when tryLock returns a non-not-acquired error, Lock must
// surface it immediately instead of retrying.
func TestLock_NonNotAcquiredError(t *testing.T) {
	lk := redislock.New(closedClient(t),
		redislock.WithRetryInterval(10*time.Millisecond),
		redislock.WithWaitTimeout(200*time.Millisecond),
	)
	start := time.Now()
	_, err := lk.Lock(context.Background(), "k")
	require.Error(t, err)
	require.False(t, errors.Is(err, redislock.ErrLockNotAcquired),
		"a transport error must not be reported as ErrLockNotAcquired")
	// Must return promptly (no full waitTimeout retry loop).
	require.Less(t, time.Since(start), 2*time.Second)
}

// TestLock_RetryIntervalFallback covers the interval<=0 -> 50ms branch in Lock.
// With a held lock and waitTimeout>0, the fallback retry cadence still lets the
// caller time out; we mainly exercise the branch by setting retryInterval=0.
func TestLock_RetryIntervalFallback(t *testing.T) {
	client, _ := newClient(t)
	lk := redislock.New(client,
		redislock.WithRetryInterval(0), // forces the <= 0 fallback to 50ms
		redislock.WithWaitTimeout(60*time.Millisecond),
	)
	_, err := lk.TryLock(context.Background(), "k")
	require.NoError(t, err)
	_, err = lk.Lock(context.Background(), "k")
	require.ErrorIs(t, err, redislock.ErrLockNotAcquired)
}

// TestRefresh_ScriptError covers the refreshScript.Run error branch in Refresh.
func TestRefresh_ScriptError(t *testing.T) {
	// Acquire against a live miniredis, then close it before Refresh to force
	// a transport error from the script call.
	mr, err := miniredis.Run()
	require.NoError(t, err)
	c := goredis.NewClient(&goredis.Options{
		Addr:        mr.Addr(),
		MaxRetries:  -1,
		DialTimeout: 20 * time.Millisecond,
	})
	t.Cleanup(func() { _ = c.Close() })

	lk := redislock.New(c)
	lock, err := lk.TryLock(context.Background(), "k")
	require.NoError(t, err)

	mr.Close() // break the connection
	err = lock.Refresh(context.Background())
	require.Error(t, err)
	require.False(t, errors.Is(err, redislock.ErrLockNotAcquired))
}

// TestRelease_ScriptError covers the releaseScript.Run error branch (non-Nil)
// in Release. Closing the connection makes the script call fail.
func TestRelease_ScriptError(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	c := goredis.NewClient(&goredis.Options{
		Addr:        mr.Addr(),
		MaxRetries:  -1,
		DialTimeout: 20 * time.Millisecond,
	})
	t.Cleanup(func() { _ = c.Close() })

	lk := redislock.New(c)
	lock, err := lk.TryLock(context.Background(), "k")
	require.NoError(t, err)

	mr.Close() // break the connection
	err = lock.Release(context.Background())
	require.Error(t, err)
	require.False(t, errors.Is(err, redislock.ErrLockNotAcquired))
}

// TestAutoRenew_AcqCtxCancelled covers the `<-ctx.Done()` branch in
// startRenewer: cancelling the acquire context (without a clean Release) must
// be treated as a loss — onLost fires and Lost() closes.
func TestAutoRenew_AcqCtxCancelled(t *testing.T) {
	client, _ := newClient(t)
	var fired atomic.Int64
	lk := redislock.New(client,
		redislock.WithTTL(2*time.Second),
		redislock.WithAutoRenew(true),
		redislock.WithRenewInterval(50*time.Millisecond),
		redislock.WithOnLost(func(error) { fired.Add(1) }),
	)

	ctx, cancel := context.WithCancel(context.Background())
	lock, err := lk.TryLock(ctx, "k")
	require.NoError(t, err)

	// Cancel the acquire context (simulates request/shutdown cancellation).
	cancel()

	select {
	case <-lock.Lost():
	case <-time.After(time.Second):
		t.Fatal("expected Lost() to close after acqCtx cancellation")
	}
	require.Equal(t, int64(1), fired.Load(), "onLost should fire exactly once on acqCtx cancel")
}

// TestAutoRenew_RenewIntervalZeroTTLZero covers the interval<=0 -> 1s fallback
// in startRenewer by using TTL=0 and no explicit RenewInterval. (We do not
// assert timing — we only exercise the branch and confirm the renewer starts.)
func TestAutoRenew_RenewIntervalZeroTTLZero(t *testing.T) {
	client, _ := newClient(t)
	lk := redislock.New(client,
		redislock.WithTTL(0),
		redislock.WithAutoRenew(true),
		// no WithRenewInterval -> interval is ttl/2 = 0 -> falls back to 1s
	)
	lock, err := lk.TryLock(context.Background(), "k")
	require.NoError(t, err)
	// Give the renewer time to start and tick at least once; the 1s fallback is
	// too long to wait, so we just confirm the renewer is wired by checking Lost
	// stays open (no panic, no spurious close) for a short window.
	select {
	case <-lock.Lost():
		t.Fatal("Lost() should not close without a real loss")
	case <-time.After(80 * time.Millisecond):
	}
	require.NoError(t, lock.Release(context.Background()))
}

// TestHandleLoss_CleanShutdown covers the `<-l.stop` return branch in
// handleLoss: a clean Release (which closes stop) must not fire onLost even if
// the renewer passes through handleLoss. This complements
// TestAutoRenew_ReleaseDoesNotFireOnLost by exercising the explicit branch.
func TestHandleLoss_CleanShutdown(t *testing.T) {
	client, _ := newClient(t)
	var fired atomic.Int64
	lk := redislock.New(client,
		redislock.WithTTL(100*time.Millisecond),
		redislock.WithAutoRenew(true),
		redislock.WithRenewInterval(10*time.Millisecond),
		redislock.WithOnLost(func(error) { fired.Add(1) }),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lock, err := lk.TryLock(ctx, "k")
	require.NoError(t, err)

	// Release cleanly; the renewer's next tick sees stop closed and returns.
	require.NoError(t, lock.Release(ctx))
	// Give the renewer ample time to observe stop / process any in-flight tick.
	time.Sleep(60 * time.Millisecond)
	require.Equal(t, int64(0), fired.Load(), "onLost must not fire on clean Release")
}
