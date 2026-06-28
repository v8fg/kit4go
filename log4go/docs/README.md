# log4go — Comprehensive Evaluation & Usage Guide
# log4go — 全面评估与使用指南

**Version**: 2026.06 (dev branch, P1b + KafkaCodec complete)
**Coverage**: 100% | **Race**: clean | **Alloc**: ≤1/op (hot path)

---

## What log4go Is | log4go 是什么

A **high-performance structured logging library** for Go, designed for
ad-tech-scale (100k–1M+ QPS) production systems. It captures, serializes, and
ships log records to Kafka/file/console/net with zero hot-path overhead, while
providing industry-standard sampling, metrics, and runtime control.

一个为广告级吞吐（100k–1M+ QPS）设计的**高性能结构化日志库**。捕获、序列化、投递
日志到 Kafka/file/console/net，热路径零开销，提供业界标准的采样、指标和运行时控制。

---

## Architecture at a Glance | 架构一览

```
┌──────────── Application (caller) ────────────┐
│                                               │
│  Info("served %s", route)  ← per-record API   │
│       │                                       │
│       ▼                                       │
│  deliverRecordToWriter (HOT PATH, ≤1 alloc)   │
│    ├─ Occurred[level]++ (atomic)              │
│    ├─ level check (atomic load)               │
│    ├─ sampleDrop check (atomic load)          │
│    ├─ priorityLevel check (atomic load)       │
│    ├─ sampler.allow(level) (atomic counter)   │
│    ├─ caller cache (memoized, 0 alloc)        │
│    ├─ time format (cached per-second)          │
│    ├─ record pool (sync.Pool, 0 alloc)         │
│    └─ enqueue → records channel               │
│                                               │
└───────────────────┬───────────────────────────┘
                    │ (async, bounded)
┌───────────────────▼───────────────────────────┐
│  Bootstrap goroutine (single-writer)           │
│    ├─ deliver(r) → w.Write(r) per writer      │
│    ├─ Written[level]++ (non-atomic, no contention)│
│    └─ recordPool.Put(r) (recycle)              │
└───────────────────┬───────────────────────────┘
                    │
    ┌───────────────┼───────────────┐
    ▼               ▼               ▼
┌────────┐  ┌────────────┐  ┌──────────────┐
│ Console│  │ FileWriter │  │ KafKaWriter  │
│ (sync) │  │ (async+spill)│  │ (async+overflow)│
└────────┘  └────────────┘  └──────────────┘
```

### Design principles (8) | 设计原则（8 条）

1. **只做日志** — 不做 tracer/ES/大数据/告警渠道
2. **热路径神圣** — ≤1 alloc、无锁、无 I/O、关闭时零开销
3. **正交性** — 每个操作只有一个效果
4. **最小惊讶** — 名字必须诚实
5. **对标业界** — OTel/ECS/W3C/Dapper
6. **不过度设计** — 6 问决策框架
7. **验证强制** — 覆盖率/race/count/alloc/bench/goleak
8. **关停安全** — 幂等/无race/drain/无泄漏

See `docs/design_principles.md` for full detail.

---

## Capability Matrix | 能力矩阵

### Core logging

| Capability | API | Notes |
|------------|-----|-------|
| 9 log levels (EMERGENCY..TRACE) | `Debug/Info/Warn/Error/...` | RFC 5424 + TRACE |
| Structured fields | `With/WithField/WithFields` | zero-alloc typed (`WithString/Int/...`) |
| Text / JSON / Logfmt formats | `SetFormat` | pre-serialized once per record |
| Caller info (file:line) | `WithCaller(true)` | memoized (callerCache) |
| Context binding | `WithContext/IntoContext/FromContext` | request-scoped child loggers |
| Sampling (storm protection) | `WithSampling(initial, thereafter)` | per-level atomic counter |
| Child loggers | `With*/WithSampling/WithContext` | immutable copy-on-write |
| Panic/Fatal | `Panic/Fatal/Recover` | drains pipeline before exit |
| slog compatibility | `NewSlogHandler(logger)` | routes log/slog → log4go |

### Writers

