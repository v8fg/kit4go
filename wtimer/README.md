# wtimer

A wall-clock timer wheel: schedule one-shot and recurring callbacks with
millisecond precision, backed by a min-heap for O(log N) add and O(1) cancel.
Pure standard library.

## Why

`time.AfterFunc` works for a handful of timers, but managing thousands of
cancellable timers (creative TTL invalidation, session timeouts, pacing
checkpoints) needs a centralized wheel: one goroutine, one heap, one place to
cancel/clean/inspect. Cancel is a single atomic flag (no mutex).

## API

```go
w := wtimer.New()
defer w.Close()

t, _ := w.Add(5*time.Second, func() { invalidateCreative(id) })
// later, if the creative is refreshed:
t.Cancel()

// Recurring:
heartbeat, _ := w.AddRecurring(30*time.Second, func() { sendHeartbeat() })
defer heartbeat.Cancel()
```

| Symbol | Behavior |
|---|---|
| `New()` | Start the wheel |
| `Add(delay, fn) (*Timer, error)` | One-shot after delay |
| `AddRecurring(interval, fn) (*Timer, error)` | Recurring every interval |
| `Timer.Cancel()` | Cancel (atomic, no mutex) |
| `Timer.Cancelled()` | Was it cancelled? |
| `Wheel.Len()` | Active (non-cancelled) timer count |
| `Wheel.Close()` | Stop + drain. Idempotent |

Cancel is O(1): sets an atomic flag; the wheel skips cancelled timers when they
reach the top of the heap and cleans them lazily.

## Testing

99% coverage, `-race` clean. Covers one-shot fire, recurring fire+cancel,
one-shot cancel, nil callback error, add-after-close, idempotent close, Len,
100-timer stress, in-order firing, and cancelled-timer cleanup.

```bash
go test -race -cover ./wtimer/...
```
