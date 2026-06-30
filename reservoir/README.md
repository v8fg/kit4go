# reservoir

Uniform random sampling from a stream of unknown length (Algorithm R). Pure
standard library.

## Why

When you need a representative sample of a high-volume stream (bid requests,
impressions, clicks) for offline analysis, A/B testing, or quality auditing —
without buffering the full stream — reservoir sampling gives each item an equal
probability (k/n) of being in the final sample, in O(1) per item and O(k) memory.

## API

```go
s := reservoir.New[*BidRequest](1000)  // keep 1000 random samples
for req := range stream {
    s.Offer(req)
}
sample := s.Sample()  // []T copy, len <= k
```

| Symbol | Behavior |
|---|---|
| `New[T](k)` | Build (panics if k ≤ 0) |
| `NewWithOpts[T](k, WithSeed(...))` | With deterministic seed (tests) |
| `Offer(item)` | Present an item to the reservoir |
| `Sample() []T` | Copy of current contents (may be < k early on) |
| `Count() int` | Total items offered |
| `Cap() int` | Reservoir capacity k |
| `Reset()` | Clear (start a new sample) |

## Properties

- **Uniform**: each of n stream items has probability k/n of being in the final
  sample, regardless of stream order (Algorithm R).
- **O(1) per item, O(k) memory** — no dependency on stream length.
- **Deterministic with seed** — reproducible samples for testing/auditing.

## Ad-tech uses

- **Bid request sampling** — sample 1-in-N requests for latency/quality analysis.
- **Impression auditing** — random subset for viewability/fraud review.
- **A/B test bucketing** — reservoir as a uniform random subset.

## Testing

100% coverage, `-race` clean. Covers fill phase, cap enforcement, reset,
deterministic-with-seed reproducibility, uniform distribution (5000-run
frequency test, ±25%), concurrent offer, small stream, and Sample-is-copy.

```bash
go test -race -cover ./reservoir/...
```