| Writer | Async | Overflow | Durable | Use case |
|--------|-------|----------|---------|----------|
| `ConsoleWriter` | sync | — | — | dev / container stdout |
| `FileWriter` | sync or async | drop/block/spill | file (rotate daily/hourly) | prod logging |
| `KafKaWriter` | async | drop/block/spill | Kafka (JSON/protobuf) | prod → Kafka pipeline |
| `NetWriter` | async | drop/block | TCP/syslog | remote collector |
| `IOWriter` | sync | — | any io.Writer | test/buffer/custom |
| `WebhookWriter` | async (via sink) | rate-limited | HTTP POST | alerting (Lark/DingTalk/WeChat) |

### Runtime hot-update (RuntimeConfig interface)

All atomic, lock-free, zero hot-path cost:

| Knob | API | Effect |
|------|-----|--------|
| Level | `SetLevel(level)` | filter records below level |
| Format | `SetFormat(format)` | text ↔ JSON ↔ logfmt |
| Caller | `WithCaller(bool)` / `WithFuncName(bool)` | toggle caller info |
| Full path | `WithFullPath(bool)` | short filename ↔ full path |
| Sampling | `SetSampling(initial, thereafter)` | adjust per-level rate limiter |
| Context extractor | `SetContextExtractor(fn)` | hot-toggle trace capture |
| Base field upsert | `SetBaseField(key, val)` | add/update one infra field |
| Base field remove | `RemoveBaseField(key)` | delete one field |
| Base field clear | `ClearBaseFields()` | empty all base fields |

### Sampling strategy (industry-aligned)

| API | Algorithm | Cross-service consistent? |
|-----|-----------|--------------------------|
| `FullSampling{}` | keep all (default) | — |
| `TraceIDRatioBased{0.001}` | `hash(id) < ratio × MaxUint64` (OTel standard) | ✅ deterministic by id |
| `TailDigitSampling{10000, 1}` | `hash(id) % 10000 < 1` | ✅ deterministic by id |
| Custom (implement interface) | caller decides | caller's responsibility |
| `SetSamplingStrategy(strategy)` | install (runtime swap) | — |
| `SetSamplingStrategyFor(s, 30min)` | temporary window (auto-revert) | — |
| `SetPriorityLevel(ERROR)` | errors always bypass sampling | — |

### Per-writer control

| API | Effect |
|-----|--------|
| `PauseWriter(name)` / `writer.Pause()` | drop records (non-destructive, atomic) |
| `ResumeWriter(name)` / `writer.Resume()` | restore delivery |
| `WriterPaused(name)` / `writer.Paused()` | check state |
| `Writers()` | list all registered writers |

### Metrics funnel

| Metric | Where counted | Per-level | Per-writer |
|--------|---------------|-----------|------------|
| `Occurred` | caller entry (pre-filter) | ✅ | — |
| `Records` (Written) | bootstrap (post-deliver) | ✅ | ✅ (WriterMetrics) |
| `Dropped` | Occurred − Written | ✅ | ✅ (WriterMetrics) |
| `Status()` | aggregation snapshot | ✅ | ✅ (name/paused/metrics) |

### Shutdown safety

| Guarantee | How verified |
|-----------|-------------|
| Idempotent Stop/Close | tested |
| No send-on-closed panic | quit channel (records never closed) |
| No goroutine leak | goleak.VerifyNone |
| Drains in-flight records | bootstrap drainAndExit on quit |
| Stops all writers | Stopper interface (File/Kafka/Net) |

---

## Performance (verified) | 性能（已验证）

### Hot path

| Metric | Value | How verified |
|--------|-------|-------------|
| ns/op (no-args Info, discard writer, 4 CPU) | **1,393 ns** | `Benchmark_DeliverPipeline_NoCaller-4` |
| allocs/op (no-args Info) | **≤1** | `TestAllocBudget_HotPath` CI gate |
| ns/op (sampled-out, ratio=0) | **3.2 ns** | `Benchmark_DeliverPipeline_SampledOut` |
| ns/op (level-filtered) | **11–13 ns** | `Benchmark_DeliverPipeline_Filtered` |
| Sampling overhead | **~0 ns** (noise) | 1459 vs 1495 ns at 4 CPU |

### CPU profile

- **44%** Go runtime channel wake (pthread_cond_signal)
- **27%** Go runtime goroutine wait (pthread_cond_wait)
- **<1%** log4go code (format/pool/enqueue)
- **~0%** sampling/metrics additions

