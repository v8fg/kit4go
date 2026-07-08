# etcd

Thin, option-configured wrapper around [`go.etcd.io/etcd/client/v3`](https://pkg.go.dev/go.etcd.io/etcd/client/v3) v3.6.

Targets the dominant etcd use case in ad-tech/finance services — **service registration** (Put + Lease) and **discovery** (Get + Watch). A scan of 11 local Go projects using etcd showed KV + Lease at 100% adoption, Watch at 73%, and Lock/Mutex/Election at **0%** — so this wrapper covers KV + Lease + Watch and deliberately omits the `concurrency` primitives (reach them via `Client()` if needed).

## Why

etcd is the standard distributed coordination store: service registration/discovery, distributed config, leader-elected singletons. A thin, ergonomic wrapper with metrics + a fail-fast ping lowers the boilerplate every service re-implements.

## Install

```
go get github.com/v8fg/kit4go/etcd
```

Isolated Go module — importing it pulls etcd's gRPC/protobuf/zap tree into your module, not the rest of kit4go.

## Quick start

```go
ctx := context.Background()
c, err := etcd.New(ctx, etcd.WithEndpoints("http://localhost:2379"))
if err != nil { log.Fatal(err) }
defer c.Close()

// Register with a lease: the key auto-expires if the process stops keep-aliving.
lease, _ := c.Grant(ctx, 30)
c.Put(ctx, "/services/bidder/inst-1", "10.0.0.1:8080", clientv3.WithLease(lease.ID))

// Discover.
resp, _ := c.Get(ctx, "/services/bidder/", clientv3.WithPrefix())
for _, kv := range resp.Kvs { /* kv.Key, kv.Value */ }
```

## Operations

| Method | Notes |
|---|---|
| `Put` / `Get` / `Delete` | KV; forward `OpOption`s (`WithPrefix`, `WithLease`, `WithLimit`, ...) untouched |
| `Grant` / `KeepAlive` / `Revoke` | lease lifecycle; KeepAlive returns a channel the caller drains |
| `Watch` | subscribe to key/prefix changes; returns etcd's `WatchChan` |
| `Status` | cluster health (also used by the construction ping) |

Txn, Compact, Cluster, Auth, and the `concurrency` package (Mutex/Lock/Election) are reached via `Client()` → `*clientv3.Client`.

## Construction

`New` opens the client and runs a `Status` ping (bounded by the context, 10s fallback) to **fail fast** on an unreachable cluster — gRPC dial can succeed against a dead peer, surfacing the failure only on the first op otherwise.

## Options

`WithEndpoints` (required), `WithDialTimeout` (default 5s), `WithDialKeepAliveTime`, `WithTLSConfig`, `WithUsername`/`WithPassword`, `WithAutoSyncInterval`, `WithRejectOldCluster`.

## Metrics & events

```go
c.SetOnEvent(func(e etcd.Event) { /* e.Kind, e.Outcome */ })
m := c.Metrics() // Puts, Gets, Deletes, Grants, Watches, Errors
```

The hook runs on the calling goroutine; keep it cheap. When nil, the cost is a single atomic-pointer load per op (effectively zero overhead).

## Mock seam

`*clientv3.Client` embeds the `KV`/`Lease`/`Watcher`/`Maintenance` **interfaces**, whose methods promote to the client. A local `etcdAPI` interface subset is therefore satisfied by structural promotion — the sole unit-test strategy (there is no miniredis-equivalent for etcd). `Wrap(*clientv3.Client)` adopts an existing client.

## Testing

```
go test -short -race -cover ./...          # unit (mock), ~96% coverage
# integration (optional, needs a live cluster):
docker run -d -p 2379:2379 -e ALLOW_NONE_AUTHENTICATION=yes bitnami/etcd
ETCD_ENDPOINT=http://127.0.0.1:2379 go test -run Integration -v ./etcd/
```
