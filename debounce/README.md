# debounce

Debounce and throttle utilities for coalescing rapid function calls. Pure
standard library.

## API

### Debounce

```go
d := debounce.New(200*time.Millisecond, func() { reloadConfig() })
defer d.Close()
// Each call resets the timer; reloadConfig fires only after 200ms of quiet.
for _, change := range changes { d.Call() }
```

| Symbol | Behavior |
|---|---|
| `New(after, fn)` | Build (panics if fn nil) |
| `Call()` | Schedule/reschedule execution after `after` of quiet |
| `CallWith(arg)` | Same + stores arg (retrievable via LastArg) |
| `Flush()` | Execute immediately if pending |
| `Cancel()` | Discard pending call |
| `Pending()` | Is a call scheduled? |
| `Close()` | Stop (subsequent Call is no-op) |

### Throttle

```go
th := debounce.NewThrottle(time.Second, func() { flushMetrics() })
defer th.Close()
// At most one flush per second; intermediate calls return false.
for _, ev := range events { th.Call() }
```

| Symbol | Behavior |
|---|---|
| `NewThrottle(interval, fn)` | Build |
| `Call() bool` | Fire if interval elapsed (async); false if throttled |
| `CallBlocking() bool` | Same but synchronous |
| `Calls() int64` | Total successful calls |
| `Close()` | Stop |

## Ad-tech uses

- **Config reload debounce** — batch rapid flag changes into one reload.
- **Metrics flush throttle** — don't flush more than once per second.
- **Creative cache invalidation debounce** — coalesce rapid invalidation signals.

## Testing

97% coverage, `-race` clean. Covers debounce fire-after-quiet, cancel, flush,
CallWith/LastArg, Pending, Close-stops-calls, throttle first-call-fires,
drops-within-interval, fires-after-interval, blocking variant, call counter,
Close, and nil-fn panic guards.

```bash
go test -race -cover ./debounce/...
```
