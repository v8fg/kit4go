// Package minio is a thin, option-configured wrapper around
// github.com/minio/minio-go/v7.
//
// It speaks both MinIO and AWS S3 (one client, two backends) and provides
// ergonomic construction (functional options + sane defaults), a fail-fast
// connectivity/credentials check at construction, pass-through object-store
// operations (PutObject/GetObject/StatObject/RemoveObject/BucketExists/
// MakeBucket/ListObjects/PresignedGetObject), lightweight metrics + an event
// hook, and an escape hatch to the underlying *minio.Client. Like the
// redis/postgres/clickhouse wrappers it deliberately stays small: no retry
// policy beyond minio-go's own, no admin/replication/lifecycle ops, no domain
// types.
//
// minio.Client is stateless (an HTTP connection pool owned by http.Transport),
// so there is intentionally no Close: release a custom Transport set via
// WithTransport yourself; the default transport needs no teardown.
package minio

import (
	"context"
	"errors"
	"io"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/minio/minio-go/v7"
)

// ErrNoEndpoint is returned by New when no endpoint was configured (WithEndpoint).
var ErrNoEndpoint = errors.New("minio: endpoint required (WithEndpoint)")

// minioAPI is the subset of *minio.Client that Client uses internally. The real
// *minio.Client satisfies it automatically by structural typing; tests inject a
// mock. Methods use minio's exact signatures (including the single non-variadic
// options struct and the *minio.Object / <-chan returns) — the wrapper is thin
// and never reinvents the minio API. ListBuckets is included for the
// construction-time connectivity/credentials ping.
type minioAPI interface {
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (*minio.Object, error)
	StatObject(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error)
	RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	MakeBucket(ctx context.Context, bucketName string, opts minio.MakeBucketOptions) error
	ListObjects(ctx context.Context, bucketName string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo
	ListBuckets(ctx context.Context) ([]minio.BucketInfo, error)
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expires time.Duration, reqParams url.Values) (*url.URL, error)
}

// Compile-time interface assertion: *minio.Client must satisfy our local
// minioAPI subset. Catches drift if minio changes a method signature.
var _ minioAPI = (*minio.Client)(nil)

// Client wraps a minio-go client. It is safe for concurrent use: all methods
// are goroutine-safe.
type Client struct {
	api  minioAPI      // local interface; mock seam
	raw  *minio.Client // non-nil when built from a real *minio.Client; nil when mock-injected
	opts Options

	puts, gets, stats, removes, errors, bytesUploaded atomic.Uint64
	onEvent                                           atomic.Pointer[func(Event)]
}

// opener opens a *minio.Client from an endpoint + minio-go Options. New uses
// the real minio.New (which takes the endpoint as its first arg, separate from
// Options); tests inject a fake via newClient.
type opener func(endpoint string, opts *minio.Options) (*minio.Client, error)

var defaultOpener opener = minio.New

// New connects to the endpoint, verifies connectivity and credentials with a
// ListBuckets call, and returns a Client.
//
// The context bounds the construction-time ping. If it carries no deadline, a
// 10s fallback is applied so a caller passing context.Background() against a
// half-open endpoint cannot block startup indefinitely.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	return newClient(ctx, opts, defaultOpener)
}

// newClient is the testable core of New: resolve options, open, ping, and (on
// success) return a Client. The open seam lets tests cover the open/ping paths
// without a live MinIO/S3 endpoint.
func newClient(ctx context.Context, opts []Option, open opener) (*Client, error) {
	o := withDefaults(opts)
	if o.Endpoint == "" {
		return nil, ErrNoEndpoint
	}
	raw, err := open(o.Endpoint, o.toDriver())
	if err != nil {
		return nil, err
	}
	c := &Client{api: raw, raw: raw, opts: o}
	pingCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		// Bound the construction ping: without this a context.Background()
		// caller blocks on a half-open endpoint until the TCP stack gives up.
		var cancel context.CancelFunc
		pingCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	// Fail fast on bad endpoint or credentials. minio.New is lazy (no dial), so
	// without this a misconfigured client surfaces only on the first op.
	if _, err := c.api.ListBuckets(pingCtx); err != nil {
		// minio.Client is stateless — nothing to close; just drop the reference.
		return nil, err
	}
	return c, nil
}

// Wrap adopts an existing *minio.Client (e.g. one constructed elsewhere). Useful
// for sharing a client. The wrapper adds metrics/events; the underlying client
// is untouched.
func Wrap(raw *minio.Client) *Client {
	return &Client{api: raw, raw: raw, opts: withDefaults(nil)}
}

// newWithAPI builds a Client from an injected minioAPI (testing only); raw is
// left nil so Client() returns nil, mirroring postgres.Pool() when mock-injected.
func newWithAPI(api minioAPI) *Client { return &Client{api: api, opts: withDefaults(nil)} }

// PutObject uploads an object from reader. objectSize is the byte length, or -1
// for streaming (minio-go then multipart-uploads an unknown size up to 5TiB).
// The returned UploadInfo carries Size; it is added to BytesUploaded.
func (c *Client) PutObject(ctx context.Context, bucket, object string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	c.puts.Add(1)
	info, err := c.api.PutObject(ctx, bucket, object, reader, objectSize, opts)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindPut, Outcome: OutcomeError})
		return minio.UploadInfo{}, err
	}
	c.bytesUploaded.Add(uint64(info.Size))
	c.fireEvent(Event{Kind: KindPut, Outcome: OutcomeSuccess})
	return info, nil
}

