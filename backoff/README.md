# backoff

Exponential retry delays with optional jitter, an attempt cap, and context-aware
sleep. Pure standard library (`math`, `math/rand/v2`, `sync`, `time`).

## Why

Retries without jitter synchronize failing clients into a thundering herd.
Retries without a cap let one failure back off into minutes. Retries without an
attempt cap loop forever. This package gives all three knobs plus the
well-tested AWS jitter shapes, in a tiny, dependency-free API.

## API

```go
b := backoff.New(
    backoff.WithBase(100*time.Millisecond),
    backoff.WithFactor(2),
    backoff.WithMax(5*time.Second),
    backoff.WithJitter(backoff.JitterEqual),
    backoff.WithMaxAttempts(5),
)
for {
    err := doWork(ctx)
    if err == nil { break }
    if werr := b.Wait(ctx); werr != nil {
        break // max attempts or ctx cancelled
    }
}
```

| Method | Behavior |
|---|---|
| `New(opts...) *Backoff` | Build (defaults: base 100ms, factor 2, max 10s, JitterFull) |
| `Next() (time.Duration, bool)` | Next delay + advance; `false` when attempt cap reached |
| `Wait(ctx) error` | Sleep until next delay (ctx-aware); `ErrMaxAttempts` when capped |
| `Reset()` | Restart the sequence |
| `Attempt() int` | Calls since last Reset |

## Jitter modes

| Mode | Shape |
|---|---|
| `JitterNone` | pure exponential (sync-prone; tests only) |
| `JitterFull` (default) | uniform in `[0, exp]` |
| `JitterEqual` | `exp/2 + uniform[0, exp/2]` (centered) |
| `JitterDecorrelated` | next uniform in `[base, last*3]` (AWS shape) |

## Ad-tech uses

- Retry a transient **SSP / broker** failure without hammering it in lockstep.
- Back off after a **429 / rate-limit**, then resume.
- Re-establish a dropped connection with increasing patience.

## Testing

96% statement coverage, `-race` clean. Verifies exact exponential growth
(no-jitter), the cap saturation, the attempt cap + `ErrMaxAttempts`, Reset,
the bounds of each jitter shape (Full/Equal/Decorrelated), context cancellation
during Wait, and defaults.

```bash
go test -race -cover ./backoff/...
```