**Bottleneck = Go runtime channel scheduling, not log4go code.**
Path to higher throughput = ShardLogger (multi-channel) — already exists.

### Scaling

| Config | Throughput | Notes |
|--------|-----------|-------|
| Single Logger, 1 CPU | ~170k rec/s | baseline |
| Single Logger, 4 CPU | ~670k rec/s | 4× scaling |
| ShardLogger(4), 4+ CPU | scales linearly | distributes channel pressure |
| SampledOut (ratio=0) | **~300M rec/s** | drops before record build |
| WithCaller(false) | ~40% faster | skips runtime.Callers |

---

## Quick Start (RTB / ad-tech) | 快速上手（RTB / 广告）

```go
package main

import (
    "log4go"
    "time"
)

func main() {
    // 1. Infrastructure fields (startup, once)
    log4go.SetBaseField("service_name", "bidder")
    log4go.SetBaseField("host", hostname())
    log4go.SetBaseField("ip", localIP())
    log4go.SetBaseField("env", "prod")
    log4go.SetBaseField("region", "ap-southeast-1")
    log4go.SetBaseField("es_index", "adx-logs-"+time.Now().Format("2006.01"))

    // 2. Level control (primary volume knob)
    log4go.SetLevel(log4go.INFO)

    // 3. Error protection (industry standard — errors always logged)
    log4go.SetPriorityLevel(log4go.ERROR)

    // 4. Sampling for Kafka bandwidth control (adjustable, default full)
    log4go.SetSamplingStrategy(log4go.TraceIDRatioBased{Ratio: 0.001}) // 0.1%

    // 5. Kafka writer (async, bounded, overflow-safe)
    log4go.Register(log4go.NewKafKaWriter(log4go.KafKaWriterOptions{
        Enable: true, Brokers: []string{"kafka:9092"},
        ProducerTopic: "adx-logs", BufferSize: 1 << 14,
        OverflowPolicy: "spill", SpillType: "ring", SpillSize: 1 << 16,
    }))

    defer log4go.Close()

    // Per-request logging (business record)
    // In your handler:
    //   lg := log4go.WithContext(ctx). // auto-extracts trace_id/request_id/device_id
    //       WithString("ad_id", adID).
    //       WithString("campaign_id", campID).
    //       WithFloat64("bid_price", price)
    //   ... process ...
    //   lg.WithString("error_code", code).Info("bid served")
}
```

### Emergency debug window (30 min 10% sampling)

```go
stop := log4go.SetSamplingStrategyFor(
    log4go.TraceIDRatioBased{Ratio: 0.1}, 30*time.Minute)
defer stop() // or let it expire → reverts to 0.1%
```

### Ops snapshot (for Grafana / admin)

```go
st := log4go.Status()
// st.Sampling.Strategy == "trace_id_ratio:0.001"
// st.Writers[0].Name == "kafka_writer"
// st.Writers[0].Paused == false
// st.Writers[0].Metrics == WriterMetrics{Sent:..., Errored:..., Dropped:...}
```

### Pause a noisy writer during incident

```go
log4go.PauseWriter(log4go.WriterNameKafka) // drops records, connection stays
// ... investigate ...
log4go.ResumeWriter(log4go.WriterNameKafka)
```

---

## Kafka Codec (JSON + Protobuf) | Kafka 编解码

### Supported codecs (zero external dependency)

| Codec | Status | Size/rec | Deps | Use case |
|-------|--------|---------|------|----------|
| **JSON** | ✅ Done (default) | ~177B | zero | ES native, debug, human-readable |
| **Protobuf** | ✅ Done (hand-rolled) | ~95B (46% smaller) | zero | production, cross-language, 1M QPS bandwidth |
| Avro | ❌ Not doing | ~70B | +38% binary + Schema Registry | assessed: advantage insufficient vs cost |
| FlatBuffers | ⏸️ Future (if needed) | ~100B | codegen | gaming zero-copy consumer read |

### API

