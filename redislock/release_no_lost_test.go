package redislock_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/redislock"
)

// Regression: a clean Release must NOT fire onLost or close Lost(). Before the
// fix, an in-flight renew whose Refresh landed just after Release's DEL saw
// ErrLockNotAcquired and spuriously reported a loss — defeating the "fire onLost
// only on a real loss" contract and aborting critical sections on graceful
// shutdown.
func TestAutoRenew_ReleaseDoesNotFireOnLost(t *testing.T) {
	for i := range 5 { // repeat to catch the Release-vs-renew race
		client, _ := newClient(t)
		var lostCount atomic.Int64
		lk := redislock.New(client,
			redislock.WithTTL(120*time.Millisecond),
			redislock.WithAutoRenew(true),
			redislock.WithRenewInterval(20*time.Millisecond), // short: a renew likely overlaps Release
			redislock.WithOnLost(func(error) { lostCount.Add(1) }),
		)
		ctx := context.Background()

		lock, err := lk.TryLock(ctx, "k")
		require.NoError(t, err)
		// Let at least one renew tick land, then release cleanly.
		time.Sleep(30 * time.Millisecond)
		require.NoError(t, lock.Release(ctx))
		// Give the renewer time to process any in-flight Refresh that raced Release.
		time.Sleep(80 * time.Millisecond)

		require.Equal(t, int64(0), lostCount.Load(), "onLost fired on a clean Release (iteration %d)", i)
		select {
		case <-lock.Lost():
			t.Fatalf("Lost() closed on a clean Release (iteration %d)", i)
		default:
		}
	}
}
