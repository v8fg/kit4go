# elasticsearch

Thin, option-configured wrapper around the official [`github.com/elastic/go-elasticsearch/v8`](https://pkg.go.dev/github.com/elastic/go-elasticsearch/v8) (low-level `esapi`).

Covers document CRUD + search — Index/Get/Search/Delete — and adds ergonomic construction (functional options + sane defaults), a fail-fast Ping at construction, lightweight metrics + an event hook, and an escape hatch to `*elasticsearch.Client`. Bulk/Aggregation/Cat/Indices/Cluster are reached via `Client()` (not wrapped). There is no Close (elasticsearch.Client is stateless — an HTTP pool owned by `http.Transport`).

Like the minio/etcd/mongo wrappers, `New` and all ops take a `context.Context`: it bounds the construction Ping and each HTTP call (applied via `esapi.X.WithContext`). When the caller's context has no deadline, `New` applies a 10s fallback to the Ping so a `context.Background()` caller cannot block startup on a half-open endpoint.

## Why

Elasticsearch is the default search/analytics backend (creative/event/log search) for ad-tech/finance. A thin wrapper with metrics + a fail-fast ping + consistent options removes per-service boilerplate. Uses the **official** client (the local projects use the semi-maintained `olivere/elastic`; this targets the maintained `go-elasticsearch/v8`).

## Install

```
go get github.com/v8fg/kit4go/elasticsearch
```

Isolated Go module.

## Quick start

```go
ctx := context.Background()
c, err := elasticsearch.New(ctx, elasticsearch.WithAddresses("http://localhost:9200"))
if err != nil { log.Fatal(err) }

c.Index(ctx, "creatives", strings.NewReader(`{"name":"banner-1"}`),
    esapi.Index(nil).WithDocumentID("1"),
)

res, _ := c.Search(ctx,
    esapi.Search(nil).WithIndex("creatives"),
    esapi.Search(nil).WithBody(strings.NewReader(`{"query":{"match_all":{}}}`)),
)
defer res.Body.Close()
```

> **v8.19 options**: in go-elasticsearch v8.19, the option helpers (WithDocumentID, WithIndex, WithBody, ...) are **methods on the named func types** — build them with `esapi.Index(nil).WithDocumentID("1")` (the method builds the option without invoking the nil receiver), or via the escape hatch `c.Client().Index.WithDocumentID("1")`.

## Operations

| Method | Notes |
|---|---|
| `Index(ctx, index, body, opts...)` | create/replace a doc (body = JSON `io.Reader`); returns `*esapi.Response` |
| `Get(ctx, index, id, opts...)` | fetch by id |
| `Search(ctx, opts...)` | query (body via `WithBody`) |
| `Delete(ctx, index, id, opts...)` | remove by id |

Returns `*esapi.Response` (`.StatusCode`, `.Body`). Only a **transport error** (`err != nil`) is counted in `Errors` — HTTP-level outcomes (404 etc.) are in `resp.StatusCode` (inspect directly). Bulk/Indices/Cat/Cluster/Aggregations via `Client()`.

## Construction

`New(ctx, opts...)` builds the client and runs a `Ping` (200 = OK) to **fail fast** on an unreachable cluster — `elasticsearch.NewClient` is lazy (does not connect). `ctx` bounds the Ping; when it has no deadline, a 10s fallback is applied. There is no Close (stateless client). A non-2xx Ping returns an error wrapping the `ErrPingFailed` sentinel (`errors.Is(err, elasticsearch.ErrPingFailed)`).

## Options

`WithAddresses` (required, or `WithCloudID`), `WithCredentials` (basic auth), `WithCloudID` (Elastic Cloud), `WithCACert` (PEM CA cert for TLS), `WithTransport`.

## Metrics & events

```go
c.SetOnEvent(func(e elasticsearch.Event) { /* e.Kind, e.Outcome */ })
m := c.Metrics() // Indexes, Searches, Gets, Deletes, Errors
```

## Mock seam

v8.19 exposes `Index`/`Search`/`Get`/`Delete`/`Ping` as **fields of named func types** (`esapi.Index`, etc.) on `*elasticsearch.Client`. The wrapper holds these func fields directly (copied from the client) — tests overwrite them with their own funcs. No adapter or interface layer needed.

## Testing

```
go test -short -race -cover ./...          # unit (mock func fields), ~98% coverage
# integration (optional, needs a live cluster):
docker run -d -p 9200:9200 -e discovery.type=single-node -e xpack.security.enabled=false \
  docker.elastic.co/elasticsearch/elasticsearch:8.19.0
ELASTICSEARCH_ADDR=http://127.0.0.1:9200 go test -run Integration -v ./elasticsearch/
```
