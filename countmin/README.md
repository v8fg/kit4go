# countmin

A Count-Min Sketch: fixed-space frequency estimation for a stream. Estimates are
always **over**-approximations (never under-count). Pure standard library.

## Why

Counting per-key frequency for a high-volume stream (per-SSP request volume, per-
creative impressions) needs a map entry per key — expensive at scale and you often
only care about the heavy hitters. A Count-Min Sketch uses `d × w` counters and
estimates any key's count in O(d); the min across d hash rows cancels most
collisions, so a frequent element's estimate stays close to its true count. Pair
with `freqcap` when you need exact per-entity caps, and with `hyperloglog` for
distinct counts.

## API

```go
c := countmin.New(2048, 5)          // 5 rows × 2048 counters
c.AddString("ssp-a")                 // +1
c.Add([]byte("ssp-a"), 3)            // +3
c.EstimateString("ssp-a")            // >= true count
share := float64(c.EstimateString("ssp-a")) / float64(c.Total())
```

| Symbol | Behavior |
|---|---|
| `New(width, depth)` | Build (0 → defaults 2048 × 5) |
| `NewForError(eps, delta)` | Size from error bound (`w=ceil(e/eps)`, `d=ceil(ln(1/delta))`) |
| `Add(data, count)` / `AddString(s)` | Increment by `count` (1 for a single event) |
| `Estimate(data)` / `EstimateString(s)` | Approximate frequency (≥ true count) |
| `Total()` | Sum of all Add counts (stream length; for heavy-hitter share) |
| `Merge(other)` | Add another sketch's counts (sum) — same shape |
| `Reset()` | Zero all counters |

## Concurrency

`Add` is not internally locked (hot path). For concurrent producers, use
per-shard sketches and `Merge` them — Merge sums element-wise, order-independent.

## Accuracy

- Estimate ≥ true count, always (never under).
- Over-count bounded: `Pr(over > eps*N) < delta` when sized via `NewForError`.
- Heavy hitters (count ≫ N/w) stay within a few percent; rare keys may over-count
  by the collision noise.

## Ad-tech uses

- **Heavy-hitter detection** — top SSPs / creatives / placements by volume.
- **Traffic share** — a key's `Estimate / Total`.
- **Approximate frequency** where per-key storage is too costly (with freqcap for
  exact caps).

## Testing

95% statement coverage, `-race` clean. Covers never-under-count, single-add,
heavy-hitter isolation (10000 among 1000 rare, over-count < 10%), Total, Reset,
Merge (+ incompatible-shape error), NewForError, defaults, determinism, and a
sharded-merge concurrency run.

```bash
go test -race -cover ./countmin/...
```
