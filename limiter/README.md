# limiter: in-process rate limiting

Local, in-process rate-limiting algorithms behind one `Limiter` interface with
`Allow` / `Wait` / `TryAcquire` / `Close` / `Metrics`. CAS-based,
allocation-free on the hot path. Pure standard library. For a distributed
(Redis-backed) limiter, see package `rate`.

## Algorithms

Select via `LimiterOptions.Algorithm`:

- `AlgorithmTokenBucket` (`"token_bucket"`) continuous refill with burst cap.
- `AlgorithmSlidingWindow` (`"sliding_window"`) rolling-window counter.
- `AlgorithmFixedWindow` (`"fixed_window"`) per-window counter.
- `AlgorithmLeakyBucket` (`"leaky_bucket"`) smoothing queue.
- `AlgorithmGCRA` (`"gcra"`) Generic Cell Rate Algorithm.

## Usage

- `NewLimiter(opts LimiterOptions) Limiter` returns the `Limiter` interface, or
  nil if the algorithm is unrecognised or `Rate <= 0`.
- `LimiterOptions{ Algorithm, Rate, Burst, Window }` — zero values fall back to
  defaults (`Rate` is required; `Algorithm ""` selects token-bucket).
- `(*Limiter).Allow() bool` non-blocking, one token.
- `(*Limiter).Wait(ctx) error` block until acquired or ctx done (returns ctx.Err()).
- `(*Limiter).TryAcquire(n int) bool` atomic batch; `n <= 0` is a no-op true.
- `(*Limiter).Close()` idempotent; subsequent calls are no-ops.
- `(*Limiter).Metrics() LimiterMetrics` `{ Allowed, Denied, Acquired }`.

## Example

```go
import (
    "context"
    "github.com/v8fg/kit4go/limiter"
)

l := limiter.NewLimiter(limiter.LimiterOptions{
    Algorithm: limiter.AlgorithmTokenBucket,
    Rate:      1000, // tokens/sec
    Burst:     100,
})
if l.Allow() { /* serve */ }
_ = l.Wait(ctx)
```
