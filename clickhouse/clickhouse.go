// Package clickhouse is a thin, option-configured wrapper around
// github.com/ClickHouse/clickhouse-go/v2.
//
// It provides ergonomic construction (functional options + sane defaults), a
// health check, pass-through Exec/Query/QueryRow/PrepareBatch, an escape hatch
// to the underlying driver.Conn, lightweight metrics + an event hook, and a
// graceful, idempotent Close. Everything else is the standard clickhouse-go API
// reached via Client.Conn(). Like the redis/postgres wrappers it deliberately
// stays small: no query builder, no domain types, no business logic.
package clickhouse

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// ErrNoAddrs is returned by New when no address was configured (WithAddrs).
var ErrNoAddrs = errors.New("clickhouse: at least one address required (WithAddrs)")

// Conn is the subset of driver.Conn that Client uses internally. driver.Conn
// (returned by clickhouse.Open) satisfies it automatically; tests inject a
// mock. Methods returning clickhouse-specific types (Rows/Row/Batch/Stats) use
// those types directly — the wrapper is thin and never reinvents the driver API.
type Conn interface {
	Ping(context.Context) error
	Exec(ctx context.Context, query string, args ...any) error
	Query(ctx context.Context, query string, args ...any) (driver.Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) driver.Row
	PrepareBatch(ctx context.Context, query string, opts ...driver.PrepareBatchOption) (driver.Batch, error)
	Stats() driver.Stats
	Close() error
}

// Client wraps a clickhouse-go connection.
type Client struct {
	conn    Conn        // local interface; mock seam
	rawConn driver.Conn // non-nil when built from a real driver.Conn; nil when mock-injected
	own     bool        // true -> Close closes the underlying conn
	opts    Options

	queries, execs, batches, errors, pings, pingErrors atomic.Uint64
	onEvent                                            atomic.Pointer[func(Event)]
}

// opener opens a driver.Conn from clickhouse-go Options. New uses the real
// clickhouse.Open; tests inject a mock via newClient.
type opener func(*clickhouse.Options) (driver.Conn, error)

var defaultOpener opener = clickhouse.Open

// New opens a connection, pings it, and returns a Client. The context bounds
// the construction-time Ping.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	return newClient(ctx, opts, defaultOpener)
}

// newClient is the testable core of New: resolve options, call open, ping, and
// (on success) return an owning Client. The open seam lets tests cover the
// open/ping/close paths without a live ClickHouse.
func newClient(ctx context.Context, opts []Option, open opener) (*Client, error) {
	o := withDefaults(opts)
	if len(o.Addrs) == 0 {
		return nil, ErrNoAddrs
	}
	conn, err := open(o.toDriver())
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &Client{conn: conn, rawConn: conn, own: true, opts: o}, nil
}

// Wrap adopts an existing driver.Conn (e.g. one opened elsewhere). The Client
// does not own it: Close is a no-op. Useful for testing and for sharing a conn.
func Wrap(raw driver.Conn) *Client {
	return &Client{conn: raw, rawConn: raw, own: false}
}

// newWithConn builds a Client from an injected Conn (testing only); rawConn is
// left nil so Conn() returns nil, mirroring postgres.Pool() when mock-injected.
func newWithConn(c Conn) *Client { return &Client{conn: c, own: false} }

// Ping verifies connectivity.
func (c *Client) Ping(ctx context.Context) error {
	c.pings.Add(1)
	err := c.conn.Ping(ctx)
	if err != nil {
		c.pingErrors.Add(1)
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindPing, Outcome: OutcomeError})
		return err
	}
	c.fireEvent(Event{Kind: KindPing, Outcome: OutcomeSuccess})
	return nil
}

// Exec executes a query without returning rows (DDL/DML).
func (c *Client) Exec(ctx context.Context, query string, args ...any) error {
	c.execs.Add(1)
	err := c.conn.Exec(ctx, query, args...)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindExec, Outcome: OutcomeError})
		return err
	}
	c.fireEvent(Event{Kind: KindExec, Outcome: OutcomeSuccess})
	return nil
}

// Query executes a query that returns rows.
func (c *Client) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	c.queries.Add(1)
	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindQuery, Outcome: OutcomeError})
		return nil, err
	}
	c.fireEvent(Event{Kind: KindQuery, Outcome: OutcomeSuccess})
	return rows, nil
}

// QueryRow executes a query expected to return at most one row. The error (if
// any) surfaces on the returned row's Scan/Err — not here — so QueryRow does
// not increment the error counter.
func (c *Client) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
	c.queries.Add(1)
	row := c.conn.QueryRow(ctx, query, args...)
	c.fireEvent(Event{Kind: KindQuery, Outcome: OutcomeSuccess})
	return row
}

// PrepareBatch prepares an INSERT for bulk appending. The returned driver.Batch
// is the driver's own type — call Append/Send/Close on it exactly as upstream
// documents. Options are forwarded untouched.
func (c *Client) PrepareBatch(ctx context.Context, query string, opts ...driver.PrepareBatchOption) (driver.Batch, error) {
	c.batches.Add(1)
	batch, err := c.conn.PrepareBatch(ctx, query, opts...)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindBatch, Outcome: OutcomeError})
		return nil, err
	}
	c.fireEvent(Event{Kind: KindBatch, Outcome: OutcomeSuccess})
	return batch, nil
}

// Stats returns the driver's connection-pool statistics.
func (c *Client) Stats() driver.Stats { return c.conn.Stats() }

// Conn returns the underlying driver.Conn for anything the wrapper does not
// expose directly (Select, AsyncInsert, ServerVersion, ...). Returns nil when
// the Client was built from a mock (newWithConn).
func (c *Client) Conn() driver.Conn { return c.rawConn }

// Options returns the resolved options the client was built with.
func (c *Client) Options() Options { return c.opts }

// Close releases the connection. No-op for a wrapped or mock-injected client.
func (c *Client) Close() error {
	if !c.own {
		return nil
	}
	return c.conn.Close()
}
