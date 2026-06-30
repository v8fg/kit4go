# topk

Maintains the top-K most frequent items in a stream using a min-heap of size K.
O(log K) per touch, O(K) heap + O(N) count-map memory. Pure standard library.

## Why

When you need a real-time "leaderboard" of the most frequent items — top SSPs by
request volume, top creatives by impressions, top placements by spend — without
scanning the full count map on every query. A min-heap of size K keeps the
threshold updated incrementally: only items above the current minimum enter.

Distinct from `hotkey` (sliding-window detection) and `countmin` (approximate
per-key frequency): topk tracks exact cumulative counts and returns the sorted
top-K set.

## API

```go
tr := topk.New(10)           // track top 10
tr.Touch("ssp:rubicon")      // +1
tr.TouchN("ssp:rubicon", 5)  // +5 (total 6)
tr.TouchN("ssp:appnexus", 8)

top := tr.Top()  // []Entry{{Key:"ssp:appnexus", Count:8}, {Key:"ssp:rubicon", Count:6}, ...}
```

| Symbol | Behavior |
|---|---|
| `New(k)` | Build (panics if k ≤ 0) |
| `Touch(key)` / `TouchN(key, n)` | Increment + update top-K |
| `Top() []Entry` | Sorted by count desc (Entry{Key, Count}) |
| `Count(key) int64` | Exact count for a key |
| `Len()` / `K()` | Items in heap / configured K |
| `Reset()` | Clear all |

## Ad-tech uses

- **Top SSPs / creatives / placements** by volume — real-time leaderboard.
- **Heavy-hitter reporting** — exact top-K for dashboards/alerts.
- **Spend leaderboard** — TouchN with spend amounts.

## Testing

100% coverage, `-race` clean. Covers basic top-3 ordering, eviction (new item
displaces min), incremental updates (heap re-sifts), fill-then-exceed, count
lookup, reset, TouchN(0)/negative guards, and a 16-goroutine concurrent touch
run.

```bash
go test -race -cover ./topk/...
```
