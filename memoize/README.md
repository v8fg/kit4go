# memoize

Thread-safe generic memoization of pure functions. Pure standard library.

Memoization caches a function's results keyed by its argument, so repeated calls
with the same argument return the cached value without recomputing. It trades
memory for compute — best for expensive, referentially-transparent functions
(parsers, derivations, lookups over immutable data).

## Quick start

```go
import "github.com/v8fg/kit4go/memoize"

// Memoize a pure function.
slow := memoize.Memoize(func(n int) int {
	return expensiveCompute(n)
})
slow(42) // computes
slow(42) // cache hit — no recompute

// For functions that can fail, only successes are cached; errors retry.
get := memoize.MemoizeErr(func(key string) (Result, error) {
	return fetch(key) // transient errors are NOT cached → retried next call
})
```

## API

| Function | Description |
|----------|-------------|
| `Memoize[K,V](fn)` | Cache `fn(k)` by key; thread-safe (sync.Map) |
| `MemoizeErr[K,V](fn)` | Like Memoize, but caches only successes; errors retry |

## Notes

- **fn must be pure** — same input always yields the same output. An impure
  function (depends on time, random, or mutable state) will serve stale cached
  results.
- **Concurrent first-calls may compute twice.** Two goroutines hitting the same
  uncached key can both run fn before either stores the result. For a pure fn
  this is safe (identical result) but wasteful. For expensive fns where
  single-computation matters, wrap fn with `golang.org/x/sync/singleflight`
  before memoizing — that is deliberately out of scope here.
- **MemoizeErr does not cache errors.** A transient failure (network, timeout)
  returns to the caller uncached, so the next call retries rather than
  remembering the failure permanently.
- The cache is unbounded — it grows with the key space. For a bounded cache,
  wrap the result with eviction (e.g. the `lru` package) at the call site, or
  reset by reconstructing the memoized function.
- Not safe to mutate cached values in place — treat them as read-only.
