# log4go Performance & Architecture

> High-performance, memory-safe, observable Go logging library. Architecture,
> measured writer throughput/memory, bottlenecks & optimizations, and production
> (incl. ad-tech 100K / 10M QPS) configuration. Chinese version: [PERFORMANCE.md](PERFORMANCE.md).

## 1. Architecture

```
 caller goroutine(s)                 single bootstrap goroutine
 ┌──────────────────┐                ┌──────────────────────────────────┐
 │ Debug/Info/...() │   deliver      │  for rec := range records {       │
 │  format + Caller ├────records────>│    for _, w := range writers {    │
 │  (level filter)  │     chan(4096)  │       w.Write(rec)  ← serial     │
 │  atomic counter  │                │    }                             │
 └──────────────────┘                │  } + flush/rotate timers         │
                                     └──────────────────────────────────┘
```

- **Caller does light work**: level filter, time format, `runtime.Caller`, push to bounded `records` chan (4096). Measured `deliver` ≈ **1080 ns/op ≈ 923K QPS/core**.
- **Single bootstrap goroutine, serial** per record: end-to-end QPS ≈ 1/Σ(writer Write time). A slow writer drags all.
- **OOM guard**: bounded `records` chan; KafKaWriter has its own bounded chan + multi-policy overflow framework. **Never a per-record goroutine** (the old KafKaWriter OOM root cause — fixed).

## 2. Writer throughput & memory (single core, Go 1.26)

| Writer / path | ns/op | ~QPS/core | B/op | allocs | note |
|---|---|---|---|---|---|
| `deliver` pipeline (discard) | 1084 | 923K | 395 | 8 | caller-side upper bound |
| `Logger.Filtered` (level-dropped) | 12 | 83M | 7 | 0 | near-free |
| ConsoleWriter (pipe→discard) | 1705 | 586K | 160 | 6 | real terminal 1-2 orders slower |
| FileWriter (bufio 8192) | 339 | **2.95M** | 144 | 5 | buffered, timed flush |
| KafKaWriter.buildPayload (no fields) | 288 | 3.5M | 288 | 2 | typed append, zero reflection |
| KafKaWriter.buildPayload (5 base fields) | 1014 | 1.0M | 800 | 3 | Kafka→ES typical (typed, allocs −63%) |
| RingSpiller.Push | 10 | 100M | 0 | 0 | in-memory ring |
| FileSpiller.Push | 424 | 2.4M | 148 | 4 | disk spill |

> End-to-end = caller deliver + serial writers. File + Kafka: bootstrap ≈ 339(File) + ~100(Kafka enqueue) ≈ 440ns → ~2.2M QPS/core.

## 3. Bottlenecks & fixes

| Bottleneck | Impact | Fix |
|---|---|---|
| ConsoleWriter sync stdout | blocks bootstrap | disable in prod (debug only) |
| bootstrap serial | more/slower writers → lower | keep only File + Kafka |
| records chan full | caller blocks (backpressure) | KafKaWriter drop/spill |
| per-record goroutine (old) | goroutine pile-up → OOM | **fixed** (zero per-record goroutine) |

## 4. Tuning

| Param | Default | Range | Effect |
|---|---|---|---|
| `recordChannelSize` | 4096 | 4096–65536 | records chan capacity |
| KafKaWriter `BufferSize` | 1024 | 8192–65536 | bounded send chan |
| `OverflowPolicy` | drop | drop/spill/block | full policy |
| `SpillType` / `SpillSize` / `SpillMaxBytes` | ring/1024/64MB | ring/file | spill store |
| `flushTimer` | 500ms | 200ms–1s | bufio flush interval |

## 5. Production setup

```go
fw := log4go.NewFileWriterWithOptions(log4go.FileWriterOptions{
    Filename: "/var/log/app-%Y%M%D.log", Rotate: true, Daily: true, MaxDays: 30,
})
log4go.Register(fw)

kw := log4go.NewKafKaWriter(log4go.KafKaWriterOptions{
    Brokers: []string{"kafka-1:9092"}, ProducerTopic: "app-log",
    BufferSize: 65536, OverflowPolicy: "spill", SpillType: "ring", SpillSize: 65536,
})
log4go.Register(kw)
log4go.Info("bid req=%s", reqID)
```

## 6. Scenario config (ad-tech)

**100K QPS** (standard bidding logs): single instance suffices (File 2.95M / Kafka buildPayload 1.0–3.5M). File daily-rotate + Kafka spill-ring.

**10M QPS** (full impression/click stream): single bootstrap serial is the ceiling (~2–3M/core). **Shard horizontally**: per-shard KafKaWriter + `spill=file`, N pods, Kafka partitions ≥ concurrency.

## 7–11. Monitoring, verification

- Pull: `Metrics()` (per-level), `kw.Metrics()` (Sent/Errored/Dropped/Spilled/Queued/SpillLen).
- Push: `SetOnEvent(name, delta)` real-time hook → Prometheus/statsd.
- Verify locally without real Kafka: `go test -bench . -benchmem -run '^$'`; sarama mocks for e2e; noopAsyncProducer for benches.

## 12. Per-writer throughput (full, Go 1.26 / M5 10c)

| ConsoleWriter (buffered) | ~129ns | 7.8M |
| FileWriter (async+spill) | ~186ns | 5.4M |
| KafKaWriter (mock) | ~879ns | 1.1M |
| NetWriter (TCP loopback) | ~76ns | 13.1M (caller-side; real net RTT-bound) |
| IOWriter (discard) | ~297ns | 3.4M |
| Record.JSON (goccy, 3 flds) | ~210–350ns | typed append |

## 13. Memory (100K records, MemPerWriter)

All writers HeapAlloc < 0.005MB, goroutines constant **4** (pool reuse + bounded chans + bounded spiller). 1M records: HeapAlloc 1.3MB, NumGC 7, goroutines 3.

## 16. Round A: typed fields + slog + logfmt + presets + Panic/Fatal

