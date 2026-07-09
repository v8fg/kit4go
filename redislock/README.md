# redislock

A correct, minimal distributed lock on Redis (go-redis/v9). Zero third-party
deps beyond go-redis.

## Why

Distributed locking is one of the most common Redis patterns and one of the
easiest to get subtly wrong. This package does the two things that matter and
nothing else: atomic acquire (`SET NX PX` with an owner token) and atomic
release/refresh (Lua check-and-act, so a holder only ever touches its own lock).

## Usage

```go
locker := redislock.New(redisCmdable)

// Non-blocking: one attempt.
lock, err := locker.TryLock(ctx, "budget:camp42")
if errors.Is(err, redislock.ErrLockNotAcquired) {
    // someone else holds it
}

// Blocking with retries until acquired, ctx done, or wait timeout.
lock, err = locker.Lock(ctx, "budget:camp42",
    redislock.WithWaitTimeout(2*time.Second))
defer lock.Release(ctx)

// critical section
```

## Options

| Option | Default | Effect |
|---|---|---|
| `WithTTL(d)` | 10s | Lock time-to-live |
| `WithRetryInterval(d)` | 50ms | Delay between attempts in `Lock` |
| `WithWaitTimeout(d)` | 0 (until ctx) | Max total wait in `Lock` |
| `WithToken(s)` | random hex | Stable owner identity |
| `WithAutoRenew(true)` | off | Heartbeat goroutine extends TTL |
| `WithRenewInterval(d)` | TTL/2 | Auto-renew period |
| `WithOnLost(fn)` | none | Callback when an auto-renew fails |

## Correctness

- **Acquire** is `SET key token NX PX ttl` — atomic, single round-trip.
- **Release / Refresh** run a Lua script that checks the token first: a holder
  never deletes or extends a lock that has expired and been re-acquired by
  someone else.
- **Auto-renew** runs a goroutine that extends the TTL every `renewInterval`
  until `Release`. If a renewal fails — including transient network errors
  (fail-closed to prevent split-brain) — `Lock.Lost()` closes and `OnLost`
  fires. **If you enable auto-renew, you MUST consume `Lost()` or set `OnLost`;
  an unconsumed loss means your critical section continues without the lock.**

## Single-instance note

This is a single-Redis-node lock. For SPoF intolerance, run Redis behind
Sentinel or Cluster (so the client routes to the current master); the lock
semantics are unchanged. True multi-node Redlock is rarely worth the complexity
— a well-run Redis failover covers nearly all real ad-tech needs.

## Ad-tech uses

- **Single-flight budget / pacing updates** — serialize spend writes per
  campaign so concurrent bid responses don't double-spend.
- **Leader election** — one bidder instance owns a given SSP's traffic for a
  window.
- **Auction dedup** — guard against processing the same auction ID twice across
  instances.

## Testing

92% statement coverage, `-race` clean, against an in-process miniredis. Covers
acquire/release, contention, blocking acquire + wait timeout + ctx cancel,
owner-only release (foreign-token safety), refresh, auto-renew preventing
expiry, auto-renew reporting loss on forced removal, double-release safety, and
explicit tokens.

```bash
go test -race -cover ./...
```
