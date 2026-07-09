# lru

A fixed-size, generically typed LRU cache with optional TTL and eviction
callbacks. Pure standard library (`container/list` + `sync`).

## Why

A bounded in-process cache is the first thing to reach for when a lookup is
hotter than a Redis round-trip (creative metadata, bidder config, frequency-cap
counters). This one is generic (no `any` / casts), thread-safe, and self-
contained — no background goroutine, no allocations on the hot path beyond the
map/list it maintains.

## API

```go
c := lru.New[string, *Bidder](
    lru.WithMaxSize[string, *Bidder](2048),
    lru.WithTTL[string, *Bidder](5*time.Minute),
    lru.WithOnEvicted[string, *Bidder](func(k string, b *Bidder) { b.Close() }),
)
```

| Method | Behavior |
|---|---|
| `Set(k, v)` | Insert/refresh; promotes; applies default TTL; evicts LRU if over size |
| `SetWithTTL(k, v, ttl)` | Per-entry TTL override (`0` = no expiry) |
| `Get(k) (V, bool)` | Returns value + promotes; expired entry evicted and reported miss |
| `Peek(k) (V, bool)` | Reads without promoting |
| `Contains(k) bool` | Present-and-not-expired check, no promotion |
| `Delete(k) bool` | Removes; fires `OnEvicted` |
| `DeleteExpired() int` | Sweeps all expired entries; returns count |
| `Len() int` / `Keys() []K` | Count / keys MRU→LRU |
| `Purge()` | Empties; fires `OnEvicted` for each |
| `Resize(n) int` | New max size; evicts down to it; returns evicted count (`n<=0` disables eviction) |

## Semantics

- **Recency**: `Get` and `Set` promote; `Peek`/`Contains` do not.
- **Eviction**: strict LRU when over `MaxSize`; `OnEvicted` fires once per
  departure (eviction, delete, expiry, purge, resize). It runs WITHOUT the cache
  lock held, so it may safely call back into the cache (Get/Set/...). Keep it
  cheap; it runs on the caller's goroutine.
- **Expiry**: lazy — checked on access and on `DeleteExpired`/`Purge`. There is
  no sweeper goroutine, so expired entries occupy memory until touched. Call
  `DeleteExpired` periodically if that matters.
- **MaxSize <= 0** disables eviction (unbounded) — use deliberately.

## Ad-tech uses

- **Creative / bidder config cache** keyed by SSP+creative ID, TTL'd so stale
  entries refresh on their own.
- **Frequency-cap counters** — recent impression counts per user hash, evicted
  LRU so memory stays bounded.
- **Idempotency window** — recently-seen auction IDs with a short TTL.

## Testing

96% statement coverage, `-race` clean. Covers eviction order, recency
promotion, per-entry TTL, refresh-resets-TTL, expiry sweep, resize, purge,
onEvicted firing, and a concurrent reader/writer stress run. TTL is tested with
an injected clock (no sleeping).

```bash
go test -race -cover ./lru/...
```