// GetObject downloads an object, returning a *minio.Object (readable, seekable,
// and Stat-able). The caller MUST Close it.
func (c *Client) GetObject(ctx context.Context, bucket, object string, opts minio.GetObjectOptions) (*minio.Object, error) {
	c.gets.Add(1)
	obj, err := c.api.GetObject(ctx, bucket, object, opts)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindGet, Outcome: OutcomeError})
		return nil, err
	}
	c.fireEvent(Event{Kind: KindGet, Outcome: OutcomeSuccess})
	return obj, nil
}

// StatObject fetches object metadata without downloading the body.
func (c *Client) StatObject(ctx context.Context, bucket, object string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
	c.stats.Add(1)
	info, err := c.api.StatObject(ctx, bucket, object, opts)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindStat, Outcome: OutcomeError})
		return minio.ObjectInfo{}, err
	}
	c.fireEvent(Event{Kind: KindStat, Outcome: OutcomeSuccess})
	return info, nil
}

// RemoveObject deletes an object. Pass minio.RemoveObjectOptions{} for defaults.
func (c *Client) RemoveObject(ctx context.Context, bucket, object string, opts minio.RemoveObjectOptions) error {
	c.removes.Add(1)
	err := c.api.RemoveObject(ctx, bucket, object, opts)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindRemove, Outcome: OutcomeError})
		return err
	}
	c.fireEvent(Event{Kind: KindRemove, Outcome: OutcomeSuccess})
	return nil
}

// BucketExists reports whether the bucket exists (and the caller has permission
// to see it).
func (c *Client) BucketExists(ctx context.Context, bucket string) (bool, error) {
	exists, err := c.api.BucketExists(ctx, bucket)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindBucket, Outcome: OutcomeError})
		return false, err
	}
	c.fireEvent(Event{Kind: KindBucket, Outcome: OutcomeSuccess})
	return exists, nil
}

// MakeBucket creates a bucket. Pass minio.MakeBucketOptions{} for defaults
// (set Region inside it for non-default regions).
func (c *Client) MakeBucket(ctx context.Context, bucket string, opts minio.MakeBucketOptions) error {
	err := c.api.MakeBucket(ctx, bucket, opts)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindBucket, Outcome: OutcomeError})
		return err
	}
	c.fireEvent(Event{Kind: KindBucket, Outcome: OutcomeSuccess})
	return nil
}

// ListObjects lists objects in a bucket matching opts (Prefix, Recursive, ...).
// It fully drains minio-go's result channel (required — an undrained channel
// leaks the producer goroutine) and surfaces any error embedded in the final
// ObjectInfo.Err.
func (c *Client) ListObjects(ctx context.Context, bucket string, opts minio.ListObjectsOptions) ([]minio.ObjectInfo, error) {
	ch := c.api.ListObjects(ctx, bucket, opts)
	var out []minio.ObjectInfo
	for info := range ch {
		if info.Err != nil {
			c.errors.Add(1)
			c.fireEvent(Event{Kind: KindList, Outcome: OutcomeError})
			return out, info.Err
		}
		out = append(out, info)
	}
	c.fireEvent(Event{Kind: KindList, Outcome: OutcomeSuccess})
	return out, nil
}

// PresignedGetObject returns a pre-signed URL for downloading an object without
// exposing credentials. reqParams (e.g. "response-content-type") may be nil.
func (c *Client) PresignedGetObject(ctx context.Context, bucket, object string, expires time.Duration, reqParams url.Values) (string, error) {
	u, err := c.api.PresignedGetObject(ctx, bucket, object, expires, reqParams)
	if err != nil {
		c.errors.Add(1)
		c.fireEvent(Event{Kind: KindPresign, Outcome: OutcomeError})
		return "", err
	}
	c.fireEvent(Event{Kind: KindPresign, Outcome: OutcomeSuccess})
	return u.String(), nil
}

// HealthCheck starts minio-go's background health-check goroutine (periodic
// connectivity probe) and returns a cancel func that stops it. It is a
// pass-through to *minio.Client and is a no-op on a mock-injected Client
// (Client() == nil). Most callers do not need this — the construction ping in
// New already validates connectivity. The returned cancel func is nil when no
// goroutine was started (mock-injected); call it when non-nil to stop probing.
func (c *Client) HealthCheck(d time.Duration) (context.CancelFunc, error) {
	if c.raw == nil {
		return nil, nil
	}
	return c.raw.HealthCheck(d)
}

// Client returns the underlying *minio.Client for anything the wrapper does not
// expose directly (FPutObject, bucket policy, admin ops, ...). Returns nil when
// the Client was built from a mock (newWithAPI).
func (c *Client) Client() *minio.Client { return c.raw }

// Options returns the resolved options the client was built with.
//
// The struct includes SecretKey: do not log or serialize it verbatim.
func (c *Client) Options() Options { return c.opts }
