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
// Like the minio/etcd/mongo wrappers, New and all ops take a context.Context: it
// bounds the construction Ping and each HTTP call. When the caller's context
// carries no deadline, New applies a 10s fallback to the Ping so a
// context.Background() caller cannot block startup on a half-open endpoint.
//
// go-elasticsearch v8.19 exposes Index/Search/Get/Delete/Ping as FIELDS of named
// func types (not methods). The wrapper holds these func fields directly (copied
// from the client; tests assign their own) — so no adapter or interface layer is
// needed.
package elasticsearch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// ErrNoAddresses is returned by New when no address was configured.
var ErrNoAddresses = errors.New("elasticsearch: at least one address required (WithAddresses)")

// ErrPingFailed is returned by New when the construction Ping does not yield a
// 2xx status (the cluster is reachable but unhealthy/unauthorized, or returned
// an unexpected status). Callers may errors.Is against it to distinguish a
// connectivity/health failure from a config/transport error.
var ErrPingFailed = errors.New("elasticsearch: ping returned non-success status")

// pingTimeout bounds the construction Ping when the caller's context carries no
// deadline. Mirrors the minio/etcd/mongo wrappers.
const pingTimeout = 10 * time.Second

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
//
// The context bounds the construction-time Ping. If it carries no deadline, a
// 10s fallback is applied so a caller passing context.Background() against a
// half-open endpoint cannot block startup indefinitely.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	return newClient(ctx, opts, defaultOpener)
}

// newClient is the testable core of New.
func newClient(ctx context.Context, opts []Option, open opener) (*Client, error) {
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
	if err := c.pingFailFast(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// pingFailFast calls Ping and treats a transport error (not a non-2xx HTTP
// status) as a connectivity failure. Ping returns 200 on a live cluster. The
// context bounds the call; when it carries no deadline a 10s fallback is
// applied (mirror of minio/etcd/mongo) so a context.Background() caller does
// not block on a half-open endpoint.
func (c *Client) pingFailFast(ctx context.Context) error {
	pingCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		pingCtx, cancel = context.WithTimeout(ctx, pingTimeout)
		defer cancel()
	}
	resp, err := c.ping(esapi.Ping(nil).WithContext(pingCtx))
	if err != nil {
		return err
	}
	// Close the body on ALL paths: the transport returns a non-nil *Response
	// even for HTTP errors, and draining it lets the underlying connection be
	// reused. The guard is defensive (esapi returns a non-nil resp on a nil
	// err, so a nil resp here is a driver contract violation we surface as a
	// ping failure rather than panic). The close error is intentionally
	// ignored — Ping carries no useful body and a close error cannot change
	// the connectivity verdict.
	if resp == nil {
		return ErrPingFailed
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("elasticsearch: ping returned status %d: %w", resp.StatusCode, ErrPingFailed)
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
// (WithDocumentID, WithRefresh, ...) untouched; ctx is applied via WithContext
// (caller options win — appended last, so an explicit WithContext overrides).
func (c *Client) Index(ctx context.Context, index string, body io.Reader, opts ...func(*esapi.IndexRequest)) (*esapi.Response, error) {
	c.indexes.Add(1)
	resp, err := c.index(index, body, prependCtx(esapi.Index(nil).WithContext(ctx), opts)...)
	return c.done(KindIndex, resp, err)
}

// Search runs a query. Pass WithBody(body) + WithIndex("idx") etc. in opts.
func (c *Client) Search(ctx context.Context, opts ...func(*esapi.SearchRequest)) (*esapi.Response, error) {
	c.searches.Add(1)
	resp, err := c.search(prependCtx(esapi.Search(nil).WithContext(ctx), opts)...)
	return c.done(KindSearch, resp, err)
}

// Get fetches a document by id.
func (c *Client) Get(ctx context.Context, index, id string, opts ...func(*esapi.GetRequest)) (*esapi.Response, error) {
	c.gets.Add(1)
	resp, err := c.get(index, id, prependCtx(esapi.Get(nil).WithContext(ctx), opts)...)
	return c.done(KindGet, resp, err)
}

// Delete removes a document by id.
func (c *Client) Delete(ctx context.Context, index, id string, opts ...func(*esapi.DeleteRequest)) (*esapi.Response, error) {
	c.deletes.Add(1)
	resp, err := c.delete(index, id, prependCtx(esapi.Delete(nil).WithContext(ctx), opts)...)
	return c.done(KindDelete, resp, err)
}

// prependCtx prepends the WithContext option to the caller's options. The
// caller's options are appended after, so a caller that explicitly passes
// WithContext wins (esapi applies options in order; last write wins).
func prependCtx[Req any](ctxOpt func(*Req), opts []func(*Req)) []func(*Req) {
	out := make([]func(*Req), 0, len(opts)+1)
	out = append(out, ctxOpt)
	out = append(out, opts...)
	return out
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
