# hotreload

An atomic-swap double buffer for hot-reloading config or data without blocking
readers. A `Loader` builds the live value (from a file, a remote endpoint, an
in-memory generator); a `Buffer` stores it behind an atomic pointer. `Get` is a
single atomic load and never blocks on a reload, even while a slow `Load` is
mid-flight. Pure standard library.

## Why

Config and data that are read on every request but refreshed periodically are
a classic reader/writer problem. A `sync.RWMutex` makes every reader pay for
the rare reload; reading a value mid-update risks a torn read. `hotreload`
keeps readers lock-free: they keep reading the previously swapped-in value
until the new one is fully built and atomically published, so neither a slow
`Load` nor a concurrent `Reload` can stall or corrupt a reader.

## API

| Symbol | Behavior |
|---|---|
| `Loader[T]` interface | `Load() (T, error)` seam; any file/remote/in-memory source |
| `New[T](loader) (*Buffer[T], error)` | Build with an initial `Load`; fails if the first load fails (`ErrLoadFailed`) |
| `(*Buffer).Get() T` | Lock-free atomic read; zero value of `T` before the first successful load |
| `(*Buffer).Reload() error` | Build a fresh value via the `Loader` and atomically publish; serialized, failed load keeps the prior value |
| `(*Buffer).Start(ctx, interval) (stop func())` | Spawn a goroutine that `Reload`s on a ticker until `stop()` or `ctx` cancel; `stop()` is idempotent and joins the goroutine |
| `ErrLoadFailed` | Returned by `New` when the initial load fails |

Concurrency: safe for concurrent use. `Get` is lock-free. `Reload` serializes
`Load` calls via a mutex so a slow or expensive `Load` is never invoked twice
in parallel. Reload failures inside `Start` are ignored: the last good value
remains live (fail-open for hot config).

## Example

```go
type countLoader struct{ n atomic.Int64 }

func (c *countLoader) Load() (int, error) { return int(c.n.Add(1)), nil }

b, err := hotreload.New[int](&countLoader{})
if err != nil {
    log.Fatal(err)
}
fmt.Println("initial:", b.Get()) // populated by New's first Load

if err := b.Reload(); err != nil {
    log.Fatal(err)
}
fmt.Println("reloaded:", b.Get()) // atomically swapped in

// Periodic background reload; stop() joins the goroutine so no leak.
stop := b.Start(ctx, 10*time.Second)
defer stop()
```

## Testing

Pure standard library; no external services. Initial-load failure, atomic
swap visibility, serialized `Reload`, `Start`/`stop` lifecycle and idempotency,
context cancellation, and goroutine-leak checks.

```bash
go test -race -cover ./hotreload/...
```
