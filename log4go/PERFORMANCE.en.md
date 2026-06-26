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