```go
// Default (JSON):
log4go.SetKafkaCodec(log4go.KafkaCodecJSON{})

// Production (Protobuf, 3× smaller):
log4go.SetKafkaCodec(log4go.KafkaCodecProto{})

// Runtime switch (RWMutex, rare config change):
writer := log4go.NewKafKaWriter(...)
writer.SetKafkaCodec(log4go.KafkaCodecProto{})

// Custom (interface open):
type KafkaCodec interface {
    Encode(p *kafkaPayload) []byte
    ContentType() string
}
```

### Protobuf schema (.proto)

File: `log4go/proto/log_record.proto`. Consumers generate decoders:

```bash
protoc --python_out=. log_record.proto   # Python
protoc --java_out=. log_record.proto     # Java
protoc --go_out=. log_record.proto       # Go
```

### Why Avro was rejected | 为什么不选 Avro

| Factor | Protobuf | Avro | Verdict |
|--------|---------|------|---------|
| Size | 95B | 70B | 25% diff — marginal at Kafka scale |
| Binary cost | 0 (hand-rolled) | +38% binary (hamba/avro ~8000 lines) | ❌ |
| Infrastructure | .proto in git (zero) | Schema Registry server (new ops) | ❌ |
| Spark/Flink | ✅ `from_protobuf()` (Spark 3.x) | ✅ native | tied |
| ES compatibility | needs decode (same as Avro) | needs decode | tied |
| Hand-roll possible | ✅ done (~158 lines) | ❌ too complex | ❌ |
| Schema evolution | ✅ field numbers + optional | ✅ stronger (named fields) | tied (logs rarely change schema) |

**Decision**: JSON + Protobuf only. Avro's complexity far exceeds its marginal benefit.
Available via open KafkaCodec interface if genuinely needed.

Avro 不做：25% 体积优势不值得 +38% binary + Schema Registry + 重依赖 + 无法手写。
Spark 3.x 已支持 Protobuf，Avro 的"原生大数据"优势消失。

### Performance (verified)

```
Benchmark_KafKaCodec_JSON-10    5831253    200 ns/op    384 B/op    2 allocs/op
Benchmark_KafKaCodec_Proto-10   4695116    264 ns/op    288 B/op    3 allocs/op
```

Proto is 46% smaller (95B vs 177B); slightly slower (264 vs 200 ns) due to `time.Format`
in timestamp. Optimization path: pre-compute ISO timestamp from unixNano without
`time.Unix().Format()` (saves ~60ns + 1 alloc).

### Gaming + Kafka analysis | 游戏 + Kafka 分析

| Game data type | Kafka? | Why |
|----------------|--------|-----|
| Real-time gameplay (frames) | ❌ | latency <10ms; use UDP/WebSocket |
| Match/room management | ❌ | strong consistency; use Redis |
| Player behavior logs | ✅ | event stream = log4go use case |
| Operations analytics (DAU) | ✅ | big-data analysis |
| Audit/compliance | ✅ | must persist |
| Anti-cheat streaming | ✅ | Flink CEP |

**Conclusion**: Game backends don't use Kafka for real-time gameplay (too slow), but DO
use it for operations/logs/analytics/audit — exactly log4go's domain. Game telemetry
through log4go is identical to ad-tech's pattern.

**FlatBuffers**: value is on consumer side (zero-copy selective field read), not producer
side. Relevant only if a game-event consumer needs to read billions of records selectively.
Not needed now; available via open KafkaCodec interface.

---

## Industry Alignment Summary | 业界对标汇总

| Concern | Industry standard | log4go implementation |
|---------|-----------------|----------------------|
| Deterministic sampling | OTel TraceIDRatioBased | `TraceIDRatioBased{Ratio}` |
| Cross-service consistency | W3C traceparent sampled flag | head-based via `WithContext` |
| Error protection | Stripe/Dapper/Netflix (keep errors) | `SetPriorityLevel(ERROR)` |
| Adjustable sampling rate | All big-tech (runtime-adjustable) | `SetSamplingStrategy` + `SetSamplingStrategyFor` |
| Per-service differentiation | Uber/ByteDance/Netflix | each service = independent logger + strategy |
| Sampling quality > quantity | Google Dapper (1% for P99) | `TraceIDRatioBased` + `PriorityLevel` |
| Metric for alerting | Prometheus/Grafana | `Metrics()` (Occurred/Written/Dropped) |
| Log for detail | structured + trace_id | `WithString/WithInt` typed fields + `WithContext` |
| Trace for chain | OpenTelemetry | `trace_id` correlation (OTel owns spans) |
| Field naming | ECS (flat snake_case) | `FieldServiceName`, `FieldESIndex`, etc. |
| Shutdown safety | idempotent + drain | quit channel + drainAndExit + Stopper |

