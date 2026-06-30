package redislock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/redislock"
)

func newClient(t *testing.T) (goredis.Cmdable, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	c := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, c.Close()) })
	return c, mr
}

func TestTryLock_AcquireAndRelease(t *testing.T) {
	client, _ := newClient(t)
	lk := redislock.New(client)
	ctx := context.Background()

	lock, err := lk.TryLock(ctx, "k1")
	require.NoError(t, err)
	require.Equal(t, "k1", lock.Key())
	require.Len(t, lock.Token(), 32)
	require.NoError(t, lock.Release(ctx))
}

func TestTryLock_AlreadyHeld(t *testing.T) {
	client, _ := newClient(t)
	lk := redislock.New(client)
	ctx := context.Background()

	_, err := lk.TryLock(ctx, "k")
	require.NoError(t, err)
	_, err = lk.TryLock(ctx, "k")
	require.ErrorIs(t, err, redislock.ErrLockNotAcquired)
}

// Re-acquiring after Release succeeds.
func TestReacquireAfterRelease(t *testing.T) {
	client, _ := newClient(t)
	lk := redislock.New(client)
	ctx := context.Background()

	lock, err := lk.TryLock(ctx, "k")
	require.NoError(t, err)
	require.NoError(t, lock.Release(ctx))
	_, err = lk.TryLock(ctx, "k")
	require.NoError(t, err)
}

// A second holder with a different token must NOT be able to release the first's lock.
func TestReleaseOnlyByOwner(t *testing.T) {
	client, _ := newClient(t)
	// Use an explicit token to simulate a foreign holder.
	owner := redislock.New(client, redislock.WithToken("owner-token"))
	ctx := context.Background()

	lock, err := owner.TryLock(ctx, "k")
	require.NoError(t, err)

	// Foreign actor sets a DIFFERENT token behind our back (simulating expiry +
	// re-acquisition by someone else): writing directly, then our Release must
	// report not-acquired and NOT delete the foreign lock.
	require.NoError(t, client.Set(ctx, "k", "someone-else", time.Minute).Err())
	err = lock.Release(ctx)
	require.ErrorIs(t, err, redislock.ErrLockNotAcquired)

	// The foreign lock is still there.
	got, err := client.Get(ctx, "k").Result()
	require.NoError(t, err)
	require.Equal(t, "someone-else", got)
}

func TestLock_BlockingAcquire(t *testing.T) {
	client, _ := newClient(t)
	lk := redislock.New(client, redislock.WithRetryInterval(10*time.Millisecond))
	ctx := context.Background()

	first, err := lk.TryLock(ctx, "k")
	require.NoError(t, err)

	// Release the first after a short delay; the blocking Lock should then win.
	go func() {
		time.Sleep(40 * time.Millisecond)
		require.NoError(t, first.Release(ctx))
	}()

	start := time.Now()
	second, err := lk.Lock(ctx, "k")
	require.NoError(t, err)
	require.Less(t, time.Since(start), 500*time.Millisecond) // got it once first released
	require.NoError(t, second.Release(ctx))
}

func TestLock_WaitTimeout(t *testing.T) {
	client, _ := newClient(t)
	lk := redislock.New(client,
		redislock.WithRetryInterval(10*time.Millisecond),
		redislock.WithWaitTimeout(80*time.Millisecond),
	)
	ctx := context.Background()

	_, err := lk.TryLock(ctx, "k")
	require.NoError(t, err)

	_, err = lk.Lock(ctx, "k")
	require.ErrorIs(t, err, redislock.ErrLockNotAcquired)
}

