package rate_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/rate"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }

func newLimiter(t *testing.T, clk *fakeClock) (*rate.Limiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	c := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = c.Close() })
	if clk != nil {
		return rate.New(c, rate.WithClock(clk.now)), mr
	}
	return rate.New(c), mr
}

func TestAllowBurst(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	l, _ := newLimiter(t, clk)
	limit := rate.PerSecond(5, 5) // 5/sec, burst 5

	// The first 5 (burst) are allowed.
	for i := 0; i < 5; i++ {
		r, err := l.Allow(context.Background(), "k", limit)
		require.NoError(t, err)
		require.True(t, r.Allowed, "burst token %d should be allowed", i)
	}
	// The 6th within the same instant is denied.
	r, err := l.Allow(context.Background(), "k", limit)
	require.NoError(t, err)
	require.False(t, r.Allowed)
	require.Greater(t, r.RetryAfter, time.Duration(0))
}

func TestAllowRecoversAfterInterval(t *testing.T) {
	clk := &fakeClock{t: time.Unix(2000, 0)}
	l, _ := newLimiter(t, clk)
	limit := rate.PerSecond(2, 2) // emission = 500ms

	// Exhaust the burst.
	for i := 0; i < 2; i++ {
		_, err := l.Allow(context.Background(), "k", limit)
		require.NoError(t, err)
	}
	r, _ := l.Allow(context.Background(), "k", limit)
	require.False(t, r.Allowed)

	// Advance past one emission interval -> one token is free again.
	clk.t = clk.t.Add(600 * time.Millisecond)
	r, err := l.Allow(context.Background(), "k", limit)
	require.NoError(t, err)
	require.True(t, r.Allowed, "a token should be free after ~one emission interval")
}

func TestKeysAreIndependent(t *testing.T) {
	l, _ := newLimiter(t, nil)
	limit := rate.PerSecond(2, 2)
	r1, _ := l.Allow(context.Background(), "user:a", limit)
	r2, _ := l.Allow(context.Background(), "user:b", limit)
	require.True(t, r1.Allowed)
	require.True(t, r2.Allowed)
}

func TestAllowNMultiToken(t *testing.T) {
	clk := &fakeClock{t: time.Unix(3000, 0)}
	l, _ := newLimiter(t, clk)
	limit := rate.PerSecond(10, 10) // emission 100ms

	// Consume 8 of 10 at once.
	r, err := l.AllowN(context.Background(), "k", limit, 8)
	require.NoError(t, err)
	require.True(t, r.Allowed)
	require.Equal(t, 2, r.Remaining)

	// Asking for 5 more (only 2 left) is denied; the bucket is not consumed.
	r, err = l.AllowN(context.Background(), "k", limit, 5)
	require.NoError(t, err)
	require.False(t, r.Allowed)

	// The remaining 2 are still consumable one at a time.
	r, _ = l.AllowN(context.Background(), "k", limit, 2)
	require.True(t, r.Allowed)
}

func TestCostExceedingBurstDenied(t *testing.T) {
	l, _ := newLimiter(t, nil)
	limit := rate.PerSecond(5, 3) // burst 3
	// Asking for more than burst is always denied.
	r, err := l.AllowN(context.Background(), "k", limit, 4)
	require.NoError(t, err)
	require.False(t, r.Allowed)
}

func TestRemainingNeverNegative(t *testing.T) {
	l, _ := newLimiter(t, nil)
	limit := rate.PerSecond(1, 1)
	for i := 0; i < 5; i++ {
		r, _ := l.Allow(context.Background(), "k", limit)
		require.GreaterOrEqual(t, r.Remaining, 0)
	}
}

func TestInvalidLimits(t *testing.T) {
	l, _ := newLimiter(t, nil)
	_, err := l.Allow(context.Background(), "k", rate.Limit{Rate: 0, Period: time.Second, Burst: 1})
	require.ErrorIs(t, err, rate.ErrLimitInvalid)
	_, err = l.Allow(context.Background(), "k", rate.Limit{Rate: 1, Period: 0, Burst: 1})
	require.ErrorIs(t, err, rate.ErrLimitInvalid)
	_, err = l.Allow(context.Background(), "k", rate.Limit{Rate: 1, Period: time.Second, Burst: 0})
	require.ErrorIs(t, err, rate.ErrLimitInvalid)
	_, err = l.AllowN(context.Background(), "k", rate.PerSecond(1, 1), 0)
	require.ErrorIs(t, err, rate.ErrLimitInvalid)
}

func TestPerMinuteLimit(t *testing.T) {
	clk := &fakeClock{t: time.Unix(4000, 0)}
	l, _ := newLimiter(t, clk)
	limit := rate.PerMinute(60, 2) // 1/sec, burst 2
	r, err := l.Allow(context.Background(), "k", limit)
	require.NoError(t, err)
	require.True(t, r.Allowed)
}
