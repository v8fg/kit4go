# hyperloglog

Estimates the number of **distinct** elements in a stream (cardinality) using a
fixed, small amount of memory — the HyperLogLog algorithm. Pure standard library.

## Why

Counting uniques over a high-volume stream (unique users, unique auctions,
audience size) by storing every ID is infeasible at scale. HyperLogLog uses a
few bytes of register per ~10K distinct and estimates cardinality to ~0.8%
relative error regardless of how many millions you add — duplicates don't move
the estimate. It is the "how many?" companion to bloom's "have I seen this?".

## API

```go
h, _ := hyperloglog.New(14)        // precision 14 -> m=16384, ~0.8% error, ~16KB
for _, uid := range stream {
    h.AddString(uid)
}
distinct := uint64(h.Estimate())
```

| Symbol | Behavior |
|---|---|
| `New(precision)` | 2^p registers; precision 4–16 (14 is the sweet spot) |
| `Add(data)` / `AddString(s)` | Record an element (DefaultHash = FNV-1a 64 + splitmix64) |
| `AddHashed(x)` | Record a precomputed 64-bit hash (skip re-hashing) |
| `Estimate() float64` | Approximate distinct count (small/large-range corrected) |
| `Merge(other)` | Union sketches (per-register max) — the concurrency model |
| `Reset()` | Clear registers |

## Concurrency model

`Add` is **not** internally synchronized (it is the hot path — locking would
defeat the purpose). For concurrent producers, give each a per-shard
`HyperLogLog` and `Merge` them: Merge takes the per-register max, so the union is
exact-regardless-of-order. `Estimate`/`Merge` are safe when no `Add` is in flight.

## Accuracy

Relative error ≈ `1.04 / sqrt(2^p)`:
- p=12 (m=4096): ~1.6%
- p=14 (m=16384): ~0.8%  ← default
- p=16 (m=65536): ~0.4%

Small cardinalities use linear counting (no high bias); large ones use the
2^32 correction. Verified: 50K distinct lands within 6% at p=12 and p=14.

## Ad-tech uses

- **Unique users / sessions / auctions** over impression/bid streams.
- **Audience size** estimation without a full ID store.
- **Reach** computation across shards (Merge) and time windows.

## Testing

98% statement coverage, `-race` clean. Covers precision bounds, empty estimate,
distinct-count accuracy (50K within 6%), duplicates-don't-inflate, reset, merge
(+ incompatible-precision error), determinism, AddHashed≡Add, the rho upper-bound
clamp, low-precision alpha branches, and a sharded-merge concurrency run.

```bash
go test -race -cover ./hyperloglog/...
```