---

## What log4go Does NOT Do (intentional) | log4go 不做的事（故意）

| Out of scope | Why | Use instead |
|--------------|-----|-------------|
| Distributed tracing | Separation of concerns | OpenTelemetry |
| Cross-service aggregation | Unrealistic in-process | Backend (ES/Loki/Tempo) |
| ES index routing | Consumer-side decision | Kafka consumer |
| Error-code semantics | Business domain | Service code |
| Big-data analysis | Not logging | Spark/Flink |
| Alert notification channels | Not logging | PagerDuty/Slack/DingTalk |
| Config file reload | Over-engineered (removed) | Granular setters (RuntimeConfig) |
| Batch field replace | Violates orthogonality (removed) | SetBaseField per key |
| Avro codec | Marginal size gain, heavy cost | Protobuf (or custom via KafkaCodec) |

---

## Document Index | 文档索引

| Document | Content |
|----------|---------|
| `docs/README.md` | **This document** — comprehensive evaluation + usage guide |
| `docs/design_principles.md` | 8 design principles + decision log + perf profile |
| `docs/observability.md` | Distributed observability design (OTel division, sampling, metrics) |
| `docs/observability_en.md` | Field schema + industry best practices + codec comparison (bilingual) |
| `doc.go` | Package-level godoc (API reference, examples) |
| `PERFORMANCE.en.md` | Benchmark methodology + historical results |
| `proto/log_record.proto` | Protobuf schema for Kafka cross-language consumers |

---

## Session Change Log (32 commits) | 本次改动记录（32 commits）

| Phase | Commits | Key deliverable |
|-------|---------|----------------|
| **100% coverage** | 41bf6eb | dead-code removal + caller-cache fix |
| **Shutdown safety** | 94e4f6e | auto-stop writers + idempotent Stop |
| **Reload removed** | 3f8d162 | over-engineered; granular setters instead |
| **RuntimeConfig** | 5f5eae1, 69864f0 | atomic hot-update (level/format/caller/sampling/trace) |
| **Race-free retirement** | 5f5eae1 | quit channel + enqueue select-on-quit |
| **Per-writer Pause/Resume** | 4e70574 | non-destructive + by-name control |
| **Sampling algorithms (P1a)** | c27a414 | OTel TraceIDRatioBased + TailDigit + Full |
| **Hot-path wiring (P1b-1)** | e1b75b2 | sampleDrop at WithContext + deliver check |
| **Temporary session (P1b-2)** | 78979dd | SetSamplingStrategyFor (auto-revert) |
| **Status snapshot (P1b-3)** | 7848c02 | strategy + writers + metrics |
| **Metrics funnel** | 29ba4c6 | Occurred/Written/Dropped + Written→bootstrap |
| **Quality gates** | 4223203 | alloc-budget CI gate + sampling benchmarks |
| **PriorityLevel** | cfaf06f | error-protection threshold (Stripe/Dapper) |
| **Base field refactor** | 880a108 | orthogonal 3-op (Set/Remove/Clear) |
| **Design principles** | 4cfff87 | 8 principles + decision log |
| **Performance profile** | 6362ec2 | pprof + bottleneck analysis |
| **KafkaCodec (JSON + Protobuf)** | 782c204 | pluggable codec + hand-rolled proto + .proto schema |
| **Codec decision (Avro rejected)** | a77a579 | JSON + Protobuf only; assessment matrix |

---

## Roadmap | 后续

| Item | Priority | Effort | Status |
|------|----------|--------|--------|
| `kit4go/kafka` wrapper (Producer/Consumer) | high | large | planned — seamless sarama/kafka-go/confluent switch |
| `OnErrorBurst(window, threshold, fn)` | low | small | optional (wraps RateAlerter) |
| Snapshot goroutine + bounded cache | low | medium | optional (periodic aggregation for Grafana) |
| Per-shard Occurred counter | low | small | ShardLogger integration |
| FlatBuffers codec | low | medium | future (gaming zero-copy consumer read); open interface |
