# signalbus

A string-keyed synchronous pub/sub event bus. `Connect` subscribes a handler
to a named signal; `Send` dispatches an argument list to every handler
registered for that name. Pure standard library.

## Dispatch model

`Send` runs synchronously on the caller's goroutine: each registered handler is
invoked in registration order, one after another, in the same call stack that
invoked `Send`. There are no goroutines and no channels: a `Send` returns only
after every handler has returned. Handlers MUST be non-blocking (no network
waits, no long locks); a slow handler stalls the caller and every handler after
it.

A panicking handler is recovered so it cannot abort the dispatch of the
remaining handlers. Recovered panics are counted (`Recovered`) and optionally
surfaced via a hook (`SetPanicHook`), but never re-panicked, so one buggy
handler cannot take down the process.

The producer (`Send`) and the consumers (`Connect`) share only the signal name
and the argument contract, never types. This lets a low-level package emit a
signal that higher-level packages subscribe to, without the low-level package
importing them, breaking what would otherwise be a circular import.

## API

| Symbol | Behavior |
|---|---|
| `New() *Bus` | Empty bus (zero value is not usable) |
| `(*Bus).Connect(name, h) (disconnect func())` | Subscribe; returned func removes exactly that one entry, idempotent |
| `(*Bus).Send(name, args...)` | Dispatch to handlers in registration order, on the caller's goroutine |
| `(*Bus).Disconnect(name)` | Remove ALL handlers for `name` |
| `(*Bus).Len(name) int` | Handler count for tests/debug; do not gate dispatch on it |
| `(*Bus).SetPanicHook(fn)` | Fired on a recovered handler panic; runs on the `Send` goroutine, must be non-blocking |
| `(*Bus).Recovered() uint64` | Total recovered panics across all signals |

Concurrency: safe for concurrent use. `Send` snapshots the handler slice under
the lock and dispatches outside it, so a handler may re-entrantly
`Connect`/`Disconnect`/`Send` on the same bus without deadlocking. Handlers
added or removed during a dispatch are not visible to the in-flight `Send`.

## Example

```go
bus := signalbus.New()

// Connect in order; Send invokes in this same order.
bus.Connect("user.signed_up", func(args ...any) {
    fmt.Println("metrics:", args[0])
})
bus.Connect("user.signed_up", func(args ...any) {
    fmt.Println("welcome-email:", args[0])
})

// Synchronous dispatch on this goroutine: both prints happen before Send
// returns.
bus.Send("user.signed_up", "alice@example.com")

// output:
// metrics: alice@example.com
// welcome-email: alice@example.com
```

## Testing

Pure standard library; no external services. Registration-order dispatch,
precise single-handler disconnect (idempotent), `Disconnect` all, panic
recovery + counter + hook, re-entrant `Send`/`Connect` from inside a handler,
and `Len` accounting.

```bash
go test -race -cover ./signalbus/...
```
