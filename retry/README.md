# retry

A composable retry helper: call a function with configurable backoff, max
attempts, and error classification (retryable vs permanent). Pure standard library.

## Why

Every external call (SSP, broker, DB) has transient failures that warrant a
retry — but retrying a permanent error (400, auth, schema mismatch) is wasted
effort. This package separates the retry strategy (how many, how long between)
from the error classification (should I retry this?), so the caller controls both.

## API

```go
r := retry.Do[string](ctx, func(ctx context.Context) (string, error) {
    return callSSP(ctx)
}, retry.WithMaxAttempts(5),
   retry.WithBackoff(retry.ExponentialBackoff(100*time.Millisecond, 5*time.Second)),
   retry.WithRetryable(func(err error) bool {
       return !errors.Is(err, ErrBadRequest) // retry transient, abort 4xx
   }))
if r.Err == nil {
    use(r.Value)
}
```

| Symbol | Behavior |
|---|---|
| `Do[T](ctx, fn, opts...)` | Call fn with retry; returns Result{Value, Err, Tries} |
| `WithMaxAttempts(n)` | Total attempts incl. first (default 3; 0 = unlimited) |
| `WithBackoff(b)` | Delay strategy (default: immediate) |
| `WithRetryable(fn)` | Error classifier (nil = retry all) |
| `ConstantBackoff(d)` | Fixed delay |
| `ExponentialBackoff(base, max)` | base × 2^(n-1), capped at max |
| `NoBackoff()` | Retry immediately |

## Properties

- **Error classification**: `IsRetryable(err)` returning false aborts immediately
  — permanent errors don't waste a retry slot.
- **Context-aware**: respects ctx cancellation between attempts; the in-flight fn
  call receives ctx too.
- **Backoff strategies**: constant, exponential (with cap), or none.
- **Result**: returns the last error + number of tries (for metrics/logging).

## Ad-tech uses

- Retry transient SSP/broker/DB failures with exponential backoff.
- Abort immediately on permanent errors (bad request, auth failure).
- Pair with kit4go/breaker for circuit-level protection and kit4go/backoff for
  jitter-aware backoff.

## Testing

95% coverage, `-race` clean. Covers success-first-try, retry-until-success,
max-attempts-exhausted, permanent-error-abort, retryable-classification, backoff
delay timing, exponential capping, context cancellation, unlimited attempts,
default config, no-backoff, and nil-classifier-retries-all.

```bash
go test -race -cover ./retry/...
```
