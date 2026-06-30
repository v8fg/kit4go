# bloom

A classic Bloom filter: a space-efficient, probabilistic set-membership test
with one-sided error. Pure standard library (`math`, `hash/fnv`, `sync`).

## Why

A Bloom filter answers "was this element added?" using a few bits per element.
A **no** is certain (the element was never added); a **yes** is probable, with a
false-positive rate you choose at construction. That tradeoff is ideal for
dedup at scale, where storing full keys would blow memory and a rare false
positive is harmless (it just means treating a genuinely-new item as already
seen).

This implementation uses Kirsch-Mitzenmacher double hashing: two base hashes
derive all k indices, so there is no slice of hash functions and lookups stay
cache-friendly.

## API

```go
// Size for 1M elements at ~1% false positives (~1.2 MB).
f := bloom.New(1_000_000, 0.01)

f.AddString("user:42")
f.TestString("user:42")            // true (added)
f.TestString("user:999")           // false (certainly not added)

wasDup := f.TestAndAddString("auction:7") // pre-add result, then insert
```

| Method | Behavior |
|---|---|
| `New(expectedN, fp)` | Size from element count + target false-positive rate |
| `NewFromParams(m, k)` | Explicit bits (`m`) and hashes (`k`) |
| `Add(data)` / `AddString(s)` | Insert |
| `Test(data)` / `TestString(s)` | "Probably added" (true) vs "definitely not" (false) |
| `TestAndAdd(data)` | Return previous state, then insert (duplicate check) |
| `Merge(other)` | Union another compatible filter (same m, k) |
| `Reset()` | Clear all bits |
| `N()`, `M()`, `K()` | Items added / bit count / hash count |
| `EstimatedFalsePositiveRate(n)` | Current FPR estimate |

All methods are safe for concurrent use.

## Sizing

Given `n` elements and target rate `p`:

```
m = ceil(-n * ln(p) / (ln2)^2)   // bits  (~9.6 bits/element at 1%)
k = round((m/n) * ln2)           // hashes (~7 at 1%)
```

At 1% FPR, ~1.2 MB filters 1M elements. Memory scales with `m`, not with the
element size.

## Ad-tech uses

- **Per-user / per-auction dedup** — "have I already bid on / logged this?"
  across many instances, sharing the filter or merging.
- **Repeat-impression / bot suppression** — cheaply reject a user already seen
  in the window.
- **Pre-filter before a costly lookup** — a DB/cache hit is only attempted for
  items the filter says are present.

## Testing

97% statement coverage, `-race` clean. Verifies no false negatives over 1000
inserts, measures the observed false-positive rate against the 1% target (within
bounds), TestAndAdd semantics, merge (union + incompatible rejection), reset,
FPR estimate at the design point, panic guards, and a concurrent add/test churn
run.

```bash
go test -race -cover ./bloom/...
```
