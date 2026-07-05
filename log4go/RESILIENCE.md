# log4go Resilience

log4go sits on every caller's hot path. Its contract is **do no harm**: it must
not crash the host process, stall the business path, or silently lose records
because of its own bugs or a downstream (disk / kafka / network) failure. This
doc states the contract and how each guarantee is delivered, so operators know
what to monitor and what trade-offs they are accepting.

The contract is the kit's `L` dimension (hot-path infrastructure do-no-harm).
log4go is the reference implementation of it.

## The contract: L1 to L7

| # | Guarantee | How log4go delivers it | Where |
|---|---|---|---|
| L1 | No-panic hot path | A user value whose `MarshalJSON` panics, or a typed-nil error, is recovered and degrades to `null` — the log call never crashes the host. The recovery is the only sanctioned internal recover. | `field.go` `safeJSONMarshal` / `safeErrorString` |
| L2 | Non-blocking ingress | The caller's log call never blocks on a slow writer. `OverflowPolicy` resolves a full channel to drop or spill, never to stall. | `kafka_overflow.go`, `file_writer.go` |
| L3 | Bounded resources | Channels, ring spill, and file spill (`SpillMaxBytes`) are all capped. log4go cannot OOM the host or fill its disk. | `kafka_overflow.go`, `file_writer.go` |
| L4 | Downstream isolation | An inline circuit breaker around the kafka producer: when the broker-error rate is sustained, the daemon diverts records to the spill store instead of futile Sends; it half-opens to probe recovery and replays the backlog on close. | `kafka_breaker.go`, `kafka_writer.go` |
| L5 | Observable degradation | Every recovered panic and every dropped record is counted and surfaced. Silent failure is a bug. | `runtime_metrics.go`, per-writer `Metrics()` |
| L6 | Bounded shutdown | `Stop` waits on a writer daemon for at most `defaultShutdownTimeout` (5s), THEN closes the producer. For the kafka writer the producer Close runs a final Flush bounded by `kafka.Options.CloseFlushTimeout` (franz-go; default 30s; sarama drains its Successes/Errors channels). Total `Stop` is approximately daemon-timeout + CloseFlushTimeout, so lower `CloseFlushTimeout` when shutdown grace is tight. Neither a wedged daemon nor a dead broker can hang exit unbounded. | `daemon_panic.go` `waitQuit`, `kafka/franzgo_producer.go` `flushAndClose` |
| L7 | Business-data protection | Critical records (Panic / Fatal) flush before exit; the spill failover keeps records durable across a kafka outage. Bounded loss is visible. | `fatal.go` `Sync`, `kafka_writer.go` failover |

## Kafka circuit breaker and failover (L4)

A producer `Send` returns an error only for client-side failures; broker-level
delivery failures arrive asynchronously via the error event, which carries no
message. So per-record recovery of a broker-rejected record is infeasible
without offset tracking (rejected as over-engineering). Instead:

1. The daemon counts sends and errors in a rolling window.
2. When the error rate crosses `BreakerFailRate` over at least `BreakerMinSamples`
   sends within `BreakerWindow`, the breaker **opens**.
3. While open, the daemon **diverts** records to the spill store (under
   `OverflowSpill`) instead of calling `Send`. No records are lost; the caller is
   never blocked.
4. After `BreakerCooldown` the breaker **half-opens**: Sends resume to probe. A
   clean probe window **closes** it (and `drainSpill` replays the backlog); a bad
   one re-opens it.

Diversion and sync-send-error failover apply **only** under `OverflowSpill`.
`Drop` and `Block` keep their existing semantics; the breaker still reports its
state for all policies.

Defaults (conservative, all overridable):

| Option | Default | Meaning |
|---|---|---|
| `BreakerDisabled` | `false` (on) | Opt out entirely |
| `BreakerFailRate` | `0.5` | Trip when errors/sends >= 50% |
| `BreakerMinSamples` | `20` | Ignore windows smaller than this |
| `BreakerWindow` | `2s` | Rolling evaluation window |
| `BreakerCooldown` | `5s` | Open before probing recovery |

The hot path is two atomic adds; the state machine runs on the daemon's drain
ticker (one goroutine). No external dependency.

## Operational signals

Scrape these at monitoring cadence. Non-zero on any of the panic/drop counters
means log4go is degrading and you should investigate.

`RuntimeStats()` (package-level, call at scrape cadence, not per record):

- `MarshalPanics` — a logged value's marshal panicked and became `null`. A
  recurring non-zero value points at a buggy `MarshalJSON` in the caller.
- `DaemonPanics` — a writer daemon panicked and is now dead; its records stop
  flowing. Restart the process to recover.

`KafKaWriter.Metrics()` (per writer):

- `CircuitState` — `0` closed, `1` open, `2` half-open.
- `Failovered` — records diverted to spill on breaker-open or a sync send error.
- `Errored`, `Spilled`, `Dropped`, `SpillLen`, `Queued` — the existing counters.

`SetOnEvent` (real-time hook, non-blocking): fires on `sent`, `error`,
`failover`, and breaker state transitions for low-latency alerting.

## Trade-offs

- **At-least-once.** A record failovered to spill and later re-drained may be
  delivered twice if the original Send actually succeeded but reported an error.
  Enable `acks=all` with the idempotent producer to dedup.
- **Half-open probe loss window.** While half-open the daemon Sends to test
  recovery; a probe record whose delivery async-fails is not recoverable. This
  window is bounded to one `BreakerCooldown` (5s default). Under the open state
  (the actual outage) records go to spill and are recovered on close.
- **Bounded spill.** A prolonged outage fills the spill store; once full,
  records drop (counted in `Dropped`/`Failovered`). log4go does not attempt
  unbounded buffering, which would trade a kafka outage for an OOM.
- **Daemon death is visible, not auto-healed.** A panicked daemon is recorded
  and `Stop` returns within the deadline, but the writer stays dead until the
  process restarts. Auto-restart is a process-owner decision, not a library one.

## Recommended production config

```go
// Kafka, durable + outage-proof:
log4go.NewKafKaWriter(log4go.KafKaWriterOptions{
    Brokers:        brokers,
    ProducerTopic:  topic,
    Acks:           log4goAcksAll, // durability + idempotent dedup
    BufferSize:     4096,
    OverflowPolicy: log4go.OverflowPolicySpill,
    SpillType:      log4go.SpillTypeChain, // ring (hot) -> file (cold, persistent)
    SpillSize:      1 << 14,
    SpillDir:       "/var/log/app/spill",
    SpillMaxBytes:  1 << 30,
    // breaker defaults are fine; tune only if your error profile demands it.
})
```

For best-effort logs where throughput matters more than per-record durability,
`OverflowPolicy:"drop"` keeps the lowest latency and the breaker still reports
kafka health via `CircuitState`.