func TestLock_ContextCancelled(t *testing.T) {
	client, _ := newClient(t)
	lk := redislock.New(client, redislock.WithRetryInterval(50*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())

	_, err := lk.TryLock(ctx, "k")
	require.NoError(t, err)

	cancel()
	_, err = lk.Lock(ctx, "k")
	require.ErrorIs(t, err, context.Canceled)
}

func TestRefresh(t *testing.T) {
	client, _ := newClient(t)
	lk := redislock.New(client, redislock.WithTTL(2*time.Second))
	ctx := context.Background()

	lock, err := lk.TryLock(ctx, "k")
	require.NoError(t, err)
	require.NoError(t, lock.Refresh(ctx))

	// After Release, Refresh reports not acquired.
	require.NoError(t, lock.Release(ctx))
	err = lock.Refresh(ctx)
	require.ErrorIs(t, err, redislock.ErrLockNotAcquired)
}

func TestAutoRenew_PreventsExpiry(t *testing.T) {
	client, _ := newClient(t)
	// TTL 200ms, renew every 50ms. Without renewal the lock would expire at
	// ~200ms; the renewer keeps it alive past that.
	lk := redislock.New(client,
		redislock.WithTTL(200*time.Millisecond),
		redislock.WithAutoRenew(true),
		redislock.WithRenewInterval(50*time.Millisecond),
	)
	ctx := context.Background()

	lock, err := lk.TryLock(ctx, "k")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lock.Release(ctx) })

	// Sleep well past the original TTL (renewer fires several times meanwhile).
	time.Sleep(600 * time.Millisecond)

	// A second locker must still fail to acquire -> the renewer kept it held.
	contender := redislock.New(client)
	_, err = contender.TryLock(ctx, "k")
	require.ErrorIs(t, err, redislock.ErrLockNotAcquired)

	select {
	case <-lock.Lost():
		t.Fatal("lock reported lost while it should be renewed")
	default:
	}
}

func TestAutoRenew_LostWhenForcefullyRemoved(t *testing.T) {
	client, mr := newClient(t)
	var lostErr error
	lk := redislock.New(client,
		redislock.WithTTL(150*time.Millisecond),
		redislock.WithAutoRenew(true),
		redislock.WithRenewInterval(50*time.Millisecond),
		redislock.WithOnLost(func(e error) { lostErr = e }),
	)
	ctx := context.Background()

	lock, err := lk.TryLock(ctx, "k")
	require.NoError(t, err)

	// A foreign DEL removes the lock; the next renew must fail -> Lost closed.
	mr.Del("k")

	select {
	case <-lock.Lost():
	case <-time.After(time.Second):
		t.Fatal("expected Lost() to close after renewal failure")
	}
	require.Error(t, lostErr)
	require.ErrorIs(t, lostErr, redislock.ErrLockNotAcquired)
}

func TestAutoRenew_DefaultIntervalFromTTL(t *testing.T) {
	client, _ := newClient(t)
	// No WithRenewInterval: the renewer defaults to ttl/2.
	lk := redislock.New(client, redislock.WithTTL(300*time.Millisecond), redislock.WithAutoRenew(true))
	lock, err := lk.TryLock(context.Background(), "k")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lock.Release(context.Background()) })

	// Sleep past the original TTL; the default-interval renewer keeps it held.
	time.Sleep(700 * time.Millisecond)
	contender := redislock.New(client)
	_, err = contender.TryLock(context.Background(), "k")
	require.ErrorIs(t, err, redislock.ErrLockNotAcquired)
}

func TestExplicitToken(t *testing.T) {
	client, _ := newClient(t)
	lk := redislock.New(client, redislock.WithToken("fixed"))
	lock, err := lk.TryLock(context.Background(), "k")
	require.NoError(t, err)
	require.Equal(t, "fixed", lock.Token())
}

func TestReleaseTwiceSafe(t *testing.T) {
	client, _ := newClient(t)
	lk := redislock.New(client)
	ctx := context.Background()
	lock, err := lk.TryLock(ctx, "k")
	require.NoError(t, err)
	require.NoError(t, lock.Release(ctx))
	// Second release: lock no longer owned -> ErrLockNotAcquired (not a panic).
	err = lock.Release(ctx)
	require.ErrorIs(t, err, redislock.ErrLockNotAcquired)
}

func TestErrSentinels(t *testing.T) {
	require.True(t, errors.Is(redislock.ErrLockNotAcquired, redislock.ErrLockNotAcquired))
	require.True(t, errors.Is(redislock.ErrLockLost, redislock.ErrLockLost))
}
