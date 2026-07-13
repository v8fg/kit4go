# set

Generic, type-safe set for Go — backed by a map, pure standard library.

Go's standard library has no `Set` type. This package fills that gap with a
generic `Set[T comparable]` supporting the standard set algebra (union,
intersection, difference, symmetric difference) and predicates (subset, superset,
disjoint, equal).

## Quick start

```go
import "github.com/v8fg/kit4go/set"

tags := set.New("holiday", "sale", "clearance")
tags.Contains("sale")           // true
tags.ContainsAll("sale", "x")   // false

// Set algebra
allowed := set.New("holiday", "food")
both := set.Intersect(tags, allowed)         // {"holiday"}
merged := set.Union(tags, allowed)           // {"holiday", "sale", "clearance", "food"}
removed := set.Difference(tags, allowed)     // {"sale", "clearance"} — tags without allowed

// Predicates
set.IsSubset(set.New(1, 2), set.New(1, 2, 3))  // true
set.IsDisjoint(set.New(1), set.New(2))           // true
set.Equal(set.New(1, 2), set.New(2, 1))          // true (order-independent)
```

## API

| Method | Description |
|--------|-------------|
| `New[T](vals...)` | Build from values (duplicates collapsed) |
| `From[T](slice)` | Build from a slice |
| `Add(vals...)` | Insert values (idempotent) |
| `Remove(vals...)` | Delete values (absent is no-op) |
| `Contains(v)` | Membership test |
| `ContainsAll(vals...)` | Every value present? |
| `ContainsAny(vals...)` | At least one present? |
| `Len()` / `IsEmpty()` | Size queries |
| `Clear()` | Remove all |
| `Pop()` | Remove + return an arbitrary element |
| `Each(fn)` | Iterate (unordered) |
| `Filter(fn)` | New set of elements passing fn |
| `ToSlice()` / `Clone()` | Materialize |
| `Union(sets...)` | ∪ — all elements from all sets |
| `Intersect(a, b)` | ∩ — elements in both |
| `Difference(a, b)` | ∖ — a without b |
| `SymmetricDifference(a, b)` | Δ — in a or b, not both |
| `IsSubset(sub, sup)` / `IsSuperset(sup, sub)` | ⊆ / ⊇ |
| `IsDisjoint(a, b)` | No common elements |
| `Equal(a, b)` | Same elements |

## Concurrency

Like a Go map, a `Set` is **not safe for concurrent use**. For concurrent
access, protect it with a `sync.RWMutex` (RLock for read-only methods, Lock for
mutators) or shard by key.
