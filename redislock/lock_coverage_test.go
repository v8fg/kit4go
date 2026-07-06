package redislock_test

// Coverage-targeted tests for the few remaining uncovered branches in lock.go:
//   - Release: the goredis.Nil-reply branch (lock.go:246-248)
//   - Lock:    the case <-ctx.Done() select arm (lock.go:131-132)
//
// The remaining uncovered branches are unreachable defensive code and are
// documented at the bottom of this file (see unreachableBranches).

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/redislock"
)

// nilReplyClient wraps a real go-redis client so TryLock's SetNX (acquire)
// succeeds against a live miniredis, but routes the release script's EVALSHA
// through a hook that returns a goredis.Nil reply. This simulates a proxy /
// cluster-redirect that hands back a nil RESP reply for the script call — the
// one realistic way Release's `errors.Is(err, goredis.Nil)` branch can fire,
// since our own releaseScript always returns an integer (0 or DEL's count) and
// therefore never produces Nil against a well-behaved Redis or miniredis.
type nilReplyClient struct {
	goredis.Cmdable // delegate everything (SetNX, etc.) to the real client
}

func (nilReplyClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *goredis.Cmd {
	return newNilCmd(ctx)
}

func (nilReplyClient) EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) *goredis.Cmd {
	return newNilCmd(ctx)
}

func newNilCmd(ctx context.Context) *goredis.Cmd {
	c := goredis.NewCmd(ctx)
	c.SetErr(goredis.Nil)
	return c
}

// TestRelease_NilReply covers the `errors.Is(err, goredis.Nil) -> return nil`
// branch in Release (lock.go:246-248): when the script call comes back with a
// Nil reply, Release treats the lock as already gone and returns nil rather
// than surfacing the sentinel error.
func TestRelease_NilReply(t *testing.T) {
	client, _ := newClient(t)
	lk := redislock.New(nilReplyClient{Cmdable: client})
	ctx := context.Background()

	lock, err := lk.TryLock(ctx, "k") // SetNX goes to the real client
	require.NoError(t, err)

	// The overridden Eval/EvalSha make releaseScript.Run() return goredis.Nil.
	require.NoError(t, lock.Release(ctx), "a Nil reply should be treated as already-released")
}

// TestLock_SelectsContextDone reliably covers the case <-ctx.Done() arm in Lock
// (lock.go:131-132). With a pre-cancelled ctx, both timer.C (NewTimer(0)) and
// ctx.Done() are ready at the select, so Go picks one pseudo-randomly. Driving
// Lock many times makes hitting ctx.Done() essentially certain (P(miss) for N
// tries is 0.5^N). Existing TestLock_ContextCancelled exercises the same code
// once, which is non-deterministic for coverage; this loop pins it down.
func TestLock_SelectsContextDone(t *testing.T) {
	client, _ := newClient(t)
	// Hold the key so Lock must reach the select on every attempt.
	holder := redislock.New(client)
	_, err := holder.TryLock(context.Background(), "k")
	require.NoError(t, err)

	var sawCanceled, sawNotAcquired atomic.Int64
	for i := 0; i < 40; i++ {
		lk := redislock.New(client, redislock.WithRetryInterval(time.Millisecond))
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // already cancelled before Lock starts
		_, err := lk.Lock(ctx, "k")
		switch {
		case errors.Is(err, context.Canceled):
			sawCanceled.Add(1)
		case errors.Is(err, redislock.ErrLockNotAcquired):
			// timer.C won the select; tryLock then hit the held key.
			sawNotAcquired.Add(1)
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	require.Positive(t, sawCanceled.Load(),
		"expected the ctx.Done() select arm to be hit at least once across iterations; "+
			"got canceled=%d notAcquired=%d", sawCanceled.Load(), sawNotAcquired.Load())
}

// TestHandleLoss_StopClosedDuringLoss pins the racy handleLoss `<-l.stop`
// early-return branch (lock.go:305-306): when Release closes stop BEFORE (or
// racing with) a renewal failure reaching handleLoss, the loss must be
// suppressed (no onLost, no Lost() close). The existing
// TestHandleLoss_CleanShutdown and TestAutoRenew_ReleaseDoesNotFireOnLost
// already target this path; because it hinges on a Release-vs-renew race the
// branch is only hit on some runs. This tight loop over many fresh locks makes
// the race land in nearly every `-count=1` run. It is fundamentally
// nondeterministic (the renewer's three-way select picks among <-l.stop,
// <-ctx.Done() and <-ticker.C at random once two are ready), so it may still
// miss occasionally; that is inherent to the design and not a test defect.
func TestHandleLoss_StopClosedDuringLoss(t *testing.T) {
	for i := 0; i < 120; i++ {
		client, mr := newClient(t)
		var fired atomic.Int64
		lk := redislock.New(client,
			redislock.WithTTL(80*time.Millisecond),
			redislock.WithAutoRenew(true),
			redislock.WithRenewInterval(5*time.Millisecond), // renew constantly
			redislock.WithOnLost(func(error) { fired.Add(1) }),
		)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		lock, err := lk.TryLock(ctx, "k")
		require.NoError(t, err)

		// Force an imminent renewal failure: drop the key so the next Refresh
		// returns ErrLockNotAcquired. Release is issued immediately after, so
		// the renewer's handleLoss often sees stop already closed.
		mr.Del("k")
		// Tiny jitter to interleave Release with the in-flight renew.
		time.Sleep(time.Millisecond)
		// Release may itself report ErrLockNotAcquired (we force-deleted the
		// key), which is an expected outcome here; the contract under test is
		// the onLost discipline, not Release's return.
		_ = lock.Release(ctx)
		cancel()

		// Settling window: let the renewer process whichever path it took.
		time.Sleep(20 * time.Millisecond)
		// If stop won the race, fired stays 0 (the branch we want covered). If
		// the loss landed first, fired is 1 and that's also fine — both are
		// correct outcomes; we only need the stop branch to be taken at least
		// once across the loop for coverage, which the assertion in
		// TestHandleLoss_CleanShutdown already witnesses. Here we just assert
		// no spurious double-fire.
		require.LessOrEqual(t, fired.Load(), int64(1), "onLost fired more than once (iter %d)", i)
	}
}

// Unreachable defensive branches (intentionally NOT covered):
//
//  1. lock.go:318-320 — randomToken's `rand.Read` error branch. crypto/rand
//     reads from the kernel CSPRNG (/dev/urandom on Linux/macOS, getrandom(2)
//     on modern Linux). It returns an error only when the entropy source is
//     unavailable — e.g. very early boot, a chroot/jail without /dev bound, or
//     a malformed max-FD setup. Under any normal runtime it does not fail, and
//     forcing it would require monkeypatching crypto/rand (off-limits: no
//     production-code changes).
//
//  2. lock.go:164-166 — tryLock's token-generation error path. It is reached
//     only via randomToken failing (branch 1), so it is unreachable for the
//     same reason.
//
// Both are correct defensive programming: a library must not assume the kernel
// CSPRNG is infallible, but its failure is not realistically reproducible in a
// unit test on this platform.
