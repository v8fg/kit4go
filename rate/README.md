# rate

A distributed, Redis-backed rate limiter using the Generic Cell Rate Algorithm
(GCRA) — the same algorithm as redis-cell and `go-redis/redis_rate`. One Redis
holds a per-key "theoretical arrival time"; a Lua script checks and advances it
atomically, so the limit holds across every process sharing the Redis.

## Why

It is the **distributed** sibling of the in-process `limiter`: use it when many
instances must agree on a global rate. GCRA is a single number per key (memory
and wire are tiny) and one atomic round-trip per decision — well-suited to
per-user / per-tenant limits at scale.

## API

```go
lim := rate.New(redisCmdable)               // any redis.Cmdable

ok, err := lim.Allow(ctx, "user:42", rate.PerSecond(100, 50)) // 100/s, burst 50
// Result{Allowed, Remaining, RetryAfter}

r, err := lim.AllowN(ctx, "batch:k", rate.PerMinute(600, 600), 25) // consume 25
```

| Symbol | Behavior |
|---|---|
| `New(client, opts...)` | Build (pass any `redis.Cmdable`) |
| `Allow(ctx, key, limit)` | Check + consume 1 token |
| `AllowN(ctx, key, limit, n)` | Check + consume n tokens (denied whole if n unavailable) |
| `PerSecond(rate, burst)`, `PerMinute(rate, burst)` | Limit builders |
| `WithClock(f)` | Inject a clock (tests) |

`Result` carries `Allowed`, `Remaining`, and `RetryAfter` (how long until a token
is free, when denied).

## Semantics

- **Burst**: the bucket can hold up to `Burst` tokens; they refill at `Rate` per
  `Period` (continuous, not tick-aligned).
- **AllowN**: requests n tokens at once; denied entirely if n exceeds what's
  available (the bucket is not partially consumed). Asking for more than `Burst`
  is always denied.
- **Atomic**: each decision is one Redis Lua call — safe under concurrency across
  instances. The TAT key carries a TTL so idle keys expire.

## Ad-tech / finance / push / chain uses

- A shared **bid QPS cap** across a bidder fleet.
- **Per-user / per-tenant API budgets** (free-tier limits).
- Cross-pod **postback / webhook throttle** so a downstream isn't overwhelmed.
- **Onboarding / signup** rate caps.

## Testing

96% statement coverage, `-race` clean, against an in-process miniredis. Covers
burst exhaustion + denial, recovery after one emission interval (injected clock),
independent keys, multi-token AllowN (partial-denial preserves the bucket),
cost-exceeding-burst denial, non-negative Remaining, invalid-limit guards, and
per-minute limits. The Lua runs for real against miniredis (not mocked).

```bash
go test -race -cover ./...
```
