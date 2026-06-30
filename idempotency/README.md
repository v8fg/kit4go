# idempotency

An in-process idempotency cache: concurrent or near-term repeat calls with the
same key are coalesced into a single execution, and the successful result is
served from cache until it expires. Pure standard library.

## Why

Two guarantees combine behind one `Do` call:

1. **Singleflight** — while one call for a key is in flight, later callers wait
   for the leader's result instead of re-running the work.
2. **Result cache** — after the leader succeeds, the result is returned to all
   callers within the TTL, so a retried request yields the original outcome
   (Stripe-style `Idempotency-Key` semantics, in-process).

A failed call is **not** cached by default, so the next caller retries. Set
`WithCacheErrors(true)` to also persist failures (hard de-dup).

## API

```go
c := idempotency.New[*Order](
    idempotency.WithTTL[*Order](5*time.Minute),
    idempotency.WithMaxEntries[*Order](10000),
)

order, err := c.Do(ctx, idempotencyKey, func(ctx context.Context) (*Order, error) {
    return chargeCard(ctx, key)   // runs at most once per TTL window
})
```

| Method | Behavior |
|---|---|
| `Do(ctx, key, fn)` | Run `fn` once; concurrent/repeat callers get the same result |
| `Forget(key)` / `Clear()` | Drop a key / all keys (force re-run) |
| `Len()` | Tracked keys (in-flight + cached) |

| Option | Default | Effect |
|---|---|---|
| `WithTTL(d)` | 1m | Success cache lifetime (0 = no expiry) |
| `WithMaxEntries(n)` | 4096 | Cap; evicts expired-then-oldest (never an in-flight leader) |
| `WithCacheErrors(true)` | off | Cache failures too (no retry) |

Followers whose `ctx` is cancelled while waiting return `ctx.Err()` immediately;
the leader still completes and caches.

## Ad-tech / finance / push / chain uses

- **Concurrent bid dedup** — two requests for the same auction ID run the bid
  logic once.
- **Payment / charge idempotency** — a retried checkout with the same key returns
  the original outcome, never double-charging.
- **Webhook / postback delivery dedup** — a redelivered event is served from
  cache.
- **Transaction dedup** — same for chain tx submission.

For cross-instance idempotency (multiple pods), back the store with Redis; this
package covers the common in-process / short-window case with zero dependencies.

## Testing

95% statement coverage, `-race` clean. The headline test runs 50 concurrent
callers for one key under `-race` and asserts `fn` ran **exactly once** with all
observing the same result. Also covers sequential cache hits, error-not-cached
retry, `WithCacheErrors`, TTL expiry (injected clock, no sleeping), TTL=0,
follower ctx-cancellation, Forget/Clear, and eviction (expired-first, never an
in-flight leader).

```bash
go test -race -cover ./idempotency/...
```