Three optimization layers: ① typed append (no map/reflection/boxing) ② `appendISOTimeUTC` (no time.Format string alloc) ③ batched string escaping (clean-run single append). JSON/logfmt down to **1 alloc**.

| Bench | peak ns | allocs | note |
|---|---|---|---|
| Record.JSON (3 flds) | ~195 | **1** | vs 5801/6 baseline (**−95% / −83%**) |
| Record.Logfmt (3 flds) | ~229 | **1** | |
| Kafka buildPayload (5 base) | ~432 | 2 | |
| SlogHandler.Handle | ~1270 | 7 | slog bridge (+19% vs native) |
| Logger.WithInt (typed) vs With(iface) | ~138 vs ~140 | 3 vs 3 | typed avoids type-switch |
| Field append int / float | ~15 / ~45 | **0** | zero-alloc scalars |

**codec comparison**: goccy/std/sonic all 240B/1alloc for scalar records — codec choice is irrelevant for scalars (only kindAny hits it).

## 17. Type coverage & robustness (vs zap/zerolog/slog)

fieldOf maps: string/bool/int*/uint*/uintptr/float/[]byte/complex/duration/time/error → typed (zero-box); else kindAny.

| Risk | Handling |
|---|---|
| panicking MarshalJSON | `safeJSONMarshal` recover → null |
| typed-nil error | `safeErrorString` recover → null |
| chan/func unmarshallable | kindAny fail → null |
| NaN/±Inf (invalid JSON) | bitmask → null |
| complex NaN component | → null |

> Recovery only on kindAny/kindError (slow path); scalar hot path zero overhead; float NaN/Inf via single bitmask AND.

## 17.4 Pipeline alloc floor (pprof)

`LoggerInfo` (caller + format args): 7 → 6 (path.Base→slice, Itoa→AppendInt) → **3** (caller cache: PC→file:line memoize via runtime.Callers).

| path | allocs | composition |
|---|---|---|
| hasCaller + `Info(fmt,i)` | **3** | Sprintf(1) + variadic boxing(2) — Go floor |
| WithCaller(false) + no-arg `Info("msg")` | **1** | max single-core throughput |
| Record.JSON (3 flds) | **1** | buf (multi-writer/async floor) |

> Below 3 needs builder API (zerolog chain, avoids variadic boxing) — trades `Info("msg")` ergonomics; declined.

## 18. Sharding strategy & multi-env (vs zap/zerolog/slog)

Sharding pays off when the **writer bottlenecks the single bootstrap** (slow writer ~µs). Measured (M5/10c, slowWriter ~1µs): 1→2 shards ~2×, 4 shards ~3.1× (peak), 8 regresses (contention). Fast writer (discard/memory): sharding only adds dispatch overhead.

**AutoShardCount** = `max(2, GOMAXPROCS/2)`. `/2` leaves cores for producers; floor 2. No hard cap — shard count scales with cores (a 64-core box needs more parallel consumers; capping at 8 would bottleneck it).

| GOMAXPROCS | shards | env |
|---|---|---|
| 1 | 2 | single-core container |
| 4 | 2 | common cloud |
| 8 | 4 | |
| 16 | 8 | |
| 32 | 16 | scales with cores |
| 64 | 32 | big machine, more consumers |
| 128 | 64 | |

Multi-env: Go 1.25+ honors cgroup CPU quota natively; older/abnormal → `import _ "github.com/v8fg/kit4go/maxprocs"`.

| log4go | zap | zerolog | slog | |
|---|---|---|---|---|
| typed zero-box | yes | yes | yes | yes |
| overflow anti-OOM | yes built-in | no self-build | no | no |
| crash recovery | yes built-in | no | no | no |
| multi-core sharding | yes built-in+auto | no | no | no |
| strict ordering | yes | no | no | no |
| alert webhook | yes | Hook | Hook | — |
| field robustness | yes | no | no | no |

## 19. Monitoring & diagnostics

- Startup: `[log4go] ShardLogger started: GOMAXPROCS=N shards=M`.
- `RuntimeStats()` → GOMAXPROCS/NumGoroutine/HeapAlloc/HeapInuse/HeapObjects/NumGC/GCCPUFraction. Calls ReadMemStats (sub-ms STW) — **never on the hot path**; gather at scrape cadence.
- Prometheus collector example in PERFORMANCE.md §19.3.

## 20. High-performance vs easy usage (cookbook)

| Tier | Usage | allocs | peak ns | QPS/core | ease | for |
|---|---|---|---|---|---|---|
| **A easy** | `NewProduction()` + `Info("msg")` | **1** | ~1100 | ~910K | high | 99% logs |
| A | `With(k,v).Info(fmt,i)` | 3 | ~1200 | ~830K | high | structured |
| **B perf structured** | `WithString/WithInt/WithAttrs` | 2-3 | ~1250 | ~800K | medium | high-rate+fields |
| **C max throughput** | `WithCaller(false)` + no-arg + async File | **1** | ~1100 | **~923K** | low | ultra-high-rate |
| **D multi-core** | `ShardLogger(0)` + `RegisterFunc` | — | shard×core | 4 shard ~3× | low | 10M-class |

**Switch payoff (ranked)**: `WithCaller(false)` −2 allocs (biggest single win) → no-arg `Info("msg")` −2 allocs → typed fields (zero-box scalars) → `FileWriter{Async:true}` → `OverflowPolicy:"spill"` → `ShardLogger(0)`.

**Anti-patterns**: unbuffered Console in prod; sharing one `*FileWriter` across shards (use RegisterFunc); high-rate `With("count",i)` (use WithInt); NetWriter for high volume.

> The **default tier is already solid** (typed fields, caller cache, zero reflection, + overflow/recovery/sharding/alerting). Unless a bottleneck is measured (single-core >900K QPS or 10M-class), use the default — spend effort on the business.

## 21. Stress & soak verification (2026-06-28, Apple M5 10c, Go 1.26)

Full stress re-run after the v0.1.0 multi-module split + golangci-lint migration. All
numbers `go test -bench -benchmem` unless noted.

