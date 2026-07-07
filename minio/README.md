# minio

Thin, option-configured wrapper around [`github.com/minio/minio-go/v7`](https://github.com/minio/minio-go) — speaks **both MinIO and AWS S3** with one client.

Like the [`redis`](../redis) / [`postgres`](../postgres) / [`clickhouse`](../clickhouse) wrappers it stays small: ergonomic construction with sane defaults, a fail-fast connectivity/credentials check at construction, pass-through object-store operations, lightweight metrics + an event hook, and an escape hatch to the underlying `*minio.Client`. No retry policy beyond minio-go's own, no admin/replication/lifecycle ops, no domain types.

## Why

Object storage is core ad-tech/finance infra: creative/banner/video storage, report files, audit artifacts. `minio-go/v7` covers MinIO and AWS S3 with one client, so one wrapper serves both backends.

## Install

```
go get github.com/v8fg/kit4go/minio
```

Isolated Go module — importing it pulls minio-go's dependency tree into your module, not the rest of kit4go.

## Quick start

```go
c, err := minio.New(ctx,
    minio.WithEndpoint("play.min.io"),
    minio.WithCredentials("ACCESS", "SECRET"),
    minio.WithSecure(true),            // HTTPS by default
)
if err != nil {
    log.Fatal(err)
}

body := bytes.NewReader(payload)
if _, err := c.PutObject(ctx, "creatives", "banner-1.png", body, body.Size(), miniogo.PutObjectOptions{}); err != nil {
    log.Fatal(err)
}

url, err := c.PresignedGetObject(ctx, "creatives", "banner-1.png", time.Hour, nil)
```

## Operations

| Method | Notes |
|---|---|
| `PutObject` | upload from `io.Reader`; `-1` size streams unknown length |
| `GetObject` | returns `*minio.Object` (readable, seekable, Stat-able); caller closes |
| `StatObject` | metadata without the body |
| `RemoveObject` | delete one object |
| `BucketExists` / `MakeBucket` | bucket management |
| `ListObjects` | drains minio-go's channel; surfaces embedded errors |
| `PresignedGetObject` | short-lived download URL |

Advanced ops (FPutObject, bucket policy, admin, multipart tuning) are reached via `Client()` → `*minio.Client`.

## Construction

`New` opens the client and runs a `ListBuckets` ping (bounded by the context, 10s fallback) to **fail fast** on a bad endpoint or credentials — `minio.New` itself is lazy and would otherwise surface misconfiguration only on the first op.

## Options

`WithEndpoint` (required), `WithCredentials`, `WithSecure` (default **true** — HTTPS-by-default so a forgotten flag never ships plaintext credentials), `WithRegion`, `WithBucketLookup` (Auto/DNS/Path), `WithTransport` (custom `http.RoundTripper`; caller-owned).

## Metrics & events

```go
c.SetOnEvent(func(e minio.Event) { /* e.Kind, e.Outcome */ })
m := c.Metrics() // Puts, Gets, Stats, Removes, Errors, BytesUploaded
```

The hook runs on the calling goroutine; keep it cheap. When nil, the cost is a single atomic-pointer load per op (effectively zero overhead).

## Design notes

- **No `Close`**: `minio.Client` is stateless (an HTTP connection pool owned by `http.Transport`). Release a custom `Transport` set via `WithTransport` yourself; the default transport needs no teardown.
- **Mock seam**: a local `minioAPI` interface subset (which `*minio.Client` satisfies by structural typing) is the sole unit-test strategy — there is no miniredis-equivalent for S3. `Wrap(*minio.Client)` adopts an existing client.

## Testing

```
go test -short -race -cover ./...          # unit (mock), ~96% coverage
# integration (optional, needs a live endpoint):
docker run -d -p 9000:9000 -e MINIO_ROOT_USER=minio -e MINIO_ROOT_PASSWORD=minio123 minio/minio server /data
MINIO_ENDPOINT=127.0.0.1:9000 MINIO_ACCESS_KEY=minio MINIO_SECRET_KEY=minio123 \
  go test -run Integration -v ./minio/
```
