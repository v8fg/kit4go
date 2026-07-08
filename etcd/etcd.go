// Package etcd is a thin, option-configured wrapper around
// go.etcd.io/etcd/client/v3.
//
// It targets the dominant etcd use case in ad-tech/finance services — service
// registration (Put + Lease) and discovery (Get + Watch) — and provides ergonomic
// construction (functional options + sane defaults), a fail-fast cluster ping at
// construction, pass-through KV (Put/Get/Delete), Lease (Grant/KeepAlive/Revoke)
// and Watch operations, lightweight metrics + an event hook, an escape hatch to
// the underlying *clientv3.Client, and a graceful Close. The concurrency
// primitives (Mutex/Lock/Election) are deliberately NOT wrapped (0 local usage;
// reach them via Client() if needed) — this keeps the surface thin.
package etcd

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ErrNoEndpoints is returned by New when no endpoints were configured.
var ErrNoEndpoints = errors.New("etcd: at least one endpoint required (WithEndpoints)")

// etcdAPI is the subset of *clientv3.Client the wrapper calls. *clientv3.Client
// embeds the KV/Lease/Watcher/Maintenance interfaces, whose methods promote to
// the client — so it satisfies this subset by structural promotion; tests inject
// a mock. Method signatures match client/v3 exactly (variadic OpOption;
// KeepAlive/Watch return channels).
type etcdAPI interface {
	Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error)
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error)
	Grant(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error)
	KeepAlive(ctx context.Context, id clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error)
	Revoke(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error)
	Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan
	Status(ctx context.Context, endpoint string) (*clientv3.StatusResponse, error)
}

// Compile-time assertion: *clientv3.Client must satisfy etcdAPI via interface
// promotion. Catches drift if etcd changes a method signature.
var _ etcdAPI = (*clientv3.Client)(nil)

// Client wraps an etcd client/v3 client. It is safe for concurrent use: all
// methods are goroutine-safe and Close is idempotent.
type Client struct {
	api  etcdAPI          // local interface; mock seam
	raw  *clientv3.Client // non-nil when built from a real client; nil when mock-injected
	own  bool             // true -> Close closes the underlying client
	opts Options

	puts, gets, deletes, grants, watches, errors atomic.Uint64
	onEvent                                      atomic.Pointer[func(Event)]
	closed                                       atomic.Bool // guards raw.Close for owning clients (clientv3.Client.Close is not idempotent)

	// closeFn is the underlying close invoked by Close for owning clients; nil
	// in production (Close falls back to c.raw.Close). Tests inject a counted
	// fake so idempotency can be asserted without a real *clientv3.Client.
	closeFn func() error
}

// opener opens a *clientv3.Client from a clientv3.Config. New uses the real
// clientv3.New; tests inject a fake via newClient.
type opener func(clientv3.Config) (*clientv3.Client, error)

var defaultOpener opener = clientv3.New

// New connects to the etcd cluster, verifies reachability with a Status call,
// and returns a Client.
//
// The context bounds the construction-time ping. If it carries no deadline, a
// 10s fallback is applied. etcd's clientv3.New opens a gRPC connection but does
// not guarantee the peer is live, so the Status ping fail-fast on a dead node.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	return newClient(ctx, opts, defaultOpener)
}

// newClient is the testable core of New: resolve options, open, ping, and (on
// success) return an owning Client. The open seam lets tests cover the
// open/ping/close paths without a live etcd.
func newClient(ctx context.Context, opts []Option, open opener) (*Client, error) {
	o := withDefaults(opts)
	if len(o.Endpoints) == 0 {
		return nil, ErrNoEndpoints
	}
	raw, err := open(o.toConfig())
	if err != nil {
		return nil, err
	}
	c := &Client{api: raw, raw: raw, own: true, opts: o}
	pingCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		pingCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	// Fail fast on an unreachable cluster. Without this, a misconfigured client
	// surfaces only on the first op (gRPC dial can succeed against a dead peer).
	if _, err := c.api.Status(pingCtx, o.Endpoints[0]); err != nil {
		_ = raw.Close() // etcd owns a gRPC conn + goroutines; release on ping failure.
		return nil, err
	}
	return c, nil
}

// Wrap adopts an existing *clientv3.Client. The Client does not own it: Close is
// a no-op. Useful for sharing a client.
func Wrap(raw *clientv3.Client) *Client {
	return &Client{api: raw, raw: raw, own: false, opts: withDefaults(nil)}
}

// newWithAPI builds a Client from an injected etcdAPI (testing only); raw is
// left nil so Client() returns nil.
func newWithAPI(api etcdAPI) *Client { return &Client{api: api, own: false, opts: withDefaults(nil)} }

