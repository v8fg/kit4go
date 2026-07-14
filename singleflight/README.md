# singleflight

Deduplicates concurrent in-flight function calls. Pure standard library.

When multiple goroutines call `Do(key, fn)` for the same key while a call is
already running, only the first executes `fn`; the rest wait and share its result.
Results are **not** cached — once the in-flight call completes, the next `Do` runs
`fn` again. Singleflight only coalesces calls that overlap in time.

This is the classic cure for the **thundering herd**: N requests for the same key
race to compute it (a cache miss, a cold DB row, an image render); without
singleflight all N run the expensive work; with it, only one runs and the rest
share.

## singleflight vs memoize

| | singleflight | memoize |
|---|---|---|
| Coalesces concurrent calls | yes | yes (with possible double-compute) |
| Caches after completion | **no** | yes |
| Bounded memory | yes (entry freed when the call ends) | unbounded (grows with key space) |

Use **singleflight** to suppress burst duplicates without retaining results. Use
**memoize** to permanently cache a pure function. They compose: `singleflight.Do`
can wrap a `memoize`d lookup to add burst protection.

## Quick start

```go
import "github.com/v8fg/kit4go/singleflight"

g := singleflight.New[string, *Result]()

// 50 goroutines hit "user:42" at once; the DB query runs only once.
r := g.Do("user:42", func() (*Result, error) {
	return db.LoadUser(42)
})
// r.Value, r.Err, r.Shared
```

## API

| Method | Description |
|--------|-------------|
| `New[K,V]()` | Empty group |
| `Do(key, fn)` | Run `fn` once for `key`, dedup concurrent callers; returns `Result{Value,Err,Shared}` |

`Result.Shared` is `true` when this caller received another in-flight caller's
result (it did not run `fn`).

## Notes

- **No caching.** After the in-flight call completes its entry is deleted; the
  next `Do` re-runs `fn`. Pair with `memoize` if you want the result retained.
- **Errors are shared, not cached.** If `fn` errors, every concurrent caller for
  that key receives the same error. The next `Do` runs `fn` again.
- **Blocking.** `Do` blocks until `fn` completes (or the shared call completes).
  Callers that need cancellation can run `Do` in a goroutine and `select` on
  their context.
- Thread-safe. Construct with `New`; the zero value is not ready to use.
