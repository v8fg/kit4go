# freqcap

A per-entity sliding-window event counter for frequency capping: "may this key
produce one more event within the window, given it has already produced N?".
Pure standard library.

## Why

It is the per-entity *counting* sibling of `limiter`. `limiter` throttles an
action's global rate to protect the caller; `freqcap` caps how many times an
*entity* (user, creative, device) may act per window to protect the audience.
The data is exact (a timestamp per allowed event), and idle keys are pruned so
memory tracks active entities, not historical ones.

## API

```go
// "show this creative to this user at most 3 times per hour"
cap := freqcap.New(time.Hour, 3)
if cap.Allow(userID + "|" + creativeID) {
    serveCreative()   // recorded: counts toward the 3/hour cap
} else {
    skip()            // already at cap this hour
}
```

| Method | Behavior |
|---|---|
| `New(window, maxEvents, opts...)` | Allow at most `maxEvents` per key in `window` |
| `Allow(key) bool` | Record an event if under cap; `true` when recorded |
| `Count(key) int` | Events currently in the window (read-only, lazy-trimmed) |
| `Reset(key)` | Drop all events for a key |
| `Len() int` | Tracked keys (active + not-yet-pruned idle) |

| Option | Default | Effect |
|---|---|---|
| `WithMaxKeys(n)` | 0 (unbounded) | Cap tracked keys; prunes idle, then oldest-start |
| `WithClock(f)` | `time.Now` | Inject a clock (tests) |

## Ad-tech / push uses

- **Creative frequency capping** — show an ad to a user at most N times/hour.
- **Notification caps** — message a user at most K times/day.
- **Bot / repeat suppression** — cap events per device per minute.
- **Win/conv rate guards** — limit actions per entity per window.

## Memory

Each allowed event stores one `time.Time` (8 bytes) per key. For low-cap keys
(the typical N=3/hour user cap) this is trivial; for high-volume keys (thousands
per window) prefer a bucketed limiter. Idle keys are reclaimed automatically on
each `Allow`.

## Testing

95% statement coverage, `-race` clean. Covers under-cap/over-cap, independent
keys, window expiry (injected clock), Count/Reset, idle-key pruning, MaxKeys
eviction, panic guards, and a 32-goroutine concurrent stress run that asserts the
cap is never exceeded.

```bash
go test -race -cover ./freqcap/...
```
