// Package mongo is a thin, option-configured wrapper around
// go.mongodb.org/mongo-driver (the official Go driver).
//
// It targets the dominant local use — document CRUD (Find/Insert/Update/Delete)
// — and provides ergonomic construction (functional options + sane defaults), a
// fail-fast Ping at construction, a Collection wrapper that adds metrics + an
// event hook to every op, an escape hatch to the underlying *mongo.Collection /
// *mongo.Client, and a graceful Disconnect. CountDocuments/Aggregate/BulkWrite
// are deliberately NOT wrapped (0 local usage — reach them via Collection()).
package mongo

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// ErrNoURI is returned by New when no connection URI was configured (WithURI).
var ErrNoURI = errors.New("mongo: connection URI required (WithURI)")

// clientAPI is the subset of *mongo.Client the wrapper uses. *mongo.Client is a
// concrete struct; it satisfies this subset by structural typing. Tests inject a
// mock at the Collection level (not here) — the Client always wraps a real
// *mongo.Client produced by Connect.
type clientAPI interface {
	Ping(ctx context.Context, rp *readpref.ReadPref) error
	Database(name string, opts ...*options.DatabaseOptions) *mongo.Database
	Disconnect(ctx context.Context) error
}

// collectionAPI is the subset of *mongo.Collection the wrapper calls.
// *mongo.Collection (returned by Database().Collection()) satisfies it by
// structural typing; tests inject a mock. This is the sole unit-test seam —
// there is no miniredis-equivalent for MongoDB.
type collectionAPI interface {
	InsertOne(ctx context.Context, document any, opts ...*options.InsertOneOptions) (*mongo.InsertOneResult, error)
	InsertMany(ctx context.Context, documents []any, opts ...*options.InsertManyOptions) (*mongo.InsertManyResult, error)
	Find(ctx context.Context, filter any, opts ...*options.FindOptions) (*mongo.Cursor, error)
	FindOne(ctx context.Context, filter any, opts ...*options.FindOneOptions) *mongo.SingleResult
	UpdateOne(ctx context.Context, filter any, update any, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error)
	UpdateMany(ctx context.Context, filter any, update any, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error)
	DeleteOne(ctx context.Context, filter any, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error)
	DeleteMany(ctx context.Context, filter any, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error)
}

// Compile-time assertions: the upstream types satisfy our local subsets.
var (
	_ clientAPI     = (*mongo.Client)(nil)
	_ collectionAPI = (*mongo.Collection)(nil)
)

// Client wraps a mongo-driver client. It owns the connection (Connect/Disconnect)
// and carries the shared metrics + event hook that every Collection op bumps.
// Safe for concurrent use; Disconnect is idempotent.
type Client struct {
	raw    *mongo.Client // non-nil when built from a real client; nil when no real client (mock-backed Collection tests)
	own    bool          // true -> Disconnect closes the underlying client
	opts   Options
	dbName string // default database (optional; applied by Collection when db == "")

	inserts, finds, updates, deletes, errors atomic.Uint64
	onEvent                                  atomic.Pointer[func(Event)]
}

// connector connects to MongoDB. New uses mongo.Connect; tests inject a fake.
type connector func(ctx context.Context, opts ...*options.ClientOptions) (*mongo.Client, error)

var defaultConnector connector = mongo.Connect

// New connects, pings (readpref.Primary), and returns an owning Client.
//
// The context bounds the construction-time Ping. If it carries no deadline, a
// 10s fallback is applied. mongo.Connect does not guarantee the server is live,
// so the Ping fail-fast on an unreachable node.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	return newClient(ctx, opts, defaultConnector)
}

// newClient is the testable core of New: resolve options, connect, ping, and (on
// success) return an owning Client.
func newClient(ctx context.Context, opts []Option, connect connector) (*Client, error) {
	o := withDefaults(opts)
	if o.URI == "" {
		return nil, ErrNoURI
	}
	raw, err := connect(ctx, o.toDriver())
	if err != nil {
		return nil, err
	}
	pingCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		pingCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	if err := raw.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = raw.Disconnect(context.Background()) // release on ping failure
		return nil, err
	}
	return &Client{raw: raw, own: true, opts: o, dbName: o.Database}, nil
}

// Wrap adopts an existing *mongo.Client. The Client does not own it: Disconnect
// is a no-op. Useful for sharing a client.
func Wrap(raw *mongo.Client, opts ...Option) *Client {
	o := withDefaults(opts)
	return &Client{raw: raw, own: false, opts: o, dbName: o.Database}
}

// newCollection builds a Collection from an injected collectionAPI (testing only).
// The Client carries the metrics; pass a real or test Client to aggregate into.
func (c *Client) newCollection(api collectionAPI, db, coll string) *Collection {
	return &Collection{api: api, client: c, dbName: db, collName: coll}
}

// Collection returns a wrapper around the named database/collection whose ops
// bump this Client's metrics and fire its event hook. db == "" falls back to the
// Client's default database (Database option / WithDatabase), if any.
func (c *Client) Collection(db, coll string) *Collection {
	if db == "" {
		db = c.dbName
	}
	raw := c.raw.Database(db).Collection(coll)
	return c.newCollection(raw, db, coll)
}

// Disconnect releases the underlying connection. No-op for a wrapped client.
func (c *Client) Disconnect(ctx context.Context) error {
	if !c.own {
		return nil
	}
	return c.raw.Disconnect(ctx)
}

