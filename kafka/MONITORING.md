# kafka — Monitoring, Tuning & Defaults

This document records the **upstream library defaults** (sarama, franz-go), the
**kit4go-chosen defaults** that override them, and the **monitoring** surface
(`Snapshot`, `History`, `Metrics`). It is the human-readable companion to the
godoc on `Options`, `ProducerSnapshot`, and the constants in `options.go`.

## Why kit4go overrides the upstream defaults

Both backends are selected at build time (default sarama; `-tags franzgo` for
franz-go) behind identical interfaces — the **seamless-swap (无感切换)** goal. But
the two libraries ship with **different native defaults**, so "do nothing" is not
"identical behavior":

| Param | sarama v1.50.3 native | franz-go v1.21.4 native | "do nothing" divergence |
|---|---|---|---|
| linger / flush time | `Flush.Frequency=0` (**off**) | `linger=10ms` (**on**) | franz-go silently adds 10ms latency |
| buffered records | `Flush.Messages=0` (soft trigger) | `maxBufferedRecords=10000` (hard cap) | 10× memory difference |
| buffered bytes | `Flush.Bytes=0` | `maxBufferedBytes=0` (unlimited) | — |
| max batch/request | `MaxMessageBytes=1MiB` | `maxRecordBatchBytes=1000012` (~1MiB) | aligned |
| input channel | `ChannelBufferSize=256` | (kgo has none) | — |
| retries | `Retry.Max=3` | `recordRetries=MaxInt64` | kit4go pins to 5 |

kit4go **always sets linger + MaxBufferedRecords explicitly** on both backends,
so the native defaults never leak in — behavior is pinned to the kit4go resolved
`Options` and matches what `Snapshot()` reports.

## kit4go defaults (`options.go`)

| Option | kit4go default | Constant |
|---|---|---|
| `ProducerLinger` | **10ms** | `DefaultProducerLinger` |
| `MaxBufferedRecords` | **1000** | `DefaultMaxBufferedRecords` |
| `ChannelBufferSize` | **= MaxBufferedRecords (1000)** | (derived in `withDefaults`) |

- **`ProducerLinger=10ms`**: matches franz-go's native default (a proven batching
  sweet-spot — part of why franz-go measures ~2.9× faster on raw Produce),
  unifies both backends, and trades ~10ms latency for larger batches / fewer
  RPCs / higher throughput. Disable batching with `WithProducerLinger(LingerOff)`.
- **`MaxBufferedRecords=1000`**: more conservative than franz-go's native 10000
  (1000 × ~200B ≈ 200 KB/instance) to bound memory.
- **`ChannelBufferSize` tracks `MaxBufferedRecords`** so sarama's input-channel
  backpressure point stays aligned with the in-flight cap (see below).

> **Sync producers** (`NewSyncProducer`) force linger/MaxBufferedRecords **off**
> (see *Sync vs Async* below) — these async defaults apply only to `NewProducer`.

## The MaxBufferedRecords soft/hard contract (important)

`MaxBufferedRecords` has the **same name and default value** on both backends, but
**different native semantics** — an honest contract, not a smoothed-over one:

- **franz-go**: `MaxBufferedRecords` is a **hard cap** — `Send`/`SendBatch`
  **blocks** (backpressure) when ≥N records are in-flight unacked.
- **sarama**: `Flush.Messages` is a **soft flush trigger** — flushes when N
  messages accumulate, but does **not** block `Send`. sarama's real hard
  backpressure point is the **Input channel** (`ChannelBufferSize`).

To narrow this gap, kit4go defaults `ChannelBufferSize = MaxBufferedRecords`
(1000), so sarama's channel-based backpressure kicks in at ~the same point
franz-go's hard cap does. `ChannelBufferSize` is the lever to tune sarama's
buffering headroom independently.

**Monitoring is uniform** regardless: `InFlight` (= `Enqueued - Success - Failed`)
and `BufferedBytes` (= `BytesEnqueued - Bytes - BytesFailed`) use the **same
formula** on both backends → consistent **visibility**. Under broker slowness the
*sarama* value may differ from franz-go's (soft vs hard) — monitoring gives
visibility, not an isomorphic guarantee. Neither backend can grow in-flight
without bound (sarama: channel + batch; franz-go: hard cap).

### Size & refresh (sarama)

- **Size**: peak sarama in-flight ≈ `ChannelBufferSize` (input queue) + current
  batch (≤ `MaxBufferedRecords`) ≈ **2 × MaxBufferedRecords**. Bounded; halve the
  peak by lowering either. The channel itself holds only pointers (~8 KB at 1000).
- **Refresh**: flush timing is governed by `Flush.Frequency` (10ms) /
  `Flush.Messages` (1000) / `Flush.Bytes` **on the batch** and is **independent**
  of `ChannelBufferSize`. The channel adds no latency — it is burst headroom +
  the backpressure point. No stall: the dispatch goroutine drains it continuously;
  `Send` blocks only when the broker can't keep up (intended memory bounding).

## Sync vs Async — which options apply

`NewProducer` and `NewSyncProducer` take the **same `Options`** (compatibility).
The difference is which knobs take effect:

