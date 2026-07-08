package etcd

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// shortCtx bounds dead-endpoint ping tests to 1s so the suite isn't dominated by
// the gRPC dial retry backoff (otherwise ~10s per test).
func shortCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), time.Second)
}

var errTest = errors.New("boom")

// --- New error paths ---

func TestNew_NoEndpoints(t *testing.T) {
	_, err := newClient(context.Background(), nil, defaultOpener)
	require.ErrorIs(t, err, ErrNoEndpoints)
}

func TestNew_OpenError(t *testing.T) {
	open := func(clientv3.Config) (*clientv3.Client, error) { return nil, errTest }
	_, err := newClient(context.Background(), []Option{WithEndpoints("http://127.0.0.1:1")}, open)
	require.ErrorIs(t, err, errTest)
}

// Construction-error: a real client aimed at a dead endpoint fails at the
// Status ping (or at open) — either way New returns nil + err, and the owning
// client is released.
func TestNew_ConstructionError(t *testing.T) {
	ctx, cancel := shortCtx(t)
	defer cancel()
	open := func(cfg clientv3.Config) (*clientv3.Client, error) { return clientv3.New(cfg) }
	c, err := newClient(ctx, []Option{WithEndpoints("http://127.0.0.1:1")}, open)
	require.Error(t, err)
	require.Nil(t, c)
}

// --- KV ops ---

func TestPut_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	resp, err := c.Put(context.Background(), "k", "v")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, uint64(1), c.Metrics().Puts)

	m.putFn = func(context.Context, string, string, ...clientv3.OpOption) (*clientv3.PutResponse, error) {
		return nil, errTest
	}
	_, err = c.Put(context.Background(), "k", "v")
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, uint64(1), c.Metrics().Errors)
}

func TestGet_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	resp, err := c.Get(context.Background(), "k")
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)
	assert.Equal(t, "k", string(resp.Kvs[0].Key))
	assert.Equal(t, uint64(1), c.Metrics().Gets)

	m.getFn = func(context.Context, string, ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return nil, errTest
	}
	_, err = c.Get(context.Background(), "k")
	require.ErrorIs(t, err, errTest)
}

func TestDelete_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	_, err := c.Delete(context.Background(), "k")
	require.NoError(t, err)
	assert.Equal(t, uint64(1), c.Metrics().Deletes)

	m.deleteFn = func(context.Context, string, ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
		return nil, errTest
	}
	_, err = c.Delete(context.Background(), "k")
	require.ErrorIs(t, err, errTest)
}

// --- Lease ops ---

func TestGrant_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	resp, err := c.Grant(context.Background(), 60)
	require.NoError(t, err)
	assert.Equal(t, clientv3.LeaseID(1), resp.ID) // mock default
	assert.Equal(t, uint64(1), c.Metrics().Grants)

	m.grantFn = func(context.Context, int64) (*clientv3.LeaseGrantResponse, error) {
		return nil, errTest
	}
	_, err = c.Grant(context.Background(), 60)
	require.ErrorIs(t, err, errTest)
}

func TestKeepAlive_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	ch, err := c.KeepAlive(context.Background(), clientv3.LeaseID(1))
	require.NoError(t, err)
	// drain the closed channel so the keep-alive goroutine is not left waiting
	for range ch {
	}

	m.keepAliveFn = func(context.Context, clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error) {
		return nil, errTest
	}
	_, err = c.KeepAlive(context.Background(), clientv3.LeaseID(1))
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, uint64(1), c.Metrics().Errors)
}

func TestRevoke_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	_, err := c.Revoke(context.Background(), clientv3.LeaseID(1))
	require.NoError(t, err)

	m.revokeFn = func(context.Context, clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error) {
		return nil, errTest
	}
	_, err = c.Revoke(context.Background(), clientv3.LeaseID(1))
	require.ErrorIs(t, err, errTest)
}

// --- Watch + Status ---

func TestWatch_StartsAndReturnsChannel(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	ch := c.Watch(context.Background(), "k", clientv3.WithPrefix())
	assert.Equal(t, uint64(1), c.Metrics().Watches)
	// drain the mock channel (one Created response then close)
	for range ch {
	}
}

func TestStatus_SuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	resp, err := c.Status(context.Background(), "http://e")
	require.NoError(t, err)
	require.NotNil(t, resp)

	m.statusFn = func(context.Context, string) (*clientv3.StatusResponse, error) {
		return nil, errTest
	}
	_, err = c.Status(context.Background(), "http://e")
	require.ErrorIs(t, err, errTest)
}

// --- Wrap / Client() / Close / Options ---

func TestWrap_ClientAndClose(t *testing.T) {
	// Wrap a real client built lazily (no connection attempted here).
	raw, err := clientv3.New(clientv3.Config{Endpoints: []string{"http://127.0.0.1:1"}, DialTimeout: 0})
	require.NoError(t, err)
	c := Wrap(raw)
	assert.Equal(t, raw, c.Client()) // escape hatch
	assert.NoError(t, c.Close())     // wrapped -> Close no-op (raw not owned)
}

func TestClient_NilWhenMockInjected(t *testing.T) {
	c := newWithAPI(&mockAPI{})
	assert.Nil(t, c.Client())
}

func TestClose_NoOpWhenNotOwned(t *testing.T) {
	c := newWithAPI(&mockAPI{})
	assert.NoError(t, c.Close()) // own=false -> no-op, no panic
}

