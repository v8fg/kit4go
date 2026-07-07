# hotkey

Detects heavy-hitter (hot) keys in a sliding time window — the keys receiving
disproportionate traffic in the last N seconds. Pure standard library.

## Why

In high-QPS systems a small set of keys (a viral creative, a specific SSP, a bot
farm) can dominate traffic and skew load. Detecting them lets you act: route to a
local cache, dispatch to a dedicated shard, or throttle individually. This
package tracks per-key hit counts in a sliding window and returns the top-K by
count on demand. Idle keys are pruned so memory tracks only active keys.

## API

```go
d := hotkey.New(10*time.Second, 10)  // top-10 keys in a 10s window
d.Touch("ssp:rubicon")               // record a hit
top := d.Top()                       // []HotKey{{Key:"ssp:rubicon", Count:...}, ...}
n    := d.Count("ssp:rubicon")       // current window count
```

| Symbol | Behavior |
|---|---|
| `New(window, topK, opts...)` | Build a Detector (panics if window/topK ≤ 0) |
| `Touch(key)` | Record a hit at the current time |
| `Top() []HotKey` | Top-K keys by count (desc), excludes zero-hit |
| `Count(key) int` | Hits for key in the current window |
| `Len() int` | Active (non-idle) key count |
| `Reset()` | Clear all |
| `WithMaxKeys(n)` | Cap tracked keys (prune idle, then fewest-hits). `0` (the Go zero value) disables the cap (unbounded); negative values are treated the same way. Default `DefaultMaxKeys` (10000) when omitted. Matches `freqcap`. |
| `WithClock(f)` | Inject a clock (tests) |

## Ad-tech uses

- **Hot SSP / creative detection** — identify the few endpoints or creatives
  generating disproportionate load; cache or shard them.
- **Bot / burst detection** — a sudden spike on one user_hash signals automation.
- **Load balancing signal** — feed Top() to a load balancer or sharder to spread
  hot keys.

## Testing

96% statement coverage, `-race` clean. Covers top-K ordering, K-limit, window
expiry, count, reset, idle-key pruning, maxKeys eviction, empty-top, and a
16-goroutine concurrent touch run.

```bash
go test -race -cover ./hotkey/...
```