| Option | Async | Sync |
|---|---|---|
| `ProducerLinger` (10ms) | ✅ real batching | 🔒 forced off (per-send blocking) |
| `MaxBufferedRecords` (1000) | ✅ backpressure/trigger | 🔒 forced off (no async buffer) |
| `BatchMaxBytes` | ✅ batch byte cap | ⚠️ request byte cap only |
| `ChannelBufferSize` | ✅ sarama input channel | ✅ sarama (franz-go N/A) |
| `RetryMax` / `ProducerTimeout` | ✅ | ✅ |
| `Metrics` fields | full (InFlight/BufferedBytes/BatchCount/BatchMax…) | Enqueued/Success/Failed/Bytes |
| `Snapshot()` | ✅ (interface) + optional `History()` | ✅ method (no buffer to monitor) |

**Why sync forces batch knobs off**: sarama's `SyncProducer` wraps an
`AsyncProducer` — a non-zero `Flush.Frequency` would stall each `SendMessage` up
to the linger window, diverging from franz-go's immediate `ProduceSync`. Forcing
Flush off makes sync per-send on both backends → consistent (see
`buildSaramaConfig(o, sync)` / `kgoSyncProducerOpts`).

## Monitoring API

### `Snapshot()` — latest, lock-free, scrape-driven

```go
snap := producer.Snapshot()
// snap.Timestamp    time.Time (UTC; JSON → RFC3339 "...Z")
// snap.Name/Backend instance + backend id
// snap.ProducerMetrics  Enqueued/Success/Failed/Bytes/BytesFailed/BytesEnqueued/
//                        BatchCount/BatchMax/InFlight/BufferedBytes
// snap.Linger/MaxBufferedRecs/BatchMaxBytesCfg  effective config
```

- **Lock-free hot path**: reads atomic counters; never contends with `Send`.
  Records into the history ring under a scrape-path mutex that `Send` never
  touches.
- **Scrape-driven**: each `Snapshot()` call is one timestamped sample (Prometheus
  model). No background goroutine.

### `History()` — bounded trend samples (opt-in)

Enable with `WithSnapshotHistory(n)`; obtain via the optional interface:

```go
if h, ok := producer.(kafka.SnapshotHistory); ok {
    samples := h.History() // oldest→newest; nil if disabled/empty
    if len(samples) >= 2 {
        prev, cur := samples[len(samples)-2], samples[len(samples)-1]
        rps := kafka.SnapshotRate(prev, cur,
            func(s kafka.ProducerSnapshot) uint64 { return s.Success })
        _ = rps // records-acked/sec over the last scrape window
    }
}
```

- Async producers always satisfy `SnapshotHistory` (`History()` returns nil when
  disabled — check `len`, not the type assertion, for enablement).
- Bounded by the cap (~160 B/sample). Recommended 60–288 (1–5 min at 1 s scrape).
- `SnapshotRate(prev, cur, metric)` computes per-second rate of any counter, with
  a uint64 underflow guard (counter reset → 0).

## Prometheus scrape guidance

- Scrape `Snapshot()` at your `scrape_interval` (e.g. 15 s). Each scrape is one
  history sample; size `WithSnapshotHistory` to `retention / scrape_interval`.
- Expose counters (`Enqueued`/`Success`/`Failed`/`Bytes`) as Prometheus
  `counter` (monotonic); `InFlight`/`BufferedBytes` as `gauge`.
- Use `rate(success_total[1m])` in Prometheus, or `SnapshotRate` locally for a
  process-internal rate without an external TSDB.

## Stress matrix — max QPS & memory (real broker, both backends)

Full matrix in [`STRESS_MATRIX.md`](STRESS_MATRIX.md) (EN) / [`STRESS_MATRIX.zh.md`](STRESS_MATRIX.zh.md)
(中文). apache/kafka 3.8.0 KRaft, single node, M5. Headline numbers (linger 10ms, batch 1024):

| | sarama | franz-go |
|---|---|---|
| sync per-record | ~3.2K rec/s | ~2.7K rec/s |
| sync SendBatch(10000) | 506K rec/s | 1.15M rec/s |
| async peak rec/s (100B) | 720K | 847K |
| async peak MB/s (10KB+) | ~250 MB/s | ~1.3 GB/s |
| async buffer mem (100KB) | 197 MB | 97 MB (about half) |

sync is latency-bound (~3K per-record); use `SendBatch` for QPS. async is byte-bound (MB/s
ceiling), batch-insensitive for sarama; franz-go needs batch ≥1024 (log4go `BatchSize`
defaults to 1024). Memory (`BufferedBytes`) tracks payload × `MaxBufferedRecords`, not batch
size; franz-go holds about half sarama's buffer. franz-go `Close()` flushes before closing
(`flushAndClose`), so offsets land fully.

Single-node numbers are 1 broker / loopback / RF=1, a best-case ceiling. Multi-node RF=3
cuts byte-bound throughput to about a third (each record written to three brokers). Under
replication, acks is the dominant knob: both backends default to `acks=leader`
(unified; franz-go's idempotent producer is auto-disabled since it requires all;
set `AcksAll` to enable it). `acks` is configurable
(`Options.Acks` / `WithAcks`: `AcksLeader`/`AcksAll`/`AcksNone`). At 100B under RF=3, leader is
about 7× faster than all. Separate-host clusters scale ~N× at acks=leader; loopback can't
(shared host). The tuning params (1024/10ms/1000) are node-count-independent; only the
throughput ceiling scales with nodes.