func TestClose_OwnedClosesRaw(t *testing.T) {
	// A real client built lazily (no live connection); Close on an owning
	// wrapper must call raw.Close(). Constructed directly (white-box) since the
	// owning path only arises from a successful New (which needs a live etcd).
	raw, err := clientv3.New(clientv3.Config{Endpoints: []string{"http://127.0.0.1:1"}})
	require.NoError(t, err)
	c := &Client{raw: raw, own: true}
	assert.NoError(t, c.Close())
}

// Regression for the godoc contract "Close is idempotent". clientv3.Client.Close
// is NOT idempotent (a second call returns "context canceled"); the wrapper must
// shield callers from that. A counting fake closeFn stands in for raw.Close so the
// second call can be observed to be a no-op without driving a real client to the
// error path.
func TestClose_OwnedIsIdempotent(t *testing.T) {
	var closes atomic.Int32
	c := &Client{
		api:     &mockAPI{},
		own:     true,
		closeFn: func() error { closes.Add(1); return nil },
	}

	require.NoError(t, c.Close()) // first call closes
	require.NoError(t, c.Close()) // second call must be a no-op, not an error
	require.NoError(t, c.Close()) // and so must any further call
	assert.Equal(t, int32(1), closes.Load(), "underlying close must run exactly once")
}

// Concurrent Close calls on an owning client must also collapse to a single
// underlying close (the CAS guard is the concurrency safety claim in the godoc).
func TestClose_OwnedConcurrentSingleClose(t *testing.T) {
	var closes atomic.Int32
	c := &Client{
		api:     &mockAPI{},
		own:     true,
		closeFn: func() error { closes.Add(1); return nil },
	}

	const n = 16
	done := make(chan error, n)
	for range n {
		go func() { done <- c.Close() }()
	}
	for range n {
		assert.NoError(t, <-done)
	}
	assert.Equal(t, int32(1), closes.Load(), "concurrent Close must reach underlying close exactly once")
}

func TestNew_DelegatesAndErrors(t *testing.T) {
	// Exercises the public New (not just newClient); a dead endpoint fails
	// construction so New returns an error without a half-built client.
	ctx, cancel := shortCtx(t)
	defer cancel()
	_, err := New(ctx, WithEndpoints("http://127.0.0.1:1"))
	require.Error(t, err)
}

func TestOptions_ReturnsResolved(t *testing.T) {
	c := newWithAPI(&mockAPI{})
	assert.NotPanics(t, func() { _ = c.Options() })
	o := c.Options()
	assert.Equal(t, "5s", o.DialTimeout.String()) // default applied
}

// --- OnEvent ---

func TestSetOnEvent_FiresOnSuccessAndError(t *testing.T) {
	m := &mockAPI{}
	c := newWithAPI(m)
	var got []Event
	c.SetOnEvent(func(e Event) { got = append(got, e) })

	_, _ = c.Put(context.Background(), "k", "v") // success
	m.putFn = func(context.Context, string, string, ...clientv3.OpOption) (*clientv3.PutResponse, error) {
		return nil, errTest
	}
	_, _ = c.Put(context.Background(), "k", "v") // error

	require.Len(t, got, 2)
	assert.Equal(t, KindPut, got[0].Kind)
	assert.Equal(t, OutcomeSuccess, got[0].Outcome)
	assert.Equal(t, OutcomeError, got[1].Outcome)

	c.SetOnEvent(nil)
	assert.Nil(t, c.onEvent.Load())
}

// --- With* options ---

func TestOptions_AllWith(t *testing.T) {
	o := withDefaults([]Option{
		WithEndpoints("a", "b"),
		WithDialTimeout(0),
		WithDialKeepAliveTime(30_000_000_000), // non-zero -> exercises toConfig keepalive branch
		WithTLSConfig(nil),
		WithUsername("u"),
		WithPassword("p"),
		WithAutoSyncInterval(0),
		WithRejectOldCluster(true),
	})
	assert.Equal(t, []string{"a", "b"}, o.Endpoints)
	assert.Equal(t, "u", o.Username)
	assert.Equal(t, "p", o.Password)
	assert.True(t, o.RejectOldCluster)
	cfg := o.toConfig()
	assert.Equal(t, int64(30_000_000_000), int64(cfg.DialKeepAliveTime))
}

func TestOptions_DialTimeoutDefaultsTo5s(t *testing.T) {
	o := withDefaults(nil)
	assert.Equal(t, "5s", o.DialTimeout.String())
}

func TestOptions_EndpointsCopied(t *testing.T) {
	in := []string{"a"}
	o := withDefaults([]Option{WithEndpoints(in...)})
	in[0] = "mutated"
	assert.Equal(t, []string{"a"}, o.Endpoints, "WithEndpoints must copy (caller mutation must not leak)")
}

func TestOptions_ToConfigMaps(t *testing.T) {
	o := withDefaults([]Option{
		WithEndpoints("http://e", "http://f"),
		WithUsername("u"),
		WithPassword("p"),
		WithRejectOldCluster(true),
	})
	cfg := o.toConfig()
	assert.Equal(t, []string{"http://e", "http://f"}, cfg.Endpoints)
	assert.Equal(t, "u", cfg.Username)
	assert.True(t, cfg.RejectOldCluster)
}
