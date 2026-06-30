# cache

A unified, pluggable cache interface (`Store[V]`) with a thread-safe in-memory
backend backed by kit4go/lru. Pure standard library (reuses lru — zero new deps).

## Why

Start with an in-process cache; switch to a distributed backend (Redis, via a
separate module) behind the same `Store` interface when multi-pod needs a shared
cache. The interface is context-aware so a network backend drops in without
changing call sites; the in-memory backend stores typed values directly
(zero-copy, no serialization).

## API

```go
c := cache.NewMemory[*Creative](
    cache.WithMaxSize[*Creative](2048),
    cache.WithDefaultTTL[*Creative](5*time.Minute),
)
c.Set(ctx, "creative:42", cr, 0)        // uses default TTL
v, err := c.Get(ctx, "creative:42")     // typed *Creative, or ErrMiss
c.Delete(ctx, "creative:42")
c.Has(ctx, "creative:42")
```

| Symbol | Behavior |
|---|---|
| `Store[V]` interface | `Get(ctx, key) (V, error)`, `Set(ctx, key, val, ttl)`, `Delete`, `Has` |
| `NewMemory[V](opts...)` | In-memory Store (wraps lru, thread-safe) |
| `WithMaxSize(n)` | Max entries before LRU eviction (default 1024) |
| `WithDefaultTTL(d)` | TTL applied when Set is called with ttl=0 |
| `ErrMiss` | Sentinel for not-found / expired |

`Set(ctx, key, val, ttl)`: ttl=0 → no expiry (or default TTL if set); ttl>0 →
per-entry TTL. `Get` returns `ErrMiss` for absent or expired keys.

## Ad-tech uses

- **Creative / bidder config cache** — hot lookups that avoid a DB/Redis hop.
- **User profile fragments** — session-scoped, TTL'd.
- **Idempotency window** — recently-seen IDs with a short TTL.
- Start in-memory; switch the same `Store` interface to Redis when pods share.

## Testing

100% statement coverage, `-race` clean. Covers set/get round-trip, miss,
delete, TTL expiry, default TTL, explicit-TTL-overrides-default, max-size LRU
eviction, zero-value storage, typed values (struct), interface satisfaction, and
the ErrMiss sentinel.

```bash
go test -race -cover ./cache/...
```