// Client returns the underlying *mongo.Client for anything the wrapper does not
// expose directly (ChangeStreams, sessions, transactions, raw Database access).
// Returns nil when no real client backs the Client (mock-backed Collection tests).
func (c *Client) Client() *mongo.Client { return c.raw }

// Options returns the resolved options the client was built with.
//
// The struct may carry credentials inside the URI: do not log it verbatim.
func (c *Client) Options() Options { return c.opts }

// Collection wraps a *mongo.Collection. Every op bumps the owning Client's
// metrics and fires its event hook. The underlying *mongo.Collection is reached
// via Collection().
type Collection struct {
	api      collectionAPI
	client   *Client
	dbName   string
	collName string
}

// Collection returns the underlying *mongo.Collection for anything the wrapper
// does not expose (Aggregate, CountDocuments, BulkWrite, FindOneAndUpdate, ...).
// Returns nil when the Collection was built from a mock.
func (c *Collection) Collection() *mongo.Collection {
	if raw, ok := c.api.(*mongo.Collection); ok {
		return raw
	}
	return nil
}

// InsertOne inserts a single document.
func (c *Collection) InsertOne(ctx context.Context, document any, opts ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
	c.client.inserts.Add(1)
	res, err := c.api.InsertOne(ctx, document, opts...)
	if err != nil {
		c.client.errors.Add(1)
		c.client.fireEvent(Event{Kind: KindInsert, Outcome: OutcomeError})
		return nil, err
	}
	c.client.fireEvent(Event{Kind: KindInsert, Outcome: OutcomeSuccess})
	return res, nil
}

// InsertMany inserts multiple documents.
func (c *Collection) InsertMany(ctx context.Context, documents []any, opts ...*options.InsertManyOptions) (*mongo.InsertManyResult, error) {
	c.client.inserts.Add(1)
	res, err := c.api.InsertMany(ctx, documents, opts...)
	if err != nil {
		c.client.errors.Add(1)
		c.client.fireEvent(Event{Kind: KindInsert, Outcome: OutcomeError})
		return nil, err
	}
	c.client.fireEvent(Event{Kind: KindInsert, Outcome: OutcomeSuccess})
	return res, nil
}

// Find runs a query and returns a Cursor. The caller MUST close the cursor.
func (c *Collection) Find(ctx context.Context, filter any, opts ...*options.FindOptions) (*mongo.Cursor, error) {
	c.client.finds.Add(1)
	cur, err := c.api.Find(ctx, filter, opts...)
	if err != nil {
		c.client.errors.Add(1)
		c.client.fireEvent(Event{Kind: KindFind, Outcome: OutcomeError})
		return nil, err
	}
	c.client.fireEvent(Event{Kind: KindFind, Outcome: OutcomeSuccess})
	return cur, nil
}

// FindOne returns at most one matching document. The error (if any) surfaces on
// the returned SingleResult's Decode/Err — not here — so FindOne does not
// increment the error counter (mirrors clickhouse QueryRow).
func (c *Collection) FindOne(ctx context.Context, filter any, opts ...*options.FindOneOptions) *mongo.SingleResult {
	c.client.finds.Add(1)
	res := c.api.FindOne(ctx, filter, opts...)
	c.client.fireEvent(Event{Kind: KindFind, Outcome: OutcomeSuccess})
	return res
}

// UpdateOne updates the first matching document.
func (c *Collection) UpdateOne(ctx context.Context, filter, update any, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	c.client.updates.Add(1)
	res, err := c.api.UpdateOne(ctx, filter, update, opts...)
	if err != nil {
		c.client.errors.Add(1)
		c.client.fireEvent(Event{Kind: KindUpdate, Outcome: OutcomeError})
		return nil, err
	}
	c.client.fireEvent(Event{Kind: KindUpdate, Outcome: OutcomeSuccess})
	return res, nil
}

// UpdateMany updates all matching documents.
func (c *Collection) UpdateMany(ctx context.Context, filter, update any, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	c.client.updates.Add(1)
	res, err := c.api.UpdateMany(ctx, filter, update, opts...)
	if err != nil {
		c.client.errors.Add(1)
		c.client.fireEvent(Event{Kind: KindUpdate, Outcome: OutcomeError})
		return nil, err
	}
	c.client.fireEvent(Event{Kind: KindUpdate, Outcome: OutcomeSuccess})
	return res, nil
}

// DeleteOne deletes the first matching document.
func (c *Collection) DeleteOne(ctx context.Context, filter any, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
	c.client.deletes.Add(1)
	res, err := c.api.DeleteOne(ctx, filter, opts...)
	if err != nil {
		c.client.errors.Add(1)
		c.client.fireEvent(Event{Kind: KindDelete, Outcome: OutcomeError})
		return nil, err
	}
	c.client.fireEvent(Event{Kind: KindDelete, Outcome: OutcomeSuccess})
	return res, nil
}

// DeleteMany deletes all matching documents.
func (c *Collection) DeleteMany(ctx context.Context, filter any, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
	c.client.deletes.Add(1)
	res, err := c.api.DeleteMany(ctx, filter, opts...)
	if err != nil {
		c.client.errors.Add(1)
		c.client.fireEvent(Event{Kind: KindDelete, Outcome: OutcomeError})
		return nil, err
	}
	c.client.fireEvent(Event{Kind: KindDelete, Outcome: OutcomeSuccess})
	return res, nil
}
