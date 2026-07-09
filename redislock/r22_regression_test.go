package redislock

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func r22Client(t *testing.T) (goredis.Cmdable, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	return goredis.NewClient(&goredis.Options{Addr: mr.Addr()}), mr
}

// TestR22_PanickingOnLost_DoesNotCrash verifies the recover() in handleLoss:
// a panicking onLost callback must not crash the test process (the P1 fix).
func TestR22_PanickingOnLost_DoesNotCrash(t *testing.T) {
	client, mr := r22Client(t)

	locker := New(client, WithAutoRenew(true), WithRenewInterval(5*time.Millisecond),
		WithTTL(20*time.Millisecond),
		WithOnLost(func(error) { panic("boom from onLost") }))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	lock, err := locker.Lock(ctx, "r22key")
	require.NoError(t, err)

	mr.Del("r22key") // force loss: next renew fails -> handleLoss -> onLost panics

	select {
	case <-lock.Lost():
		// recover caught the panic; Lost() closed; renewer exited cleanly
	case <-time.After(time.Second):
		t.Fatal("Lost() not closed — onLost panic crashed the renewer without recover")
	}

	// Process still alive. Release should return ErrLockNotAcquired (already lost).
	err = lock.Release(context.Background())
	assert.ErrorIs(t, err, ErrLockNotAcquired)
}
