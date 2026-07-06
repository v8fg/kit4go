# objpool

A generic object pool: a `sync.Pool` wrapper with a reset hook and live stats.
`Get` returns a pooled object (calling the constructor on a miss) and applies
the reset hook if set; `Put` returns an object for reuse. Pure standard library.

## Why

Allocation churn dominates GC cost in hot serialization paths: `*bytes.Buffer`
for log/JSON rendering, protobuf message structs, scratch slices and maps. A
`sync.Pool` recycles these across goroutines, but the raw API is untyped, has
no reset hook, and gives no visibility into hit/miss. `objpool` adds generics,
an optional reset hook so every `Get` returns a clean value, and atomic
`Stats` for observability.

Distinct from `workerpool`, which is a goroutine pool (N workers draining a
job queue). `objpool` recycles objects, not goroutines.

## API

| Symbol | Behavior |
|---|---|
| `New[T](new func() T, opts...) *Pool[T]` | Build (panics if `new` is nil) |
| `WithReset[T](fn func(T))` | Hook called on every `Get` before returning; nil-safe |
| `(*Pool).Get() T` | Fetch, reset if hook set, update stats |
| `(*Pool).Put(x T)` | Return for reuse |
| `(*Pool).Stats() Stats` | Atomic snapshot: `Gets`, `Puts`, `Misses`, `InUse` |

`Misses` counts constructor invocations. It is best-effort: `sync.Pool` may
clear pooled objects on GC, so a steady-state workload can show ongoing misses
even when the pool is in use. `InUse = Gets - Puts`, clamped to 0 (under
concurrency the signed internal gauge can dip briefly negative before the
matching `Put` lands).

## Example

```go
pool := objpool.New(
    func() *bytes.Buffer { return new(bytes.Buffer) },
    objpool.WithReset(func(b *bytes.Buffer) { b.Reset() }),
)

b := pool.Get()
b.WriteString("hello")
pool.Put(b) // return the (now "dirty") buffer for reuse

b2 := pool.Get() // reset hook has cleared it
b2.WriteString("world")
fmt.Println(b2.String())

s := pool.Stats()
fmt.Printf("gets=%d puts=%d inUse=%d\n", s.Gets, s.Puts, s.InUse)

// output:
// world
// gets=2 puts=1 inUse=1
```

## Testing

Pure standard library; no external services. Reset-hook behavior, stat
accounting under concurrency, miss counting, and the `InUse` clamp on the
negative internal gauge.

```bash
go test -race -cover ./objpool/...
```