// Put stores a key/value. Forward OpOptions (WithLease, WithPrevKV, ...) untouched.
func (c *Client) Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	c.puts.Add(1)
	resp, err := c.api.Put(ctx, key, val, opts...)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindPut, Outcome: OutcomeError})
		return nil, err
	}
	c.fireEvent(Event{Kind: KindPut, Outcome: OutcomeSuccess})
	return resp, nil
}

// Get reads key(s). Forward OpOptions (WithPrefix, WithRange, WithLimit, ...) untouched.
func (c *Client) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	c.gets.Add(1)
	resp, err := c.api.Get(ctx, key, opts...)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindGet, Outcome: OutcomeError})
		return nil, err
	}
	c.fireEvent(Event{Kind: KindGet, Outcome: OutcomeSuccess})
	return resp, nil
}

// Delete removes key(s). Forward OpOptions (WithPrefix, WithFromKey, ...) untouched.
func (c *Client) Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	c.deletes.Add(1)
	resp, err := c.api.Delete(ctx, key, opts...)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindDelete, Outcome: OutcomeError})
		return nil, err
	}
	c.fireEvent(Event{Kind: KindDelete, Outcome: OutcomeSuccess})
	return resp, nil
}

// Grant creates a lease with the given TTL (seconds) and returns its ID. Pair
// with KeepAlive to keep it alive, or Put(..., WithLease(id)) to attach it.
func (c *Client) Grant(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
	c.grants.Add(1)
	resp, err := c.api.Grant(ctx, ttl)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindGrant, Outcome: OutcomeError})
		return nil, err
	}
	c.fireEvent(Event{Kind: KindGrant, Outcome: OutcomeSuccess})
	return resp, nil
}

// KeepAlive keeps a lease alive until ctx is cancelled, streaming responses on
// the returned channel. The caller MUST drain it (range over it) or the
// keep-alive goroutine stalls. The wrapper increments the counter once on start.
func (c *Client) KeepAlive(ctx context.Context, id clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error) {
	ch, err := c.api.KeepAlive(ctx, id)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindKeepAlive, Outcome: OutcomeError})
		return nil, err
	}
	c.fireEvent(Event{Kind: KindKeepAlive, Outcome: OutcomeSuccess})
	return ch, nil
}

// Revoke revokes a lease immediately (deletes all keys attached to it).
func (c *Client) Revoke(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error) {
	resp, err := c.api.Revoke(ctx, id)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindRevoke, Outcome: OutcomeError})
		return nil, err
	}
	c.fireEvent(Event{Kind: KindRevoke, Outcome: OutcomeSuccess})
	return resp, nil
}

// Watch subscribes to changes on key (or a prefix with WithPrefix). It returns
// etcd's WatchChan; range over it to receive events. The wrapper fires one event
// on subscription start. The channel is closed when ctx is cancelled.
func (c *Client) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan {
	c.watches.Add(1)
	ch := c.api.Watch(ctx, key, opts...)
	c.fireEvent(Event{Kind: KindWatch, Outcome: OutcomeSuccess})
	return ch
}

// Status returns cluster status for the given endpoint (health, leader, version).
func (c *Client) Status(ctx context.Context, endpoint string) (*clientv3.StatusResponse, error) {
	resp, err := c.api.Status(ctx, endpoint)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindStatus, Outcome: OutcomeError})
		return nil, err
	}
	c.fireEvent(Event{Kind: KindStatus, Outcome: OutcomeSuccess})
	return resp, nil
}

// Client returns the underlying *clientv3.Client for anything the wrapper does
// not expose directly (Txn, Compact, Cluster, Auth, concurrency.Mutex/Election).
// Returns nil when the Client was built from a mock (newWithAPI).
func (c *Client) Client() *clientv3.Client { return c.raw }

// Options returns the resolved options the client was built with.
//
// The struct includes Password: do not log or serialize it verbatim.
func (c *Client) Options() Options { return c.opts }

// Close releases the underlying gRPC connection. No-op for a wrapped or
// mock-injected client, and safe to call any number of times: clientv3.Client.Close
// is not itself idempotent (a second close returns "context canceled"), so for an
// owning client only the first call reaches the underlying close.
func (c *Client) Close() error {
	if !c.own {
		return nil
	}
	// CompareAndSwap guarantees exactly one caller proceeds even under concurrent
	// Close calls; subsequent calls (including the documented second close) are no-ops.
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	if c.closeFn != nil {
		return c.closeFn()
	}
	return c.raw.Close()
}
