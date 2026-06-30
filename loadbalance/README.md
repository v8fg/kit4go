# loadbalance

Selects an item from a weighted set under a pluggable strategy. Generic,
thread-safe, pure standard library (`math/rand/v2` + `sync`).

## Strategies

| Strategy | Behavior |
|---|---|
| `StrategySmoothWeightedRR` (default) | nginx smooth weighted round-robin: proportional, burst-free interleaving |
| `StrategyRoundRobin` | cyclic, ignores weights |
| `StrategyRandom` | uniform random |
| `StrategyWeightedRandom` | random with probability proportional to weight |

Smooth WRR is the right default: it distributes picks in proportion to weight
**and** interleaves them, so a heavy upstream never receives a burst of back-
to-back requests. Weighted random is simpler but bursty; plain round-robin
ignores capacity differences.

## API

```go
b := loadbalance.New(
    func(s string) string { return s },          // id (for Add/Remove dedup)
    []loadbalance.Entry[string]{
        {Value: "ssp-fast:443", Weight: 3},
        {Value: "ssp-backup:443", Weight: 1},
    },
)
upstream, ok := b.Next()       // ~75% fast / ~25% backup, smoothly interleaved
b.Add(loadbalance.Entry[string]{Value: "ssp-3:443", Weight: 2})
b.Remove("ssp-backup:443")
b.Len(); b.All()
```

| Method | Behavior |
|---|---|
| `New(id, entries, opts...)` | Build; `id` is the per-value identity for Add/Remove |
| `WithStrategy(s)` | Choose strategy (default SWRR) |
| `Next() (T, bool)` | Pick; `false` when empty |
| `Add(Entry)` | Insert, or replace same-id (resets its weight + SWRR state) |
| `Remove(value)` | Drop by id |
| `Len()` / `All()` | Count / snapshot copy of value+weight |

A weight `<= 0` is normalized to 1.

## Properties (verified by tests)

- **SWRR canonical**: weights 5:1:1 over one cycle yield exactly 5/1/1 picks,
  with no run of the heavy node longer than 2 (smooth, not bursty).
- **SWRR long-run**: weights 3:1 over 8000 picks land within 3% of 6000:2000.
- **Weighted random**: weights 7:3 over 40000 picks land within bounds of
  70/30.
- Concurrency: 16 goroutine readers, `-race` clean.

## Ad-tech uses

- Distribute requests across **upstream SSP endpoints** or **bidder instances**
  proportional to capacity, without bursts.
- **Failover**: drop an unhealthy upstream via `Remove`, add a new one via `Add`.
- Pair with a health-check / breaker: removed entries get no traffic until
  re-added.

## Testing

96% statement coverage, `-race` clean. Distribution assertions use generous
sample sizes to avoid flakes.

```bash
go test -race -cover ./loadbalance/...
```
