package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/redis"
)

func newMini(t *testing.T) (*miniredis.Miniredis, string) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return mr, mr.Addr()
}

func TestNew_SingleNode_RoundTrip(t *testing.T) {
	_, addr := newMini(t)
	c, err := redis.New(
		redis.WithAddrs(addr),
		redis.WithDialTimeout(time.Second),
		redis.WithReadTimeout(time.Second),
		redis.WithPoolSize(4),
		redis.WithClientName("kit4go-test"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	ctx := context.Background()
	require.NoError(t, c.Ping(ctx))

	cmd := c.Cmdable()
	require.NoError(t, cmd.Set(ctx, "k", "v", 0).Err())
	got, err := cmd.Get(ctx, "k").Result()
	require.NoError(t, err)
	require.Equal(t, "v", got)

	require.Equal(t, "kit4go-test", c.Options().ClientName)
}

func TestNew_NoAddrs(t *testing.T) {
	_, err := redis.New()
	require.ErrorIs(t, err, redis.ErrNoAddrs)
}

func TestWrap_DoesNotOwn(t *testing.T) {
	_, addr := newMini(t)
	// Caller owns the underlying goredis client.
	underlying := goredis.NewClient(&goredis.Options{Addr: addr})
	t.Cleanup(func() { require.NoError(t, underlying.Close()) })

	c := redis.Wrap(underlying)
	ctx := context.Background()
	require.NoError(t, c.Ping(ctx))
	require.NoError(t, c.Cmdable().Set(ctx, "k", "v", 0).Err())
	// Close on a wrapped client is a no-op; underlying still works after.
	require.NoError(t, c.Close())
	got, err := c.Cmdable().Get(ctx, "k").Result()
	require.NoError(t, err)
	require.Equal(t, "v", got)
}

func TestModeSelection(t *testing.T) {
	_, addr := newMini(t)

	// ModeSingle forces single-node even with two addrs.
	single, err := redis.New(redis.WithAddrs(addr, addr), redis.WithMode(redis.ModeSingle))
	require.NoError(t, err)
	t.Cleanup(func() { _ = single.Close() })
	_, ok := single.Cmdable().(*goredis.Client)
	require.True(t, ok, "ModeSingle should build a *redis.Client")

	// ModeAuto with >1 addr selects cluster.
	cluster, err := redis.New(redis.WithAddrs(addr, addr), redis.WithMode(redis.ModeAuto))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cluster.Close() })
	_, ok = cluster.Cmdable().(*goredis.ClusterClient)
	require.True(t, ok, "ModeAuto with 2 addrs should build a *redis.ClusterClient")

	// ModeAuto with 1 addr selects single.
	single2, err := redis.New(redis.WithAddrs(addr), redis.WithMode(redis.ModeAuto))
	require.NoError(t, err)
	t.Cleanup(func() { _ = single2.Close() })
	_, ok = single2.Cmdable().(*goredis.Client)
	require.True(t, ok, "ModeAuto with 1 addr should build a *redis.Client")
}

func TestPoolStats(t *testing.T) {
	_, addr := newMini(t)
	c, err := redis.New(redis.WithAddrs(addr))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.Background()
	// Force a connection.
	require.NoError(t, c.Ping(ctx))
	// PoolStats must not panic and returns a value struct.
	stats := c.PoolStats()
	_ = stats.Hits + stats.Misses + stats.TotalConns // touch fields
}

func TestAuth(t *testing.T) {
	mr, addr := newMini(t)
	mr.RequireAuth("secret")

	c, err := redis.New(redis.WithAddrs(addr), redis.WithPassword("secret"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NoError(t, c.Ping(context.Background()))
}

// Every With* option must land in the resolved Options.
func TestAllOptions(t *testing.T) {
	_, addr := newMini(t)
	c, err := redis.New(
		redis.WithAddrs(addr),
		redis.WithMode(redis.ModeSingle),
		redis.WithUsername("u"),
		redis.WithPassword("p"),
		redis.WithDB(2),
		redis.WithDialTimeout(time.Second),
		redis.WithReadTimeout(2*time.Second),
		redis.WithWriteTimeout(3*time.Second),
		redis.WithPoolSize(8),
		redis.WithMinIdleConns(2),
		redis.WithMaxRetries(4),
		redis.WithClientName("cn"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	o := c.Options()
	require.Equal(t, "u", o.Username)
	require.Equal(t, "p", o.Password)
	require.Equal(t, 2, o.DB)
	require.Equal(t, time.Second, o.DialTimeout)
	require.Equal(t, 2*time.Second, o.ReadTimeout)
	require.Equal(t, 3*time.Second, o.WriteTimeout)
	require.Equal(t, 8, o.PoolSize)
	require.Equal(t, 2, o.MinIdleConns)
	require.Equal(t, 4, o.MaxRetries)
	require.Equal(t, "cn", o.ClientName)
}

// Sentinel topology constructs a single-node failover client (no connection
// attempt until a command, so it succeeds without a real Sentinel).
func TestSentinel_Construct(t *testing.T) {
	c, err := redis.New(
		redis.WithMasterName("mymaster"),
		redis.WithAddrs("sentinel-1:26379", "sentinel-2:26379"),
	)
	require.NoError(t, err)
	require.NoError(t, c.Close())
	_, ok := c.Cmdable().(*goredis.Client)
	require.True(t, ok, "Sentinel should build a failover *redis.Client")
}
