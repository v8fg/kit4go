# redis

A thin, option-configured wrapper around [go-redis/v9](https://github.com/redis/go-redis)
that picks single-node / cluster / sentinel wiring and exposes the underlying
`redis.Cmdable` for the full command surface.

## Why

go-redis is already excellent; this wrapper adds three things that come up every
time: ergonomic construction with functional options, automatic topology
selection from the address list, and a health check. It deliberately does not
re-wrap every command — reach the full API through `Client.Cmdable()`.

## Construct

```go
c, err := redis.New(
    redis.WithAddrs("redis-1:6379", "redis-2:6379"), // >1 addr + ModeAuto => cluster
    redis.WithPassword(os.Getenv("REDIS_PASSWORD")),
    redis.WithPoolSize(32),
    redis.WithReadTimeout(200*time.Millisecond),
    redis.WithClientName("bidder"),
)
if err != nil { panic(err) }
defer c.Close()

ctx := context.Background()
if err := c.Ping(ctx); err != nil { /* unhealthy */ }

cmd := c.Cmdable()
cmd.Set(ctx, "budget:camp42", "100", time.Second).Err()
```

## Topology

| Setting | Result |
|---|---|
| `WithAddrs(a)` (1 addr), default mode | single-node `*redis.Client` |
| `WithAddrs(a, b, …)` (>1 addr), default mode | cluster `*redis.ClusterClient` |
| `WithMode(redis.ModeSingle)` | single-node even with many addrs |
| `WithMode(redis.ModeCluster)` | cluster even with one addr |
| `WithMasterName("m")` + sentinel addrs | sentinel failover `*redis.Client` |

## Options

`WithAddrs`, `WithMode`, `WithUsername`, `WithPassword`, `WithDB`,
`WithMasterName`, `WithDialTimeout`, `WithReadTimeout`, `WithWriteTimeout`,
`WithPoolSize`, `WithMinIdleConns`, `WithMaxRetries`, `WithClientName`,
`WithTLSConfig`.

## Wrap (tests / DI)

`redis.Wrap(cmdable)` adopts an existing `redis.Cmdable` without owning it:
`Close` is a no-op and the caller keeps responsibility for the underlying
client. Use it to inject a [miniredis](https://github.com/alicebob/miniredis)-backed
client in tests.

## API surface

| Method | Behavior |
|---|---|
| `New(opts...) (*Client, error)` | Build + own the underlying client |
| `Wrap(cmdable) *Client` | Adopt an existing Cmdable (not owned) |
| `Cmdable() redis.Cmdable` | Full go-redis command surface |
| `Ping(ctx) error` | Health check |
| `Close() error` | Closes owned client; no-op for wrapped |
| `Options() Options` | Resolved construction options |
| `PoolStats() redis.PoolStats` | Connection-pool stats when available |

## Ad-tech uses

- Real-time **budget / pacing** state, **frequency capping**, and
  **user/session lookups** — sub-millisecond reads that should not hit a DB.
- Pair with a distributed lock (`redislock`) for single-flight budget updates
  or leader election.

## Testing

92% statement coverage, `-race` clean, against an in-process miniredis. Covers
construction (single/cluster/sentinel), mode selection, Ping, Set/Get
round-trip, auth, option resolution, wrap semantics, and pool stats. Cluster
command execution and Sentinel failover are integration-only (real brokers).

```bash
go test -race -cover ./...
```