### Deliver pipeline — multi-core scaling (the ad-tech hot path)

| Benchmark | 1 CPU | 4 CPU | 8 CPU | allocs/op |
|---|---|---|---|---|
| `LoggerInfo` (caller + writer) | 6056 ns | 1492 ns | **1468 ns** | 3 |
| `LoggerInfoNoCaller` (writer) | 5995 ns | 1441 ns | **1374 ns** | **1** |
| `DeliverPipeline_Discard` | 5974 ns | 1473 ns | 1473 ns | 3 |
| `DeliverPipeline_SampledActive` | 5916 ns | 1537 ns | 1519 ns | 1 |
| `DeliverPipeline_Filtered` | 13.1 ns | 11.5 ns | 11.5 ns | **0** |
| `DeliverPipeline_SampledOut` | 3.27 ns | 3.27 ns | 3.27 ns | **0** |

- **1 → 4 CPU ≈ 4× scaling** (~165K → ~690K rec/s/core), **plateau at 4–8 CPU** — the
  documented channel-scheduling bottleneck. `ShardLogger` is the path past the plateau.
- Hot path **≤1 alloc/op** (no-caller); **0 alloc** on the filter/sample-drop paths.

### Soak (sustained load) — throughput stability + leak

- **10s sustained @ 8 CPU**: `LoggerInfo` 1468 ns/op, `NoCaller` 1374 ns/op — **identical
  to the 2s baseline** → no throughput degradation under sustained load.
- **Heap (1,000,000 records + GC)**: `before 603 KB → after 607 KB, Δ +3 KB` (~3
  bytes/record retained = allocator fragmentation) → **no memory leak**.
