# syncutil

Common concurrent utilities: `OrDone` (ctx-cancelable channel receive), `Merge`
(fan-in), and `Promise` (one-shot future). All goroutines exit on ctx cancel — no
leak. Pure standard library.

## Quick start

```go
import (
    "context"
    "github.com/v8fg/kit4go/syncutil"
)

// OrDone — make range-over-channel ctx-cancelable
for v := range syncutil.OrDone(ctx, ch) { ... }

// Merge — fan-in N channels into one
for v := range syncutil.Merge(ctx, ch1, ch2, ch3) { ... }

// Promise — one-shot future
p := syncutil.NewPromise[int]()
go func() { p.Set(expensiveComputation()) }()
v, err := p.Get(ctx)
```

## API

| Function/Type | Description |
|---------------|-------------|
| `OrDone(ctx, src)` | Forward src until closed or ctx cancelled |
| `Merge(ctx, ch...)` | Fan-in N channels; closes when all inputs close |
| `Promise[T]` | One-shot future (Set/SetErr once, Get blocks) |
| `(*Promise).Done()` | Channel that closes on Set/SetErr |
