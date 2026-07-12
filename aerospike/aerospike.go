// Package aerospike is a thin, option-configured wrapper around
// github.com/aerospike/aerospike-client-go/v8.
//
// It targets high-throughput KV (ad-tech session/profile/audience stores) and
// provides ergonomic construction (functional options + sane defaults), an eager
// connection (NewClientWithPolicy connects + pings), pass-through Put/Get/Delete/
// BatchGet, lightweight metrics + an event hook, an escape hatch to the
// *as.Client, and a graceful Close. Query/Scan/Operate/UDF are reached via
// Client() (not wrapped — keeps the surface thin).
//
// aerospike methods return as.Error (an interface embedding error). The wrapper's
// public methods return the builtin error (as.Error satisfies error, so the
// conversion is implicit) — callers get a plain error usable with errors.Is/As.
package aerospike

import (
	"errors"
	"sync"
	"sync/atomic"

	as "github.com/aerospike/aerospike-client-go/v8"
)

// ErrNoHost is returned by New when no host was configured.
var ErrNoHost = errors.New("aerospike: host required (WithHost)")

// asAPI is the subset of *as.Client the wrapper calls. *as.Client satisfies it
// directly by structural typing (methods return as.Error, so the interface uses
// as.Error — not builtin error). The wrapper adapts as.Error to error in its own
// public methods. Tests inject a mock returning a sentinel as.Error (obtained
// from a public aerospike function, since as.Error has unexported methods and
// cannot be constructed directly).
type asAPI interface {
	Put(policy *as.WritePolicy, key *as.Key, binMap as.BinMap) as.Error
	Get(policy *as.BasePolicy, key *as.Key, binNames ...string) (*as.Record, as.Error)
	Delete(policy *as.WritePolicy, key *as.Key) (bool, as.Error)
	BatchGet(policy *as.BatchPolicy, keys []*as.Key, binNames ...string) ([]*as.Record, as.Error)
	Close()
}

// Compile-time: *as.Client satisfies asAPI.
var _ asAPI = (*as.Client)(nil)

// opener opens a *as.Client from a ClientPolicy + host:port, returning a builtin
// error. The real opener wraps as.NewClientWithPolicy (adapting its as.Error ->
// error); tests inject a fake returning a plain error.
type opener func(policy *as.ClientPolicy, host string, port int) (*as.Client, error)

var defaultOpener opener = func(p *as.ClientPolicy, host string, port int) (*as.Client, error) {
	return as.NewClientWithPolicy(p, host, port) // as.Error -> error
}

// Client wraps an aerospike client. Safe for concurrent use; Close is idempotent.
type Client struct {
	api  asAPI      // local interface; *as.Client satisfies it; mock seam
	raw  *as.Client // non-nil when built from a real client; nil when mock-injected
	own  bool       // true -> Close closes the underlying client
	opts Options

	puts, gets, deletes, errors atomic.Uint64
	onEvent                     atomic.Pointer[func(Event)]
	closeOnce                   sync.Once
}

// New connects to the cluster eagerly (NewClientWithPolicy connects + pings the
// first node) and returns an owning Client. host is required; port defaults to
// 3000 when 0.
func New(host string, port int, opts ...Option) (*Client, error) {
	return newClient(host, port, opts, defaultOpener)
}

// newClient is the testable core of New.
func newClient(host string, port int, opts []Option, open opener) (*Client, error) {
	o := withDefaults(opts)
	o.Host = host
	if port != 0 {
		o.Port = port
	}
	if o.Host == "" {
		return nil, ErrNoHost
	}
	raw, err := open(o.toClientPolicy(), o.Host, o.Port)
	if err != nil {
		return nil, err
	}
	return &Client{api: raw, raw: raw, own: true, opts: o}, nil
}

// Wrap adopts an existing *as.Client. The Client does not own it: Close is a
// no-op. Useful for sharing a client.
func Wrap(raw *as.Client, opts ...Option) *Client {
	o := withDefaults(opts)
	return &Client{api: raw, raw: raw, own: false, opts: o}
}

// newWithAPI builds a Client from an injected asAPI (testing only); raw is nil so
// Client() returns nil.
func newWithAPI(api asAPI) *Client { return &Client{api: api, own: false, opts: withDefaults(nil)} }

// Put writes bins for a key. Pass nil policy for defaults.
func (c *Client) Put(policy *as.WritePolicy, key *as.Key, binMap as.BinMap) error {
	c.puts.Add(1)
	err := c.api.Put(policy, key, binMap)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindPut, Outcome: OutcomeError})
		return err
	}
	c.fireEvent(Event{Kind: KindPut, Outcome: OutcomeSuccess})
	return nil
}

// Get reads a key. Pass nil policy for defaults; binNames empty -> all bins.
func (c *Client) Get(policy *as.BasePolicy, key *as.Key, binNames ...string) (*as.Record, error) {
	c.gets.Add(1)
	rec, err := c.api.Get(policy, key, binNames...)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindGet, Outcome: OutcomeError})
		return nil, err
	}
	c.fireEvent(Event{Kind: KindGet, Outcome: OutcomeSuccess})
	return rec, nil
}

// Delete removes a key. Returns whether a record existed. Pass nil policy for defaults.
func (c *Client) Delete(policy *as.WritePolicy, key *as.Key) (bool, error) {
	c.deletes.Add(1)
	existed, err := c.api.Delete(policy, key)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindDelete, Outcome: OutcomeError})
		return false, err
	}
	c.fireEvent(Event{Kind: KindDelete, Outcome: OutcomeSuccess})
	return existed, nil
}

// BatchGet reads multiple keys in one round-trip. Pass nil policy for defaults.
func (c *Client) BatchGet(policy *as.BatchPolicy, keys []*as.Key, binNames ...string) ([]*as.Record, error) {
	c.gets.Add(1)
	recs, err := c.api.BatchGet(policy, keys, binNames...)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindGet, Outcome: OutcomeError})
		return nil, err
	}
	c.fireEvent(Event{Kind: KindGet, Outcome: OutcomeSuccess})
	return recs, nil
}

// Close releases the underlying client. No-op for a wrapped or mock-injected
// client. Idempotent and concurrent-safe via sync.Once — coalesces at the
// wrapper layer (matching the clickhouse template) rather than relying on the
// upstream client's own close guard.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		if c.own {
			c.api.Close()
		}
	})
}

// Client returns the underlying *as.Client for anything the wrapper does not
// expose (Query, Scan, Operate, UDF, CreateIndex). Returns nil when built from
// a mock.
func (c *Client) Client() *as.Client { return c.raw }

// Options returns the resolved options the client was built with.
//
// The struct includes Password: do not log or serialize it verbatim.
func (c *Client) Options() Options { return c.opts }