- **goleak** (`shutdown_leak_test.go`): **0 goroutine leak** after Close.
- **stress/** (`TestStress_AllClients`, `TestStress_ConcurrentSafety`): 10K ops × 5
  client types, **PASS under `-race`** — no data races.

### Codec & fields (zero-alloc where it matters)

| Benchmark | ns/op | B/op | allocs |
|---|---|---|---|
| `Field_IntJSON` | 15.2 | 0 | **0** |
| `Field_FloatJSON` | 37.0 | 0 | **0** |
| `KafKaCodec_JSON` | 196 | 384 | 2 |
| `KafKaCodec_Proto` | 250 | 288 | 3 |
| `Logger_DeliverTypedFields` (6 fields) | 1542 | 696 | 6 |

**Verdict**: all performance claims hold — ~700K rec/s/core with a writer, zero-alloc
drop paths, multi-core scaling, sustained-load stability, no leak, race-clean. Meets the
100k–1M+ QPS ad-tech target. Repo-wide numbers (microservice infra + clients) are in the
root `BENCHMARKS.md`.

## 22. KafKaWriter batch daemon mode (2026-06-29, Apple M5 10c, Go 1.26)

`BatchMode` (opt-in, default **off**) makes the daemon accumulate records and flush via
`producer.SendBatch` on `BatchSize` (**default 1024**, `DefaultKafkaBatchSize` — was 100; the
stress matrix shows 1024 is the best default: near-peak QPS for franz-go which needs batch ≥1024,
flat for sarama, no extra memory; see `kafka/STRESS_MATRIX.md`) / `BatchFlushInterval` (50ms) /
shutdown — instead of one `producer.Send` per record. The **Write hot path is unchanged**
(batching lives entirely in the daemon goroutine); the `Producer`/`SyncProducer` interfaces
and the per-record default path are untouched.

### Pipeline benchmark — full path (Write → channel → daemon → mock producer), `block` policy

| Benchmark | ns/op | B/op | allocs |
|---|---|---|---|
| `KafKaWriter_Pipeline_PerRecord` | 624 | 1033 | 2 |
| `KafKaWriter_Pipeline_Batch` (size 200, 10ms) | 696 | 1152 | 2 |

**Honest reading — batch is ~11% *slower* against a no-op mock.** With an instant
`producer.Send` there is no real per-call cost to amortize, so the batching machinery
(slice accumulate + flush) is pure overhead.

### Real-broker throughput (apache/kafka 3.8.0 KRaft, default 10ms linger, `block` policy)

| Mode | rec/s (3 runs) | batch / per-record |
|---|---|---|
| per-record (default) | 35,154 / 36,072 / 35,465 | — |
| batch (size 200, 10ms) | 35,607 / 35,291 / 35,554 | **0.98–1.01× (within noise ≈ parity)** |

**Definitive result: under the kafka backend's default 10ms linger, log4go batch mode has NO
measurable throughput benefit** (0.98–1.01× across runs ≈ parity, both ~35K rec/s on this
localhost single-broker setup). The backend's linger coalesces *both* per-record `Send`s and
`SendBatch`es into the same broker-level batches, so the daemon calling Send vs SendBatch is
invisible at the broker. **This is why `BatchMode` defaults off**: with the default linger it
is pure overhead (mock: −11%; broker: ≈ parity) for no throughput gain.

### When BatchMode DOES win — per-call cost is the key

The async producer's `Send` is a ~free enqueue, so there is no per-call cost for `SendBatch`
to amortize → parity. But **batching pays off dramatically when the producer call has cost**
(sync producer, an overloaded broker backing up the daemon, or heavy per-call work). Proven by
`Test_KafKaWriter_BatchFasterWhenSendCostly` — a mock charging 50µs per *call* (once per Send/
SendBatch, not per record):

| Mode (5000 rec, 50µs/call) | rec/s | speedup |
|---|---|---|
| per-record | 13,754 | — |
| batch (size 100) | 266,535 | **19.4×** |

So BatchMode is correct and valuable — it amortizes per-call cost — it just has nothing to
amortize against log4go's default free-enqueue async producer. Enable it when the daemon's
producer calls are (or may become) the bottleneck.

### `ProducerLinger` knob (exposed on `KafKaWriterOptions`)

`ProducerLinger` tunes the kafka **backend**'s batch flush delay (the latency/throughput knob):
`0` = backend default (10ms); `>0` = explicit linger; `kafka.LingerOff` = disable backend
time-batching. It does NOT make log4go `BatchMode` faster — the backend still count-batches
(`Flush.Messages` = `MaxBufferedRecords`) for both modes, so Send vs SendBatch stays ≈ parity.

**`LingerOff` deadlock footgun (sarama backend, fixed)**: setting only `Flush.Frequency=0`
while `Flush.Messages` stays at the default 1000 means a final partial batch never reaches the
count threshold and there's no timer to flush it → under `OverflowPolicy:"block"` the writer
deadlocks (verified: stuck at offset 9257/10000). log4go guards this: `ProducerLinger=kafka.LingerOff`
**also** forces `MaxBufferedRecords=1` (`Flush.Messages=1`) so the backend flushes every record
on arrival — true "no batching", safe. So `LingerOff` = flush-per-record (lowest latency, highest
per-record overhead), not a batch-mode throughput lever.

DATA-LOSS: records held in the un-flushed batch are lost on a hard crash; `Stop()` flushes
on graceful shutdown (verified by `Test_KafKaWriter_BatchMode_ShutdownFlush`). Keep
`BatchFlushInterval` small. Monitoring: `WriterMetrics.Batches` (flush count) and
`BatchMax` (largest batch flushed) — 0 in per-record mode.

**kafka→log4go monitoring bridge**: `WriterMetrics` also surfaces the underlying
`kafka.Producer`'s real-time buffer depth — `InFlight` (linger backlog records),
`BufferedBytes` (buffer memory), `Backend` ("sarama"/"franz-go") — which `Queued`
(channel depth) alone does not show. For full depth (UTC `Timestamp`, `Linger`,
`MaxBufferedRecs`, all byte counters) call `KafKaWriter.ProducerSnapshot()` →
`kafka.ProducerSnapshot` (nil-safe; type-assert the producer against
`kafka.SnapshotHistory` for trend samples). Verified by
`Test_KafKaWriter_ProducerMetricsBridge`.

Robustness fix landed alongside: `k.run` is now set in `Start()` *before* the daemon
goroutine launches, so `Stop()` works immediately after `Start()` returns (previously a
Stop issued before the daemon scheduled would no-op and drop the un-flushed batch — a race
the shutdown test exposed under `-cover`).

## 23. Overflow policy: drop vs spill — performance & data impact (2026-06-29)

### Hot path (channel not full, 99%+ of the time)

**drop and spill are identical — zero difference.** Both do a single non-blocking
channel send. The policy only matters when the channel overflows.

### Overflow behavior (channel full)

| | drop | spill (ring) | spill (file) |
|---|---|---|---|
| Write cost | O(1): counter increment | O(1): mutex + slice append | O(disk): file write |
| Data | **permanently lost** | saved in-memory ring | **saved to disk** |
| Process crash | lost | lost | **survives** (recovered on next Start) |
| Recovery cost | none | daemon re-injects from ring | daemon re-injects from file |
| Memory | zero | ring cap × record size | zero (on disk) |
| Disk | zero | zero | bounded by SpillMaxBytes |

### Real-world data loss at 100K QPS (BufferSize=1024)

| Incident | Duration | Backlog | drop loses | spill(ring 1024) loses | spill(file) loses |
|---|---|---|---|---|---|
| GC pause | 10ms | 1,000 | 0 | 0 | 0 |
| Broker slow | 100ms | 10,000 | ~9,000 | ~8,000 | **0** |
| Broker down | 10s | 1,000,000 | ~999,000 | ~998,000 | bounded by SpillMaxBytes |
| Process crash | — | — | all | all (in-memory) | **disk records survive** |

### Configuration constants (use these instead of magic strings)

```go
// Overflow policy
OverflowPolicyDrop    // "drop"  — fast, data lost. Default. Ad-tech/RTB logs.
OverflowPolicyBlock   // "block" — backpressure, can stall hot path. Not for RTB.
OverflowPolicySpill   // "spill" — durable recovery. Money/critical data.

// Spill type (when policy == spill)
SpillTypeRing         // "ring"  — in-memory, fast, lost on crash. Default.
SpillTypeFile         // "file"  — disk-backed, survives crash.
SpillTypeChain        // "chain" — ring→file→drop. Best of both.
```

### Recommended configurations by tier

```go
// RTB logs: drop (fastest, lossy is fine)
log4go.KafKaWriterOptions{
    OverflowPolicy: log4go.OverflowPolicyDrop,
    BufferSize:     1024,
    Acks:           kafka.AcksLeader,
}

// Conversions / spend: spill(file) + acks=all (durable)
log4go.KafKaWriterOptions{
    OverflowPolicy: log4go.OverflowPolicySpill,
    SpillType:      log4go.SpillTypeFile,
    SpillDir:       "/var/log/kafka-spill",
    SpillMaxBytes:  256 << 20, // 256MB
    BufferSize:     8192,
    Acks:           kafka.AcksAll,
}
```

## 24. Ad-tech event strategy, funnel, and sizing (2026-06-30)

Ad-tech event types differ in volume, value, and loss tolerance. The log4go + kafka
stack is per-writer configurable, so each stream gets the right durability/throughput
tradeoff. This section ties event value to industry funnel ratios, sizes
payload / disk / memory / bandwidth / CPU at three scales, and ends with concrete
config and compliance notes.

### Event value matrix — why each tier exists

| Event type | Volume | Value/record | Loss tolerance | Billing impact | Tier |
|---|---|---|---|---|---|
| RTB bid RR (request/response) | Extreme (~1M QPS) | Debug only | High | None | Tier 1: max throughput |
| Impressions | Very high (~100K-500K/s) | Low (CPM aggregate) | Medium (MRC allows 1-5% loss) | Minimal per-record | Tier 1: max throughput |
| Clicks | Medium (~1K-10K/s) | Medium (CPC billing + fraud) | Low | Revenue per click | Tier 2: balanced |
| Conversions | Low (~10-100/s) | High (CPA revenue attribution) | Very low | Direct revenue loss | Tier 3: durable |
| Budget/spend updates | Very low (~1-10/s) | Critical (overspend risk) | None | Direct money loss | Tier 3+: max durable |

### Sources

- [AWS RTB Fabric](https://aws.amazon.com/blogs/industries/next-generation-programmatic-advertising-how-aws-rtb-fabric-redefines-the-game/): 90% no-bid rate; ~3KB avg OpenRTB bid request
- [Digital Applied](https://www.digitalapplied.com/blog/programmatic-advertising-statistics-2026-data-points): DSP win rate 6.3% open exchange, 14.7% PMP
- [SmartInsights 2024](https://www.smartinsights.com/internet-advertising/internet-advertising-analytics/display-advertising-clickthrough-rates/): display CTR 0.46%
- [CXL](https://cxl.com/guides/click-through-rate/benchmarks/): display CTR 0.57%
- [Enstacked](https://enstacked.com/average-conversion-rate-for-google-ads/): display click-to-lead CVR 0.89%
- [Google Ad Manager](https://support.google.com/admanager/answer/1733124): 25-35 bytes/event compressed
- [AdMonsters LLD](https://www.admonsters.com/use-cases-log-level-data/): LLD 0.5-2KB Avro, 2-5KB JSON
- [IAB OpenRTB 2.6](https://iabtechlab.com/wp-content/uploads/2022/04/OpenRTB-2-6_FINAL.pdf): spec

### Industry funnel — verified conversion rates

| Step | From → To | Range | Mean | % of top | Source |
|---|---|---|---|---|---|
| Bid | Req → Response | 5-40% | **15%** | 15% | AWS: 90% no-bid |
| Win | Response → Win | 1-20% | **6.3%** | 0.95% | Digital Applied |
| Deliver | Win → Impression | 85-99% | **93%** | 0.88% | Industry consensus |
| CTR | Impression → Click | 0.08-1.5% | **0.46%** | 0.0041% | SmartInsights/CXL |
| CVR | Click → Conversion | 0.5-5% | **1.5%** | 0.000061% | Enstacked |
| Postback | Conv → Postback | 100-300% | **200%** | 0.00012% | Industry consensus |

### Payload sizes per stage

| Stage | Payload | Fields (typical) | Source |
|---|---|---|---|
| RTB RR log | **500B** | req_id, SSP, auction_id, bid_price, creative_id, user_hash, device, ts (15-20 fields) | Condensed from 3KB OpenRTB (AWS) |
| Bid Response log | **300B** | req_id, resp_price, creative, deal_id, ts (10-12 fields) | Subset of RR |
| Win Notice | **150B** | req_id, clearing_price, currency, ts (5-7 fields) | n/a |
| Impression | **400B** | imp_id, creative, placement, user_hash, price, geo, device, ts (15 fields) | GAM: 200-500B uncompressed |
| Click | **750B** | click_id, imp_id, user_hash, landing_url, referrer, device, ts (20 fields) | LLD: 0.5-2KB Avro |
| Conversion | **1,500B** | conv_id, click_id, imp_id, revenue, currency, product, attribution, PII, ts (25 fields) | LLD: 2-5KB JSON |
| Postback | **750B** | postback_id, conv_id, partner, event_type, SKAN, ts (15 fields) | n/a |

### Sizing at three scales: 100K / 500K / 1M bid QPS

Funnel ratios are constant; absolute volumes scale linearly with bid QPS.

#### 100K bid QPS (small-mid DSP)

| Stage | Conv | QPS | Payload | MB/s | GB/d | GB/d RF3+LZ4 | Mem | TTL | Disk |
|---|---|---|---|---|---|---|---|---|---|
| RTB RR | — | 100K | 500B | **47.7** | 4,111 | 6,166 | 3.9MB | 1d | **6.2TB** |
| Bid Resp | 15% | 15K | 300B | 4.3 | 370 | 555 | 2.4MB | 1d | 0.6TB |
| Win | 6.3% | 945 | 150B | 0.13 | 11.6 | 17.4 | 1.7MB | 7d | 0.12TB |
| Impression | 93% | 879 | 400B | 0.34 | 29.1 | 43.6 | 4.9MB | 30d | 1.3TB |
| Click | 0.46% | 4.0 | 750B | 0.003 | 0.25 | 0.38 | 4.6MB | 90d | 34GB |
| Conversion | 1.5% | 0.06 | 1.5KB | 0.0001 | 0.008 | 0.012 | 9.2MB | 365d | 4.4GB |
| Postback | 200% | 0.12 | 750B | 0.0001 | 0.008 | 0.012 | 2.3MB | 365d | 4.4GB |
| **Total** | | | | **52.5** | **4,522** | **6,784** | **29MB** | | **~8.3TB** |

#### 500K bid QPS (mid-large DSP)

| Stage | Conv | QPS | Payload | MB/s | GB/d | GB/d RF3+LZ4 | Mem | TTL | Disk |
|---|---|---|---|---|---|---|---|---|---|
| RTB RR | — | 500K | 500B | **238.4** | 20,553 | 30,830 | 3.9MB | 1d | **30.8TB** |
| Bid Resp | 15% | 75K | 300B | 21.4 | 1,849 | 2,774 | 2.4MB | 1d | 2.8TB |
| Win | 6.3% | 4,725 | 150B | 0.67 | 57.9 | 86.9 | 1.7MB | 7d | 0.6TB |
| Impression | 93% | 4,394 | 400B | 1.68 | 144.8 | 217.2 | 4.9MB | 30d | 6.5TB |
| Click | 0.46% | 20.2 | 750B | 0.014 | 1.24 | 1.86 | 4.6MB | 90d | 167GB |
| Conversion | 1.5% | 0.30 | 1.5KB | 0.0004 | 0.039 | 0.059 | 9.2MB | 365d | 22GB |
| Postback | 200% | 0.60 | 750B | 0.0004 | 0.039 | 0.059 | 2.3MB | 365d | 22GB |
| **Total** | | | | **262.2** | **22,637** | **33,911** | **29MB** | | **~40.9TB** |

#### 1000K (1M) bid QPS (large DSP)

| Stage | Conv | QPS | Payload | MB/s | GB/d | GB/d RF3+LZ4 | Mem | TTL | Disk |
|---|---|---|---|---|---|---|---|---|---|
| RTB RR | — | 1M | 500B | **476.8** | 41,106 | 61,660 | 3.9MB | 1d | **61.7TB** |
| Bid Resp | 15% | 150K | 300B | 42.7 | 3,698 | 5,548 | 2.4MB | 1d | 5.5TB |
| Win | 6.3% | 9,450 | 150B | 1.34 | 115.8 | 173.7 | 1.7MB | 7d | 1.2TB |
| Impression | 93% | 8,789 | 400B | 3.35 | 289.6 | 434.4 | 4.9MB | 30d | 13.0TB |
| Click | 0.46% | 40.4 | 750B | 0.029 | 2.47 | 3.71 | 4.6MB | 90d | 334GB |
| Conversion | 1.5% | 0.61 | 1.5KB | 0.0009 | 0.078 | 0.117 | 9.2MB | 365d | 43GB |
| Postback | 200% | 1.21 | 750B | 0.0009 | 0.078 | 0.117 | 2.3MB | 365d | 43GB |
| **Total** | | | | **524.3** | **45,270** | **67,621** | **29MB** | | **~81.8TB** |

### Side-by-side comparison

| Metric | 100K QPS | 500K QPS | 1M QPS | Scale |
|---|---|---|---|---|
| RTB RR MB/s | 47.7 | 238.4 | 476.8 | 1× / 5× / 10× |
| Impression QPS | 879 | 4,394 | 8,789 | |
| Click QPS | 4.0 | 20.2 | 40.4 | |
| Conversion QPS | 0.06 | 0.30 | 0.61 | |
| Total GB/day (RF3+LZ4) | 6,784 | 33,911 | 67,621 | |
| Total disk (with TTL) | 8.3TB | 40.9TB | 81.8TB | |
| Total log4go memory | 29MB | 29MB | 29MB | **Constant** |
| Kafka brokers needed | 3-5 | 6-12 | 12-20 | Scale with RTB volume |

**Memory is constant (29MB) at all scales** — buffer/spill sizes are fixed by
config, not by QPS. Only Kafka disk and broker count scale with bid QPS.

### Per-stage configuration (same at all scales)

| Stage | Batch | Size | Buffer | Overflow | Spill | Acks | Mem |
|---|---|---|---|---|---|---|---|
| RTB RR | true | 2048 | 8192 | drop | — | leader | 3.9MB |
| Bid Resp | true | 1024 | 8192 | drop | — | leader | 2.4MB |
| Win | true | 128 | 4096 | spill | 8192 | leader | 1.7MB |
| Impression | true | 128 | 4096 | spill | 8192 | all | 4.9MB |
| Click | true | 8 | 2048 | spill | 4096 | all | 4.6MB |
| Conversion | false | — | 4096 | spill | 2048 | all | 9.2MB |
| Postback | false | — | 2048 | spill | 1024 | all | 2.3MB |

```go
// Tier 1 (RTB/impressions): drop + leader — never block the bidding loop.
log4go.KafKaWriterOptions{
    ProducerTopic:  "impressions",
    OverflowPolicy: log4go.OverflowPolicyDrop,
    BufferSize:     4096,
    BatchMode:      true,
    BatchSize:      128,
    Acks:           kafka.AcksLeader, // default; accepts rare leader-failure loss
}

// Tier 3 (conversions/budget): spill(chain) + acks=all — durable money data.
log4go.KafKaWriterOptions{
    ProducerTopic:  "conversions",
    OverflowPolicy: log4go.OverflowPolicySpill,
    SpillType:      log4go.SpillTypeChain, // ring -> file -> drop
    SpillSize:      2048,
    SpillDir:       "/mnt/spill",          // MUST be a mounted volume in containers
    SpillMaxBytes:  128 << 20,             // 128MB
    BufferSize:     4096,
    BatchMode:      false,                 // low volume; send per record
    Acks:           kafka.AcksAll,         // wait for all ISR replicas
}
```

### Tier design rationale

- **Tier 1 (drop + leader)**: fastest path, never blocks the bidding loop. Ad-server
  logging is inherently ~1-5% lossy per MRC, so a lost impression log does not skew CPM
  aggregates. Kafka RF=2, min.insync=1.
- **Tier 2 (clicks, larger buffer + smaller batch)**: clicks are 10-100× lower volume than
  impressions, so a larger buffer absorbs longer broker stalls without drops; a smaller
  batch lowers latency for fraud-detection pipelines. Kafka RF=3, min.insync=1 (click loss
  is detectable via ad-server reconciliation).
- **Tier 3 (spill + acks=all)**: every conversion is replicated to RF=3 before ack.
  spill(chain) buffers records through a broker outage with no loss; the small/no batch
  keeps attribution latency low. Kafka RF=3, min.insync=2, cleanup.policy=compact for
  budget state.

### BatchMode threshold depends on acks

acks=all per-call round-trip (~2ms) is ~7× slower than acks=leader (~0.3ms). Batching
therefore pays off at much lower QPS under acks=all:

| acks | per-call | BatchMode helps above | Example |
|---|---|---|---|
| leader | ~0.3ms | ~5K/s | impressions, RTB |
| all | ~2ms | ~50/s | clicks, even low-volume |

### SpillSize formula

SpillSize = QPS × max_stall_seconds. Lower QPS → the same ring absorbs longer stalls.

| Stage | QPS | Target stall | SpillSize | Ring memory |
|---|---|---|---|---|
| Impressions | 25K | 500ms | 16384 | 6.5MB |
| Clicks | 200 | 30s | 8192 | 6.1MB |
| Conversions | 6 | 10min | 4096 | 6.1MB |

### Container environment — durability strategy

In containers (Docker/K8s), local file spill is NOT reliable unless on a mounted volume.
The correct durability model:

| Layer | What it protects | Container-safe? |
|---|---|---|
| kafka acks=all + RF=3 | Record after broker ack | ✅ Always safe |
| log4go spill(ring) | Record during broker outage (process alive) | ✅ In-memory |
| log4go spill(file) on overlay FS | Record during broker outage (process alive) | ⚠️ Lost on container recreation |
| log4go spill(file) on mounted emptyDir | Record during broker outage + container restart | ✅ Same pod only |
| log4go spill(file) on mounted PV (EBS/NFS) | Record survives pod rescheduling | ✅ Fully durable |

**Recommendation**: for money/critical data, rely on kafka acks=all + RF=3 as the primary
durability guarantee. Use spill(ring) for temporary broker-outage buffering. Only use
spill(file) with a properly mounted PV if the broker is expected to be unreachable for
extended periods.

### Kafka cluster-side configuration (not kit4go, but required for the strategy)

| Tier | RF | min.insync | compaction | Notes |
|---|---|---|---|---|
| Tier 1 (impressions) | 2 | 1 | delete (TTL 3-7d) | High throughput, short retention |
| Tier 2 (clicks) | 3 | 1 | delete (TTL 30d) | Fraud detection window |
| Tier 3 (conversions) | 3 | 2 | delete (TTL 90d+) | Audit/compliance retention |
| Budget state | 3 | 2 | compact | Latest state only (not history) |

### Disk: 1-day vs full-retention (RF=3, LZ4 compression)

**1-day volume (all stages, all scales)**

| Stage | 100K QPS | 500K QPS | 1M QPS | TTL |
|---|---|---|---|---|
| RTB RR | 6,166 GB | 30,830 GB | 61,660 GB | 1d |
| Bid Resp | 555 GB | 2,774 GB | 5,548 GB | 1d |
| Win | 2.5 GB | 12.4 GB | 24.8 GB | 1d |
| Impression | 1.5 GB | 7.2 GB | 14.5 GB | 1d |
| Click | 0.004 GB | 0.021 GB | 0.041 GB | 1d |
| Conversion | 0.00003 GB | 0.00016 GB | 0.00032 GB | 1d |
| Postback | 0.00003 GB | 0.00016 GB | 0.00032 GB | 1d |
| **1-day total** | **6,725 GB** | **33,624 GB** | **67,248 GB** | |
| | **6.6 TB** | **32.8 TB** | **65.7 TB** | |

**Full-retention volume (each stage at its own TTL)**

| Stage | TTL | 100K QPS | 500K QPS | 1M QPS |
|---|---|---|---|---|
| RTB RR | 1d | 6.2 TB | 30.8 TB | 61.7 TB |
| Bid Resp | 1d | 0.6 TB | 2.8 TB | 5.5 TB |
| Win | 7d | 17.4 GB × 7 = 0.12 TB | 86.9 GB × 7 = 0.6 TB | 173.7 GB × 7 = 1.2 TB |
| Impression | 30d | 43.6 GB × 30 = 1.3 TB | 217.2 GB × 30 = 6.5 TB | 434.4 GB × 30 = 13.0 TB |
| Click | 90d | 0.38 GB × 90 = 34 GB | 1.86 GB × 90 = 167 GB | 3.71 GB × 90 = 334 GB |
| Conversion | 365d | 0.012 GB × 365 = 4.4 GB | 0.059 GB × 365 = 22 GB | 0.117 GB × 365 = 43 GB |
| Postback | 365d | 0.012 GB × 365 = 4.4 GB | 0.059 GB × 365 = 22 GB | 0.117 GB × 365 = 43 GB |
| **Total disk** | | **8.3 TB** | **40.9 TB** | **81.8 TB** |

**Key insight**: RTB RR logs dominate disk (>90%) at any scale. Compressing RTB RR or
reducing its TTL from 1d to 12h cuts total disk nearly in half. All lower-funnel stages
(win through postback) combined are <3% of disk even at 1M QPS.

### Network bandwidth (producer → broker, uncompressed)

| Stage | Payload | 100K QPS | 500K QPS | 1M QPS |
|---|---|---|---|---|
| RTB RR | 500B | 47.7 MB/s (381 Mbps) | 238 MB/s (1,905 Mbps) | 477 MB/s (3,810 Mbps) |
| Bid Resp | 300B | 4.3 MB/s (34 Mbps) | 21.4 MB/s (172 Mbps) | 42.7 MB/s (342 Mbps) |
| Win | 150B | 0.13 MB/s (1.1 Mbps) | 0.67 MB/s (5.4 Mbps) | 1.34 MB/s (10.7 Mbps) |
| Impression | 400B | 0.34 MB/s (2.7 Mbps) | 1.68 MB/s (13.4 Mbps) | 3.35 MB/s (26.8 Mbps) |
| Click | 750B | 0.003 MB/s | 0.014 MB/s | 0.029 MB/s |
| Conversion | 1.5KB | 0.0001 MB/s | 0.0004 MB/s | 0.0009 MB/s |
| Postback | 750B | 0.0001 MB/s | 0.0004 MB/s | 0.0009 MB/s |
| **Total** | | **52.5 MB/s (420 Mbps)** | **262 MB/s (2,100 Mbps)** | **524 MB/s (4,194 Mbps)** |

With LZ4 (~50%): wire bandwidth halved. RF=3 replication: inter-broker × 3.

### CPU estimate (log4go daemon goroutine)

| Stage | QPS | Acks | Per-call | No-batch CPU | With batch | Batch |
|---|---|---|---|---|---|---|
| RTB RR | 500K | leader | ~0.3ms | 15,000% impossible | **7%** | 2048 |
| Bid Resp | 75K | leader | ~0.3ms | 2,250% impossible | **11%** | 1024 |
| Win | 4.7K | leader | ~0.3ms | 142% tight | **7%** | 128 |
| Impression | 4.4K | all | ~2ms | 880% impossible | **7%** | 128 |
| Click | 20 | all | ~2ms | 4% ok | **1%** | 8 |
| Conversion | 0.3 | all | ~2ms | 0.06% | 0.06% | off |
| Postback | 0.6 | all | ~2ms | 0.12% | 0.12% | off |

### Kafka partitions (producer-side recommendation)

| Stage | QPS range | Partitions | Rationale |
|---|---|---|---|
| RTB RR | 100K-1M | 30-100 | Parallel writes; partition by SSP or hour |
| Bid Resp | 15K-150K | 12-48 | Same topic or shared with RTB |
| Win | 945-9.5K | 6-24 | Partition by campaign or SSP |
| Impression | 879-8.8K | 6-24 | Partition by placement or user_hash |
| Click | 4-40 | 3-12 | Low volume; partition by campaign |
| Conversion | 0.06-0.6 | 3-6 | Very low; partition by advertiser_id |
| Postback | 0.12-1.2 | 1-3 | Minimal; single partition OK |

Rule: partitions ≈ peak_QPS / 1000 (each partition ~1K QPS max).

### TCP connections (per DSP instance)

7 KafKaWriter instances × 1 producer each = 7 bootstrap connections + up to
2 partition-leader connections per producer = **~7-21 TCP connections**.
Negligible for modern kernels.

### Data-loss risk per stage

| Stage | Risk source | Mitigation | Residual risk |
|---|---|---|---|
| RTB RR | drop on overflow | n/a (lossy) | Up to buffer-full during burst |
| Win | broker unreachable | spill(ring 8192) | Overflow if outage >1.7s at 4.7K/s |
| Impression | broker unreachable | spill(ring 8192) + acks=all | Overflow if outage >1.9s |
| Click | broker unreachable | spill(ring 4096) + acks=all | Overflow if outage >203s (3.4 min) |
| Conversion | broker unreachable | spill(ring 2048) + acks=all | Overflow if outage >17 hours |
| Process crash | all stages | acks=all after broker ack; spill(file) on bare metal | In-memory lost; file survives if PV mounted |

### Rough cloud cost (AWS, order-of-magnitude)

| Item | 100K QPS | 500K QPS | 1M QPS |
|---|---|---|---|
| Kafka brokers (m5.2xl × RF=3) | 3 ($870/mo) | 9 ($2,600/mo) | 18 ($5,200/mo) |
| EBS storage (gp3, full retention) | 8.3 TB ($190/mo) | 40.9 TB ($940/mo) | 81.8 TB ($1,880/mo) |
| S3 cold archive (RTB RR, >1d) | 3 TB ($70/mo) | 15 TB ($340/mo) | 30 TB ($690/mo) |
| Network egress | included | included | ~$200/mo |
| **Total (rough)** | **~$1,130/mo** | **~$3,880/mo** | **~$7,970/mo** |

### Industry compliance notes

- MRC (Media Rating Council): impression measurement allows 1-5% data loss in ad-server
  logging. Tier 1 (drop + acks=leader) is within this tolerance.
- IAB ads.txt/sellers.json: click-level data completeness matters for supply-chain fraud
  detection. Tier 2 (larger buffer) reduces click loss.
- GDPR/CCPA: conversion data may contain PII. Tier 3 (acks=all + RF=3) ensures data
  integrity for compliance audit trails.
- SOC 2 / ISO 27001: financial event logging (conversions, budget) requires durable,
  verifiable delivery. acks=all + RF=3 + min.insync=2 meets this bar.

### Total memory per DSP instance (all 7 writers)

~29MB combined — well within a 1-4GB container. No memory pressure at any scale; memory
is constant because buffer/spill sizes are config-fixed, not QPS-driven.

## 25. Buffer vs BatchSize — relationship and defaults (2026-06-29)

### Three buffering layers

```
Write() → [BufferSize channel] → daemon → [BatchSize accumulator] → SendBatch → [Linger backend] → broker
```

| Layer | Config | Default | Purpose |
|---|---|---|---|
| Buffer (channel) | BufferSize | 1024 | Decouple Write from daemon — Write never blocks while daemon briefly stalls |
| Batch (daemon) | BatchSize | 1024 | Amortize producer calls — accumulate N records before one SendBatch |
| Linger (backend) | ProducerLinger | 10ms | Amortize broker RPCs — backend accumulates up to 10ms before one request |

Buffer and BatchSize are independent. Buffer prevents Write from blocking; BatchSize
reduces per-call overhead. Memory: each holds its own records (~200KB at 1024 × 200B).

### Default configuration (ad-tech, BatchMode off)

```go
// Implicit defaults — no options needed
log4go.KafKaWriterOptions{
    // BufferSize:          1024,   // default
    // BatchMode:           false,  // default (per-record Send)
    // OverflowPolicy:      "drop", // default
    // Acks:                "",     // default = leader
}
```

BatchMode off: BufferSize is the only buffer. 1024 records at 100K QPS = ~10ms
headroom. With drop policy, a 10ms daemon stall causes zero drops. A 100ms stall
drops ~9K records (acceptable for impressions, not for conversions).

### When to change defaults

| Scenario | BufferSize | BatchSize | BatchMode | Why |
|---|---|---|---|---|
| RTB logs / impressions (default) | 1024 | 1024 | false | Drop-tolerant, max simplicity |
| High-throughput impressions | 4096 | 1024 | true | BatchMode for fewer producer calls |
| Clicks (fraud-sensitive) | 4096 | 512 | true | Larger buffer, smaller batch for lower latency |
| Conversions (money) | 8192 | 256 | true | Max buffer headroom, spill(chain) + acks=all |
| BatchMode off + block policy | 8192+ | N/A | false | Block needs large buffer to avoid premature backpressure |

Rule of thumb: BufferSize >= BatchSize. Buffer absorbs daemon stalls; BatchSize
controls producer-call frequency. They do not share memory.
