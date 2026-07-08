// Package elasticsearch is a thin, option-configured wrapper around the official
// github.com/elastic/go-elasticsearch/v8 (low-level esapi).
//
// It covers document CRUD + search — Index/Get/Search/Delete — and provides
// ergonomic construction (functional options + sane defaults), a fail-fast Ping
// at construction, lightweight metrics + an event hook, and an escape hatch to
// the underlying *elasticsearch.Client. Bulk/Aggregation/Cat/Indices/Cluster
// APIs are reached via Client() (not wrapped — keeps the surface thin). There is
// no Close: elasticsearch.Client is stateless (an HTTP connection pool owned by
// http.Transport; release a custom Transport set via WithTransport yourself).
//
// go-elasticsearch v8.19 exposes Index/Search/Get/Delete/Ping as FIELDS of named
// func types (not methods). The wrapper holds these func fields directly (copied
// from the client; tests assign their own) — so no adapter or interface layer is
// needed.
package elasticsearch

import (
	"errors"
	"io"
	"sync/atomic"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// ErrNoAddresses is returned by New when no address was configured.
var ErrNoAddresses = errors.New("elasticsearch: at least one address required (WithAddresses)")

// Client wraps an elasticsearch client. Safe for concurrent use.
type Client struct {
	// esapi callables (named func-type fields on *elasticsearch.Client). Copied
	// at construction; tests may overwrite them.
	index  esapi.Index
	search esapi.Search
	get    esapi.Get
	delete esapi.Delete
	ping   esapi.Ping

	raw  *elasticsearch.Client // non-nil when built from a real client; nil when mock-injected
	opts Options

	indexes, searches, gets, deletes, errors atomic.Uint64
	onEvent                                  atomic.Pointer[func(Event)]
}

// opener builds a *elasticsearch.Client from a Config. New uses the real
// elasticsearch.NewClient; tests inject a fake.
type opener func(cfg elasticsearch.Config) (*elasticsearch.Client, error)

var defaultOpener opener = elasticsearch.NewClient

// New constructs the client and verifies connectivity with a Ping. Returns an
// owning Client.
func New(opts ...Option) (*Client, error) {
	return newClient(opts, defaultOpener)
}

// newClient is the testable core of New.
func newClient(opts []Option, open opener) (*Client, error) {
	o := withDefaults(opts)
	if len(o.Addresses) == 0 {
		return nil, ErrNoAddresses
	}
	raw, err := open(o.toDriver())
	if err != nil {
		return nil, err
	}
	c := &Client{
		raw:    raw,
		opts:   o,
		index:  raw.Index,
		search: raw.Search,
		get:    raw.Get,
		delete: raw.Delete,
		ping:   raw.Ping,
	}
	// Fail fast on an unreachable cluster (NewClient does not connect eagerly).
	if err := c.pingFailFast(); err != nil {
		return nil, err
	}
	return c, nil
}

// pingFailFast calls Ping and treats a transport error (not a non-2xx HTTP
// status) as a connectivity failure. Ping returns 200 on a live cluster.
func (c *Client) pingFailFast() error {
	resp, err := c.ping()
	if err != nil {
		return err
	}
	if resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return errors.New("elasticsearch: ping returned non-success status")
}

// Wrap adopts an existing *elasticsearch.Client.
func Wrap(raw *elasticsearch.Client, opts ...Option) *Client {
	o := withDefaults(opts)
	return &Client{
		raw: raw, opts: o,
		index: raw.Index, search: raw.Search, get: raw.Get, delete: raw.Delete, ping: raw.Ping,
	}
}

// Index creates/replaces a document. body is the JSON document. Forward options
// (WithDocumentID, WithRefresh, ...) untouched.
func (c *Client) Index(index string, body io.Reader, opts ...func(*esapi.IndexRequest)) (*esapi.Response, error) {
	c.indexes.Add(1)
	resp, err := c.index(index, body, opts...)
	return c.done(KindIndex, resp, err)
}

// Search runs a query. Pass WithBody(body) + WithIndex("idx") etc. in opts.
func (c *Client) Search(opts ...func(*esapi.SearchRequest)) (*esapi.Response, error) {
	c.searches.Add(1)
	resp, err := c.search(opts...)
	return c.done(KindSearch, resp, err)
}

// Get fetches a document by id.
func (c *Client) Get(index, id string, opts ...func(*esapi.GetRequest)) (*esapi.Response, error) {
	c.gets.Add(1)
	resp, err := c.get(index, id, opts...)
	return c.done(KindGet, resp, err)
}

// Delete removes a document by id.
func (c *Client) Delete(index, id string, opts ...func(*esapi.DeleteRequest)) (*esapi.Response, error) {
	c.deletes.Add(1)
	resp, err := c.delete(index, id, opts...)
	return c.done(KindDelete, resp, err)
}

// done records the outcome (error counter + event) and returns resp/err. The
// low-level API returns a *Response with a StatusCode even on HTTP errors; only
// a transport error (err != nil) is counted here — callers inspect StatusCode
// for HTTP-level outcomes (404 etc.).
func (c *Client) done(kind string, resp *esapi.Response, err error) (*esapi.Response, error) {
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: kind, Outcome: OutcomeError})
		return resp, err
	}
	c.fireEvent(Event{Kind: kind, Outcome: OutcomeSuccess})
	return resp, nil
}

// Client returns the underlying *elasticsearch.Client for anything the wrapper
// does not expose (Bulk, Indices, Cat, Cluster, Aggregations). Returns nil when
// built from a mock.
func (c *Client) Client() *elasticsearch.Client { return c.raw }

// Options returns the resolved options the client was built with.
//
// The struct may carry credentials: do not log it verbatim.
func (c *Client) Options() Options { return c.opts }
