package redis_test

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/redis"
)

// TestWithTLSConfig covers the WithTLSConfig option (sets opts.TLSConfig). The
// client is built with a TLS config pointing at a plaintext miniredis, so Ping
// fails — but the option branch itself is exercised and the resolved Options
// carry the config. We verify via Options() rather than a successful handshake.
func TestWithTLSConfig(t *testing.T) {
	_, addr := newMini(t)
	tlsCfg := &tls.Config{InsecureSkipVerify: true}
	c, err := redis.New(
		redis.WithAddrs(addr),
		redis.WithMode(redis.ModeSingle),
		redis.WithTLSConfig(tlsCfg),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NotNil(t, c.Options().TLSConfig, "WithTLSConfig must set opts.TLSConfig")
	require.Same(t, tlsCfg, c.Options().TLSConfig)

	// Ping fails: miniredis speaks plaintext, the client attempts TLS. This
	// confirms the TLS config actually reached the dialer (not just opts).
	err = c.Ping(context.Background())
	require.Error(t, err, "TLS handshake against plaintext miniredis must fail")
}

// TestNew_ClusterModeExplicit covers the ModeCluster branch of isCluster by
// forcing cluster topology with a single address.
func TestNew_ClusterModeExplicit(t *testing.T) {
	_, addr := newMini(t)
	c, err := redis.New(
		redis.WithAddrs(addr),
		redis.WithMode(redis.ModeCluster),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	_, ok := c.Cmdable().(*goredis.ClusterClient)
	require.True(t, ok, "ModeCluster must build a *redis.ClusterClient")
}

// TestNew_ClusterModeWithTLS exercises both the ModeCluster branch and the
// TLS-config plumbing for the cluster path.
func TestNew_ClusterModeWithTLS(t *testing.T) {
	_, addr := newMini(t)
	c, err := redis.New(
		redis.WithAddrs(addr, addr),
		redis.WithMode(redis.ModeAuto),
		redis.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NotNil(t, c.Options().TLSConfig)
	_, ok := c.Cmdable().(*goredis.ClusterClient)
	require.True(t, ok, "ModeAuto with 2 addrs must build a *redis.ClusterClient")
}

// TestClose_WrappedCmdableWithoutClose covers the final `return nil` branch of
// Close when the underlying cmdable does not implement `Close() error`. We
// inject a mock Cmdable that satisfies the command surface but has no Close.
func TestClose_WrappedCmdableWithoutClose(t *testing.T) {
	c := redis.Wrap(&closelessCmdable{})
	// Close must be a no-op (no panic, returns nil) — covers the trailing
	// return nil after the failed type assertion.
	require.NotPanics(t, func() {
		require.NoError(t, c.Close())
	})
}

// TestPoolStats_MockStatter covers the `s.PoolStats()` true branch of
// PoolStats by wrapping a Cmdable whose PoolStats method returns a pointer —
// the signature real go-redis clients expose. A value-returning mock would NOT
// satisfy the interface (the assertion must match *PoolStats), which is the
// regression this test guards against.
func TestPoolStats_MockStatter(t *testing.T) {
	c := redis.Wrap(&statterCmdable{stats: goredis.PoolStats{Hits: 7, Misses: 3}})
	got := c.PoolStats()
	require.Equal(t, uint32(7), got.Hits)
	require.Equal(t, uint32(3), got.Misses)
}

// TestPoolStats_MockStatterNil covers the nil-pointer dereference guard: a
// statter returning nil must yield the zero value, not a panic.
func TestPoolStats_MockStatterNil(t *testing.T) {
	c := redis.Wrap(&statterCmdable{nilStats: true})
	got := c.PoolStats()
	require.Equal(t, goredis.PoolStats{}, got)
}

// TestPoolStats_NoStatter covers the zero-value fallback when the wrapped
// Cmdable has no PoolStats method at all.
func TestPoolStats_NoStatter(t *testing.T) {
	c := redis.Wrap(&closelessCmdable{})
	got := c.PoolStats()
	require.Equal(t, goredis.PoolStats{}, got)
}

// TestNew_SentinelWithAllOptions exercises the Sentinel branch with the full
// option set (TLS, timeouts, pool tuning) so every FailoverOptions field is
// populated. No real Sentinel is contacted (connection is lazy).
func TestNew_SentinelWithAllOptions(t *testing.T) {
	c, err := redis.New(
		redis.WithMasterName("mymaster"),
		redis.WithAddrs("sentinel-1:26379", "sentinel-2:26379"),
		redis.WithUsername("u"),
		redis.WithPassword("p"),
		redis.WithDB(1),
		redis.WithDialTimeout(time.Second),
		redis.WithReadTimeout(2*time.Second),
		redis.WithWriteTimeout(3*time.Second),
		redis.WithPoolSize(8),
		redis.WithMinIdleConns(2),
		redis.WithMaxRetries(3),
		redis.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	require.NoError(t, err)
	require.NoError(t, c.Close())
	o := c.Options()
	require.Equal(t, "mymaster", o.MasterName)
	require.Equal(t, "u", o.Username)
	require.Equal(t, "p", o.Password)
	require.NotNil(t, o.TLSConfig)
}

// closelessCmdable is a minimal goredis.Cmdable stub whose only purpose is to
// NOT implement `Close() error` (so Close's type assertion fails) and to NOT
// implement PoolStats (so PoolStats falls back to zero). We embed nothing; we
// only need the type to be passable to Wrap. Because redis.Cmdable has many
// methods, we satisfy it via an embedded interface value that stays nil and is
// never called — the methods are only invoked by tests that don't reach the
// command surface.
type closelessCmdable struct {
	goredis.Cmdable // nil; never called in these tests
}

// statterCmdable is a Cmdable stub whose PoolStats returns a pointer, matching
// the real go-redis signature (*goredis.PoolStats) so it satisfies the local
// statter interface.
type statterCmdable struct {
	goredis.Cmdable // nil; never called
	stats           goredis.PoolStats
	nilStats        bool // when true, PoolStats returns nil
}

func (s *statterCmdable) PoolStats() *goredis.PoolStats {
	if s.nilStats {
		return nil
	}
	return &s.stats
}
