# log4go 性能与架构设计

> 英文版 (English): [PERFORMANCE.en.md](PERFORMANCE.en.md) ｜ 包用法：[doc.go](doc.go)(en) / [doc_zh.md](doc_zh.md)(中)
>
> 高性能、内存安全、可观测的 Go 日志库。本文件说明架构、各 writer 的实测吞吐与内存、
> 瓶颈与优化、以及线上（含广告行业 100K / 10M QPS）的配置方法。

## 1. 架构

```
 caller goroutine(s)                 single bootstrap goroutine
 ┌──────────────────┐                ┌──────────────────────────────────┐
 │ Debug/Info/...() │   deliver      │  for rec := range records {       │
 │  format + Caller ├────records────>│    for _, w := range writers {    │
 │  (level filter)  │     chan(4096)  │       w.Write(rec)  ← serial     │
 │  atomic counter  │                │    }                             │
 └──────────────────┘                │  } + flush/rotate timers         │
                                     └──────────────────────────────────┘
                                                │
                       ┌────────────┬───────────┴──────────┬───────────────┐
                       ▼            ▼                      ▼               ▼
                ConsoleWriter  FileWriter(bufio)     KafKaWriter      (your Writer)
                  fmt→stdout   WriteString→bufio    bounded chan→   impl Writer
                               (8192, flush 500ms)  AsyncProducer    interface
                                                    + overflow框架
```

要点：
- **调用方只做轻量活**：级别过滤、时间格式化、`runtime.Caller`、入 `records` channel（有界 4096）。
  实测 `deliver` pipeline 约 **1080 ns/op ≈ 923K QPS/核**（见 `Benchmark_LoggerInfo`）。
- **单 bootstrap goroutine 串行消费**：每条 record 依次调用所有注册 writer 的 `Write`。
  因此**端到端 QPS ≈ 1 / Σ(各 writer Write 耗时)**。慢 writer 会拖累全部。
- **OOM 防护**：`records` channel 有界；KafKaWriter 自带独立有界 channel + 多策略溢出框架。
  全程**绝不在调用方按 record 起 goroutine**（旧版 KafKaWriter 的 OOM 根因已消除）。

## 2. 各 Writer 实测吞吐与内存（单核，本机 Go 1.26）

| Writer / 路径 | ns/op | ~QPS/核 | B/op | allocs | 备注 |
|---|---|---|---|---|---|
| `deliver` pipeline（discard, caller+format） | 1065 | 939K | 48 | 3 | 调用方开销上限 |
| `Logger.Filtered`（被级别过滤） | 12 | 83M | 8 | 0 | 过滤几乎零成本 |
| ConsoleWriter（pipe→discard） | 1705 | 586K | 160 | 6 | 真实终端会慢 1~2 个数量级 |
| ConsoleWriter + Color | 1908 | 524K | 192 | 6 | ANSI 着色开销小 |
| FileWriter（bufio 8192） | 127 | **7.9M** | 96 | 1 | 写缓冲，定时 flush 落盘 |
| KafKaWriter.buildPayload（fast, 无字段） | 276 | 3.6M | 320 | 2 | 框架字段手动 append，零反射 |
| KafKaWriter.buildPayload（slow, ExtraFields） | 276 | 3.6M | 320 | 2 | 含 1 用户字段，仅值 marshal |
| KafKaWriter.buildPayload（base fields, 5 字段） | 432 | 2.3M | 768 | 2 | typed + 直接时间 |
| RingSpiller.Push（溢出兜底） | 10 | 100M | 0 | 0 | 内存环覆盖最旧 |
| FileSpiller.Push（落盘兜底） | 424 | 2.4M | 148 | 4 | 磁盘溢出 |

> 端到端 = 调用方 deliver + bootstrap 串行 writer。例如同时注册 File + Kafka：
> bootstrap ≈ 339(File) + ~100(Kafka 入队) ≈ 440 ns → 理论 ~2.2M QPS/核；再加 Console(stdout)
> 会被 stdout I/O 主导（终端下常降到 数千~数万 QPS）。

## 3. 瓶颈与优化

| 瓶颈 | 影响 | 处理 |
|---|---|---|
| **ConsoleWriter 同步写 stdout** | 终端 I/O 阻塞 bootstrap，拖垮所有 writer | 生产环境**禁用 Console**，仅本地调试 |
| **bootstrap 单 goroutine 串行** | writer 越多、越慢，端到端越低 | 生产只留 File + Kafka（都不阻塞 bootstrap） |
| **FileWriter bufio 8192** | 高频下频繁 auto-flush 落盘 | 可调大缓冲（见优化项） |
| **records channel 满** | 调用方阻塞（背压） | KafKaWriter 已自带 drop/spill；File+Kafka 通常不触达 |
| **每 record 起 goroutine**（旧 KafKaWriter） | goroutine 堆积 → OOM | **已修复**（零 per-record goroutine） |

**优化项（已落地 / 建议）：**
- ✓ KafKaWriter：AsyncProducer + 有界 channel + Drop/Block/Spill(内存环/落盘) 溢出框架。
- ✓ 所有数据组件：`Metrics()` 快照 + `SetOnEvent` 实时 hook（监控预留）。
- ✓ `Logger.writers` data race 用 `atomic.Value` 修复。
-  FileWriter：bufio 缓冲当前 8192，高频可调大（需改 `bufio.NewWriterSize`，预留接口）。
-  ConsoleWriter：仅开发用；生产走 File/Kafka。

## 4. 调参清单（均衡参数）

| 参数 | 位置 | 默认 | 建议范围 | 作用 |
|---|---|---|---|---|
| `recordChannelSize` | Logger 内部 | 4096 | 4096~65536 | records channel 容量，抗突发 |
| `BufferSize` | KafKaWriterOptions | 1024 | 8192~65536 | KafKaWriter 有界 channel |
| `OverflowPolicy` | KafKaWriterOptions | drop | drop/spill/block | 满载策略（见 §6） |
| `SpillType` | KafKaWriterOptions | ring | ring/file | 兜底存储类型 |
| `SpillSize` | KafKaWriterOptions | 1024 | 4096~65536 | 内存环容量（条） |
| `SpillMaxBytes` | KafKaWriterOptions | 64MB | 64MB~1GB | 落盘兜底字节上限 |
| `flushTimer` | Logger 内部 | 500ms | 200ms~1s | FileWriter bufio flush 间隔 |
| sarama `Producer.Flush` | cfg | 关 | 高吞吐可开 | 批量发送，提升 broker QPS |

## 5. 线上启用（代码）

```go
import "github.com/xwi88/kit4go/log4go"

// 全局初始化（一次）：
w := log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{Level: log4go.LevelFlagInfo})
log4go.Register(w)                                  // 本地调试用
fw := log4go.NewFileWriterWithOptions(log4go.FileWriterOptions{
    Filename: "/var/log/app-%Y%M%D.log", Rotate: true, Daily: true, MaxDays: 30,
})
log4go.Register(fw)                                 // 生产落盘

// 业务里直接调用（零 per-record goroutine）：
log4go.Info("bid req=%s", reqID)
```

KafKaWriter（高性能 + 防丢 + 防爆）：

```go
kw := log4go.NewKafKaWriter(log4go.KafKaWriterOptions{
    Brokers: []string{"kafka-1:9092"}, ProducerTopic: "app-log",
    BufferSize: 65536,
    OverflowPolicy: "spill", SpillType: "ring", SpillSize: 65536, // 满载兜底内存环
})
kw.SetOnEvent(func(name string, delta int64) { /* 接 Prometheus 等 */ })
log4go.Register(kw)
```

## 6. 场景配置（广告行业）

### 6.1 100K QPS（常规广告投放/竞价日志）
单实例可达（File 2.95M / Kafka buildPayload 1.0~3.5M 视字段数，余量充足）。
```go
// records channel 8192 抗突发；File daily rotate；Kafka spill 内存环兜底
log4go.SetLevel(log4go.INFO)        // 关 DEBUG 降量
fw := log4go.NewFileWriterWithOptions(log4go.FileWriterOptions{
    Filename: "/var/log/adx-%Y%M%D.log", Rotate: true, Daily: true, MaxDays: 14})
log4go.Register(fw)
kw := log4go.NewKafKaWriter(log4go.KafKaWriterOptions{
    Brokers: brokers, ProducerTopic: "adx-log", BufferSize: 32768,
    OverflowPolicy: "spill", SpillType: "ring", SpillSize: 32768})
log4go.Register(kw)
// 预期：调用方 ~1µs 返回；bootstrap File+Kafka 串行 ~0.5µs，channel 不堆积，零丢失。
```

### 6.2 10M QPS（超大规模，如全量曝光/点击流）
**单进程单 bootstrap 串行是瓶颈**（理论上限约 2~3M QPS/核）。10M 级别需要**水平分片**：

- **多 Logger 实例**（按业务线/分片 sharding），每实例独立 bootstrap，并行消费；
- 每实例目标 **100K~1M QPS**，配 `spill=file`（落盘兜底，抗 broker 抖动，不 OOM）：
  ```go
  kw := log4go.NewKafKaWriter(log4go.KafKaWriterOptions{
      Brokers: brokers, ProducerTopic: "imp-stream", BufferSize: 65536,
      OverflowPolicy: "spill", SpillType: "file",
      SpillDir: "/var/log/spill", SpillMaxBytes: 1 << 30}) // 1GB 兜底
  ```
- 关闭 Console；FileWriter 按分片独立文件；开启 sarama `Producer.Flush.Frequency`/`Messages` 批量；
- 部署 N 个 pod / 多机，Kafka 分区数 ≥ 并发度，端到端线性扩展。

> 关键：10M 不要堆在单进程。**分片 + 每 shard 一个 KafKaWriter + file 兜底** 是可线性扩展的架构。

## 7. 监控接入（预留接口）

```go
// 拉取式：周期采集快照
m := log4go.Metrics()                  // Logger 各级别计数
kwm := kw.Metrics()                    // KafKaWriter: Sent/Errored/Dropped/Spilled/Queued/SpillLen

// 推送式：实时事件 hook（不阻塞）
kw.SetOnEvent(func(name string, delta int64) {
    // name: "sent"/"error"；delta: 增量。转发到 Prometheus counter / statsd
})
```
建议采集：`log4go_records_total{level}`、`kafka_sent/dropped/spilled_total`、
`kafka_queued`（channel 深度，告警阈值 = BufferSize*80%）、`kafka_spill_len`。

## 8. 验证手段（本地，不依赖真实 Kafka / Postgres / 网络）

```bash
make test                      # go test -short -race ./...
make test-bench                # 各 writer / pipeline benchmark（本文表格数据来源）
go test ./log4go/ -bench . -benchmem -run '^$'
go test ./log4go/ -run '^Test_MemPerWriter$' -v   # 全 writer 100K 条内存占用（§13）
# 单 writer 独立进程跑（避免跨 benchmark daemon 干扰，本文 §12 数据源）：
#   go test ./log4go/ -bench '^Benchmark_FileWriter_AsyncDrop$' -benchmem -run '^$' -benchtime=2s
# 溢出框架：RingSpiller 10w push 后 Len 仍 = 容量（验证不 OOM）
# KafKaWriter：sarama mocks.AsyncProducer 端到端（无真实 broker，已知 record 数）
#              benchmark 用 noopAsyncProducer（b.N 未知，避免 mock expectation 匹配）
# NetWriter：进程内 TCP loopback（无跨机网络）
# IOWriter：bytes.Buffer / io.Discard（内存 sink）
```

## 9. 架构演进（并行 writer，按需）

当前单 bootstrap **串行**调用 writer，端到端受最慢 writer 限制。File/Kafka 已不阻塞
bootstrap（File 走 bufio、Kafka 走独立 channel+daemon），单实例数十万 QPS 足够。
若需单实例数百万 QPS，可演进为 **fan-out 并行**：

- `deliver` 向每个 writer 的**独立 channel 广播**；每个 writer 自带 goroutine 消费；
- 权衡：丢失跨 writer 全局顺序、每 writer 一份 buffer（内存↑）、背压/溢出策略需各 writer 独立；
- 范式：KafKaWriter 已是"慢 writer 异步化"的样板（独立 channel + daemon + 溢出框架），
  可照此给 FileWriter 增加异步层（channel + flush goroutine + spill 兜底）；
- 建议：作为独立大改动评估并充分 `-race` 测试。

> 实践中：**先用分片（§6.2）横向扩展**，通常比单进程 fan-out 更简单、更可靠、更易线性扩展。

## 10. 溢出子系统（泛型复用 + 多级 + 恢复 + 告警）

溢出框架已**泛型化**（`Spiller[T]`/`RingSpiller[T]`/`FileSpiller[T]` + `SpillCodec[T]`），
KafKaWriter 与 FileWriter **共享一份逻辑**（消除重复 ring，避免 N 倍空间）。提供
`ProducerMsgCodec` / `RecordCodec`。

- **多级** `ChainedSpiller[T]`：ring(热内存) → file(冷磁盘) → drop。ring 用
  `PushNoOverwrite`，满才溢出到 file（不覆盖丢热数据）；file 满（MaxBytes）才 drop。
  **总空间硬上限 = ringCap + fileMaxBytes**，不会 OOM/飙升。
- **持续溢出告警**：`OverflowStats` 在首次 + 每 N 次（默认 1000，`SetAlertEvery` 可配）
  经标准 logger 打 `[log4go] overflow DROP/SPILL ...`（不刷屏、不递归）。运维据此发现
  持续溢出 → 扩容/限流/反压（`OverflowBlock`）。
- **中断恢复**：file spill 持久化（`spill.log`），KafKaWriter/FileWriter `Start` 时
  Drain → 重投，**从中断处继续**（内存 ring 不参与恢复）。见
  `Test_FileSpiller_RecoverAcrossInstances`。
- **配置**：`OverflowPolicy` = drop/block/spill；spill 下 `SpillType` = ring/file/chain
  （默认 chain = ring→file），`SpillSize`/`SpillDir`/`SpillMaxBytes` 控制各级上限。

## 11. 性能压测与调参建议（Go 1.26.0 / Apple M5 10 核 GOMAXPROCS=10）

> 压测环境：Go 1.26.0，darwin/arm64（Apple M5），NumCPU=10，GOMAXPROCS=10。
> ns/op 为每条日志耗时，B/op 为每条分配内存，QPS=1e9/ns。多核 ShardLogger 的 ns/op 为并行 wall/total（已含多核并行）。
> 数据来源：`go test ./log4go/ -bench . -benchmem -run '^$' -benchtime=2s` + `Test_MemSustained_1M`。

### 单线程极限
| 模式 | ns/op | ~QPS/核 | 内存 B/op | alloc | 说明 |
|---|---|---|---|---|---|
| caller（file:line）| 1580 | ~633K | 336 | 7 | 默认，保定位 |
| no-caller（`WithCaller(false)`）| 1017 | ~983K | **16** | **1** | 极致（失 file:line）|
| filtered | 11.5 | ~87M | 8 | 0 | 级别过滤近零成本 |

caller 路径受 `runtime.Caller` 开销主导；no-caller 仅 1 次 alloc、~983K/核，逼近 1M/核物理极限。

### 多核 ShardLogger（10 核，并行 wall/total）
| shard | ns/op | ~QPS | 内存 B/op | 备注 |
|---|---|---|---|---|
| 1 | 1752 | 571K | 289 | 退化单 logger |
| 2 | 1028 | 973K | 292 | |
| **4** | **614** | **~1.63M** | 303 | **最佳（≈ 核数/2）** |
| 8 | 714 | 1.40M | 306 | |
| 16 | 1565 | 639K | 310 | 超核数 1.6×，调度反噬 |

**shard ≈ 核数/2~核数最佳**；超过核数调度反噬。本机 10 核下 4 shard 达 ~1.63M，已稳定破 1M。

### Writer 吞吐（单核，单 writer 独立进程实测）
| writer | ns/op | ~QPS | 内存 B/op |
|---|---|---|---|
| File（sync bufio）| 140 | ~7.1M | 80 |
| Console（pipe→discard）| 1620 | ~617K | 96 |

File（bufio 缓冲）最快；Console 受 stdout I/O 限制，生产建议禁用。**全 writer 实测对比（含 async/Kafka/Net/IO/JSON）见 §12**。

### 进程内存实测（1M 条日志，MemSustained_1M，Go 1.26.0，10 核）
| 指标 | 值 | 说明 |
|---|---|---|
| Sys | 19.0 MB | 进程系统内存（含 Go runtime/栈）|
| HeapAlloc | 0.8 MB | 1M 条日志堆分配（pool 复用，极低）|
| HeapInuse | 1.7 MB | 堆使用 |
| NumGC | 6 | 1M 条仅 6 次 GC（pool 复用减 GC）|
| Goroutines | 3 | bootstrap + writers，不随 QPS 增长 |

高 QPS 下内存**不飙升**：records channel 有界 + Record/bufPool 复用 + spiller 有界。实测 1M 条仅 0.8MB 堆、6 次 GC、3 个常驻 goroutine。

### 建议参数
| 场景 | 配置 | 预期 |
|---|---|---|
| 生产（保定位）| 单 logger caller，`recordChannelSize` 8192，level INFO | ~633K/核 |
| 极致（舍定位）| 单 logger `WithCaller(false)` | ~983K/核 |
| 多核扩展 | `ShardLogger(≈GOMAXPROCS/2)` | 10 核 4 shard ~1.63M |

### 突破 1000K
单线程物理极限 ~983K（no-caller）。**多核 `ShardLogger`（shard≈核数/2）是唯一正路**，干净环境线性扩展可超 1M。防丢用 overflow `spill`（ring→file），防 OOM 用 buffer/spiller 有界。

> 注：上表为本机（M5 / Go 1.26.0）实测值；CI/生产干净环境数值可能更高。完整压测见
> `Benchmark_LoggerInfo` / `Benchmark_LoggerInfoNoCaller` / `Benchmark_ShardLoggerScale` / `Test_MemSustained_1M`。
> **全 writer 实测表见 §12，内存占用见 §13，场景选型见 §14，业界对比见 §15**。

## 12. 结构化能力（字段 / 采样 / context / JSON 格式）

本节覆盖对标 zap/zerolog 的结构化能力，及其性能开销。

### 12.1 结构化字段（With / WithField / WithFields）

`Logger.With("k", v)` 返回携带字段的子 logger，链式调用累加字段；`Record.String()` 在文本格式下追加 JSON 字段对象，`KafKaWriter` 提升到 JSON 顶层。**无 With 的热路径零开销**（`Record.String` 仅在 `len(r.fields)>0` 时追加）。

### 12.2 采样（WithSampling）

`Logger.WithSampling(initial, thereafter)` — 前 `initial` 条全记，之后每 `thereafter` 条记 1 条。按 level 独立计数（atomic，热路径无锁）。**被采样丢弃的 record 不计入 Metrics**（在 Metrics 增加之前 return），避免监控误报写入速率。

### 12.3 context.Context（zerolog 风格）

- `Logger.IntoContext(ctx)` / `FromContext(ctx)` — 把 logger 绑到 ctx，handler 内 `log4go.FromContext(r.Context())` 自动带 requestID/traceID（zerolog 模式）
- `AddContextExtractor(fn)` — 叠加多个 extractor（trace/span/requestID/uid/baggage，last-writer-wins），无硬 otel 依赖
- `RequestIDMiddleware` — HTTP 中间件，从 header 取/生成 requestID，绑定带字段的 logger
- `WithContext(ctx)` — 提取字段到子 logger（默认探测扩展 key 集：trace_id/span_id/request_id/user_id/tenant_id 等）

### 12.4 JSON 格式（FormatJSON，对标 zap/zerolog 完整标配）

`SetFormat(FormatJSON)` → 每条 record 输出一个 JSON 对象：`{"time":"...","level":"INFO","msg":"...","file":"...","fields":{...}}`。格式在 `deliverRecordToWriter` 决定一次并缓存到 `r.jsonBytes`，**所有 writer 直接输出预序列化字节**（不重复序列化）。time 用 `timestampLayout`（ISO，带时区），fields 为空时省略。

### 12.5 JSON 编解码器对比（Go 1.26.0 / Apple M5 10 核）

`SetJSONCodec(JSONCodecGoccy|JSONCodecStd|JSONCodecSonic)` 切换编码器。默认 **Goccy**（最快可移植，已依赖）。实测（含 3 个字段的 record）：

| 编解码器 | ns/op | B/op | allocs | 说明 |
|---|---|---|---|---|
| **Goccy（默认）** | **792** | 993 | 6 | 最快可移植，无 cgo |
| Std（encoding/json）| 1028 | 1120 | 12 | 标准库，兼容性完整（慢 ~30%，allocs 2x）|
| Sonic（bytedance）| 1260 | 1039 | 7 | amd64 SIMD 最快；arm64 有 fallback 开销 |
| JSON 无字段（Goccy）| 205 | 400 | 3 | 常见无 With 路径，极快 |

JSON 目标 ≤ 1500ns/op 达成（Goccy 792ns）。JSON 比 text（~291ns File）慢约 2-3x（json.Marshal 开销），但仍是亚微秒级，对绝大多数场景可接受。压测见 `Benchmark_Record_JSON_*`。

## 12. 各 Writer 实测吞吐（全 writer，Go 1.26.0 / Apple M5 10 核）

> 压测环境：Go 1.26.0，darwin/arm64（Apple M5），NumCPU=10，GOMAXPROCS=10。
> 每个 writer 单独跑 `go test ./log4go/ -bench '^Benchmark_<Writer>_$' -benchmem -run '^$' -benchtime=2s`（单 writer 独立进程，避免跨 benchmark 的 daemon/goroutine 互相干扰）。
> 接收端均为 I/O 噪音隔离：Console→pipe(discard)、File→tmpfile、Kafka→noopAsyncProducer（不连真 broker）、Net→进程内 TCP(loopback)、IO→bytes.Buffer/io.Discard。
> ns/op = 每条日志耗时，B/op = 每条分配内存，~QPS = 1e9/ns/op（单核）。async writer 的 ns/op 是**调用方 Write 返回耗时**（daemon 异步落盘/发网络不计入）。

| Writer | ns/op | ~QPS/核 | B/op | allocs | 场景推荐 |
|---|---|---|---|---|---|
| **File（sync bufio）** | **140** | **~7.1M** | 80 | 1 | 单核最快落盘（同步 bufio，无 daemon）|
| File（async drop） | 219 | ~4.6M | 128 | 1 | 生产高吞吐落盘（daemon 异步，drop 兜底）|
| File（async spill） | 217 | ~4.6M | 127 | 1 | 生产落盘 + ring 兜底（抗突发不丢热数据）|
| **NetWriter（TCP，进程内 loopback）** | **137** | **~7.3M** | 114 | 1 | 远程收集（调用方仅入队；真实网络受 RTT 限制，见下）|
| Console（buffered） | 294 | ~3.4M | 80 | 1 | 容器 stdout 收集（bufio 减 syscall）|
| IOWriter（io.Discard） | 204 | ~4.9M | 96 | 2 | 测试/自定义最快 sink |
| IOWriter（bytes.Buffer） | 649 | ~1.5M | 396 | 2 | 测试捕获（buffer 自身扩容开销）|
| Console（unbuffered） | 1620 | ~617K | 96 | 2 | 开发调试（每条 syscall）|
| Console（unbuffered + color） | 2961 | ~338K | 192 | 6 | 开发调试 + ANSI 着色 |
| **Kafka（noop producer）** | **831** | **~1.2M** | 496 | 5 | 生产发 kafka（buildPayload 手动 append 主导，见下）|
| Record.JSON（goccy，3 字段） | 1021 | ~980K | 993 | 6 | JSON 结构化（默认 codec，全 writer 通用）|
| Record.JSON（std） | 1049 | ~953K | 1120 | 12 | 标准库兼容（慢 ~3%，allocs 2x）|
| Record.JSON（sonic） | 1359 | ~736K | 1038 | 7 | amd64 SIMD 最快；arm64 fallback 有开销 |
| Record.JSON（goccy，无字段） | 250 | ~4.0M | 400 | 3 | 常见无 With 路径，极快 |
| Record.String（text，3 字段） | 1685 | ~594K | 769 | 7 | 文本格式化基线（含字段 JSON 对象）|

> **端到端** = 调用方 deliver（~1085ns caller / ~1049ns no-caller）+ bootstrap 串行 writer。生产只注册 File + Kafka：bootstrap ≈ 219(File async) + ~100(Kafka 入队) ≈ 320ns → 理论 ~3M QPS/核；JSON 格式额外 +1021ns（goccy，一次性序列化缓存到 r.jsonBytes，多 writer 不重复）。

### Kafka 为何慢于 File

Kafka `Write` 热路径 = `buildPayload`（288ns 无字段 / 522ns 含 1 字段 / 1006ns 含 5 个 base field，手动 byte append，零 map 中间层）+ 入队（~100ns）+ ProducerMessage 分配。对比 File async（219ns，仅 copy record + 入队），Kafka 慢约 4x，主因是每条 record 必须序列化为 JSON（File 文本路径无需）。早期版本经 `map[string]interface{}` 中间层 + `json.Marshal` 达 2582ns/27 allocs，现已改为框架字段直接 append、仅用户字段值单独 marshal；慢路径（含 Base Field 的 Kafka→ES 典型场景）回落到 ~500ns 级。进一步方向：对字符串类用户字段走快速 escape-append，省去 per-field `Marshal` 调用。

### NetWriter 真实网络吞吐

上表 NetWriter 137ns 是**进程内 loopback**（调用方仅入队，daemon 异步写 loopback TCP）。真实跨机网络受 RTT 限制：

| 部署 | 单连接预估 QPS | 说明 |
|---|---|---|
| 同主机 loopback | ~1M-7M | 上表实测（仅入队开销）|
| 同机房 TCP | ~50K-200K | RTT ~0.1-1ms 主导 |
| 跨机房 / WAN | ~5K-50K | RTT ~10-100ms 主导 |

> NetWriter 强制 async + 有界 channel + drop/spill：网络抖动**永不阻塞调用方**（业务流转不受影响），channel 满则 drop（默认，推荐）或 spill（ring 缓冲恢复后重发）。生产高吞吐用 File + Kafka，NetWriter 仅低量收集。

### 防阻塞设计（async writer 通用）

- **强制 async**：File(async) / Kafka / Net 均独立 daemon goroutine + 有界 channel。`Write` 在 drop/spill 下**永不阻塞调用方**。
- **OverflowPolicy（drop/spill/block）**：channel 满 → drop（默认）/ spill（ring→file 多级兜底）/ block（背压）。复用泛型溢出框架 `Spiller[T]`（见 §10）。
- **连接超时 + 重连退避**（NetWriter）：`Timeout`（默认 3s）设 `SetWriteDeadline`；写失败关连接，下条 record lazy 重连（`ReconnectBackoff` 默认 1s）。
- **JSON/Text 自适应**：`Logger.format=FormatJSON` 时 File/Console/Net/IO 直接发 `r.jsonBytes`（零额外序列化）；Kafka buildPayload 仍独立序列化（兼容其顶层 hoist 语义）。

### 12.6 多目的地告警（WebhookWriter + RateAlerter）

一个 logger 可注册多个 writer，**各自带 level 过滤**，实现多目的地不同级别分发（对标 zap `Tee` + logrus `Hook`）：

| 目的地 | 级别 | 说明 |
|---|---|---|
| KafKaWriter | INFO+ | 全量 → Kafka→ES（buildPayload 手动 append，见 §12） |
| NetWriter | WARN+ | 热线 → TCP/UDP 收集器 |
| WebhookWriter | ERROR+ | 仅严重错误 → lark/dingtalk/wechat |

**WebhookWriter** 把 `AlertSink`（`WebhookAlertSink`，已带异步/重试/限流）包成 Writer，三层可组合触发：
- **级别**（始终）：`r.level <= w.level` 才转发；
- **Filter**（可选）：`func(*Record) bool` 谓词，关键字/字段命中才发；
- **Gate**（可选）：`*RateAlerter` 阈值门控，超阈值才放行（防风暴/阈值汇总）。

实测热路径（mock sink，Apple M5）：

| 场景 | ns/op | allocs | 说明 |
|---|---|---|---|
| PassThrough（级别+filter+gate 全过 → Send） | 71 | 0 | gate.Allow() 占大头（一次锁） |
| LevelSkip（级别不过，快速跳过） | 3.7 | 0 | 仅一次整数比较 |

零分配，不成为吞吐瓶颈。`Logger.Close()` 经 `io.Closer` 路径自动关闭 WebhookWriter 的 sink daemon，`defer log4go.Close()` 一次清理全部。

**RateAlerter**：秒级桶环形数组滑动窗口，`Allow()` O(1) 摊销、无 per-event 分配，可安全挂热路径。`NewRateAlerter(window, threshold)` + `SetCooldown`。

## 13. 各 Writer 内存占用（100K 条，MemPerWriter，Go 1.26.0）

> 数据来源：`Test_MemPerWriter`（每个 writer 直接 Write 100K 条，前后各一次 GC，取 HeapAlloc/HeapInuse delta）。接收端 I/O 噪音隔离（同 §12）。**所有 writer 100K 条 HeapAlloc < 0.05MB**——records/channel 有界 + Record 池复用 + spiller 有界，高 QPS 下内存不飙升。

| Writer | 100K 条 HeapAlloc(MB) | HeapInuse(MB) | NumGC | Goroutines | 说明 |
|---|---|---|---|---|---|
| discard（基线） | 0.00 | 0.00 | 1 | 4 | 无 writer 开销 |
| Console（buffered） | 0.00 | 0.00 | 3 | 4 | bufio 缓冲，flush timer |
| File（async drop） | 0.00 | 0.00 | 5 | 4 | daemon + 有界 channel |
| File（async spill） | 0.00 | 0.00 | 5 | 4 | daemon + ring 兜底 |
| **Kafka（noop producer）** | 0.00 | 0.00 | **14** | 4 | buildPayload 手动 append（5 allocs，旧 map 路径 27）|
| NetWriter（TCP loopback） | 0.00 | 0.00 | 6 | 4 | daemon + 有界 channel |
| IOWriter（bytes.Buffer） | 0.00 | 0.04 | 5 | 4 | buffer 自身扩容（sink 持有，非 writer）|

**要点**：
- **HeapAlloc 全部 < 0.05MB**：100K 条日志几乎零堆增长。Record 池复用 + records/channel 有界（4096）+ async writer 的 messages channel 有界 + spiller 有界 = OOM 防护闭环。
- **Goroutines 恒为 4**（bootstrap + 各 daemon），不随 QPS 增长——零 per-record goroutine（旧 KafKaWriter 的 OOM 根因已消除）。
- **Kafka NumGC=14**：buildPayload 框架字段手动 byte append（5 allocs/record，旧 `map`+`json.Marshal` 路径 27），100K 条 GC 次数从 55 降到 14。FormatJSON 预序列化（r.jsonBytes）可进一步降低多 writer 场景的重复序列化（见 §12）。
- **IOWriter HeapInuse=0.04MB**：bytes.Buffer 作为 sink 持有全部 100K 条输出文本（非 writer 自身开销，是 sink 特性）。

> 对比 §11 的 `Test_MemSustained_1M`（1M 条 discard，HeapAlloc=0.8MB / NumGC=6 / Goroutines=3）：100K 条是 1M 的 1/10，内存仍线性可控。Kafka 因 JSON 序列化 GC 更频，但 HeapAlloc 仍近零（GC 及时回收）。

## 14. 场景选型指南

| 场景 | 推荐 writer | 配置 | 预期 QPS（单核）| 说明 |
|---|---|---|---|---|
| **本地开发** | Console（unbuffered）| 默认 | ~600K | 即时可见，每条 syscall 可接受 |
| **生产落盘** | File（async + drop）| `Async:true, AsyncBufferSize:1<<14, OverflowPolicy:"drop"` | ~4.6M | daemon 异步，调用方仅入队 |
| **生产落盘（抗突发）** | File（async + spill）| `Async:true, OverflowPolicy:"spill", SpillType:"ring", SpillSize:1<<14` | ~4.6M | ring 兜底不丢热数据 |
| **生产发 kafka** | Kafka（async + spill）| `BufferSize:1<<14, OverflowPolicy:"spill", SpillType:"ring"` | ~387K | JSON buildPayload 主导；FormatJSON 预序列化可加速 |
| **容器 stdout 收集** | Console（buffered）| `Buffered:true`（bufio 4KB）| ~3.4M | bufio 减 syscall，flush timer 落盘 |
| **远程日志收集** | NetWriter（async drop）| `Network:"tcp", BufferSize:1<<14, OverflowPolicy:"drop", Timeout:3s` | ~50K-200K（真实网络）| loopback ~7M；跨机受 RTT 限制 |
| **结构化 JSON** | SetFormat(JSON) + goccy | 全 writer 通用（r.jsonBytes 预序列化）| ~980K（序列化）/ 叠加 writer | 默认 codec 最快可移植 |
| **多核高吞吐** | ShardLogger(核数/2) + File | `NewShardLogger(GOMAXPROCS/2)` + `RegisterFunc` 每 shard 独立 FileWriter | ~1.6M+（10 核 4 shard）| 唯一突破单核 ~1M 的正路 |
| **测试/自定义** | IOWriter(bytes.Buffer / io.Discard) | `NewIOWriter(w, level)` | ~1.5M-4.9M | 最薄 adapter，捕获/直写 |
| **极致（舍定位）** | no-caller + File(async) | `WithCaller(false)` | ~4.6M（writer）+ ~1049ns（deliver）| 失 file:line，16 B/op 1 alloc |
| **低量 + 告警推送** | Kafka(spill=file) + WebhookAlertSink | `SpillType:"file", SpillMaxBytes:1<<30` | ~387K | 落盘兜底 + 溢出告警（lark/dingtalk）|

### 选型决策树

```
需要落盘？ ───────────────── YES ──→ File（async + drop 抗突发用 spill）
   │ NO                           │ 生产高吞吐：ShardLogger(核数/2) + RegisterFunc（每 shard 独立 File）
   ↓                              
需要发 kafka？ ──────── YES ──→ Kafka（async + spill=ring/file）
   │ NO                           
   ↓                              
容器 stdout 收集？ ── YES ──→ Console（buffered）
   │ NO                           
   ↓                              
远程收集（低量）？ ── YES ──→ NetWriter（async drop）
   │ NO                           
   ↓                              
测试/自定义 sink ──────────→ IOWriter
```

### 关键约束

1. **生产禁用 Console（unbuffered）**：同步写 stdout 阻塞 bootstrap，拖垮所有注册 writer。仅本地调试用。
2. **bootstrap 单 goroutine 串行**：注册 writer 越多、越慢，端到端越低（端到端 ≈ Σ 各 writer Write）。生产只留 File + Kafka。
3. **ShardLogger(n>1) 不能共享 FileWriter**：用 `RegisterFunc` 每 shard 建独立实例（否则 bufio/file/daemon 跨 shard 竞争损坏输出，Register 会 panic）。
4. **NetWriter 仅低量**：真实网络吞吐受 RTT 限制（~50K-200K 同机房），高吞吐必须 File + Kafka。

## 15. 业界对比（vs zap / zerolog / logrus）

| 能力 | log4go | zap（uber）| zerolog（rs）| logrus | 说明 |
|---|---|---|---|---|---|
| **结构化字段**（With/WithField） | ✓ | ✓（ SugaredLogger）| ✓（Context）| ✓（Fields）| log4go: 子 logger 链式累加，热路径零开销（无 With 不付费）|
| **JSON 格式** | ✓ goccy/std/sonic 可切换 | ✓（强项，零反射）| ✓（强项，零 alloc）| ✓（encoding/json）| log4go 默认 goccy ~980K/s；zap/zerolog ~数百万/s（零反射优势）|
| **采样（Sampling）** | ✓ WithSampling(initial, thereafter) | ✓（core level）| ✓（Sampling）|  | log4go 按 level 独立 atomic 计数，采样丢弃不计 Metrics |
| **context.Context** | ✓ IntoContext/FromContext + RequestIDMiddleware | （需 helper）| ✓（zerolog.Ctx）|  | log4go 无硬 otel 依赖，AddContextExtractor 可叠加 trace/baggage |
| **日志旋转（Rotate）** | ✓ 内置 daily/hourly/minutely + MaxDays/MaxHours | （需 lumberjack）| （需 lumberjack）| （需 lumberjack）| log4go 内置，无需第三方 |
| **多 sink 并发**（多 writer）| ✓ Register 多 writer（bootstrap 串行）| ✓（core chain）| （Multi-level）| ✓（MultiHook）| log4go: 串行保全局顺序；慢 writer 拖累全部（建议 async writer）|
| **Kafka writer** | ✓ 内置（AsyncProducer + 溢出框架）| （需自建）| （需自建）| （需自建）| log4go 内置 drop/block/spill(ring/file/chain) 防 OOM |
| **NetWriter（TCP/UDP）** | ✓ 内置 async + 溢出 | （需自建）| （需自建）| （需自建）| log4go 内置，async + 有界 + drop/spill 防阻塞 |
| **溢出/防 OOM**（drop/spill/block）| ✓ 泛型 Spiller[T]（ring→file→drop 多级）|  |  |  | log4go 独有：channel 满 ring 兜底 → file 兜底 → drop，总空间硬上限 |
| **多核分片**（ShardLogger）| ✓ NewShardLogger(n) round-robin | （多实例自管）| （多实例自管）|  | log4go: 10 核 4 shard ~1.6M QPS，唯一突破单核 ~1M 正路 |
| **告警钩子**（SetOnEvent / AlertSink）| ✓ 实时事件 + webhook（lark/dingtalk/feishu）| （hooks 弱）|  | ✓（Hook）| log4go: 溢出首次 + 每 N 次告警，不刷屏不递归 |
| **崩溃恢复**（file spill drain）| ✓ 启动 Drain 重投 |  |  |  | log4go: 中断后从 spill.log 续投（ring 不参与恢复）|
| **Metrics** | ✓ 各级别计数 + writer 运行指标 | （需自建）|  |  | log4go: Logger.recordsByLevel + FileWriter/KafKaWriter/NetWriter Metrics 快照 |
| **吞吐（单核，JSON）** | ~980K（goccy）/ ~7.1M（File text）| ~数百万/s | ~数百万/s（零 alloc）| ~100K-300K | zap/zerolog 零反射 JSON 最快；log4go goccy 次之；logrus 最慢 |
| **吞吐（多核）** | ~1.6M（4 shard）| 需自管多实例 | 需自管多实例 | — | log4go ShardLogger 内置分片，线性扩展 |
| **内存（100K 条）** | < 0.05MB HeapAlloc / 恒 4 goroutine | ~零 alloc（zerolog 更优）| ~零 alloc | 较高 | log4go 池复用 + 有界 channel，OOM 防护闭环 |
| **依赖** | goccy/go-json（可选 sonic）| 仅 zapcore | 零依赖 | 多依赖 | log4go 默认 goccy（可移植），sonic 仅 amd64 最优 |

### 选型总结

- **选 zap/zerolog**：纯追求极致 JSON 吞吐 + 零 alloc，且不介意自建 rotate/kafka/net/溢出。
- **选 log4go**：需要**开箱即用**的完整生产栈（rotate + kafka + net + 溢出防 OOM + 多核分片 + 告警 + 崩溃恢复），且吞吐够用（单核 File ~7M / JSON ~980K，多核 4 shard ~1.6M）。
- **选 logrus**：已用且性能不敏感（吞吐最低，~100K-300K）。

> log4go 的**差异化优势**是内置的**溢出防 OOM 框架**（ring→file→drop 多级 + 告警 + 崩溃恢复）和**多核 ShardLogger**——这两项 zap/zerolog 均需自建且易踩 OOM/竞争坑。

## 16. Round A 升级：类型化字段 + slog + logfmt + 预设 + Panic/Fatal

对标 zap.Field / slog.Attr 的**类型化字段**（零装箱）+ 标准库 slog 接入 + logfmt + 生产/开发预设 + Panic/Fatal/Recover。性能对比（Apple M5，基线 = dev `67d62a7`，改造后）：

| Benchmark | 基线 ns / allocs | 改造后 ns / allocs | Δ |
|---|---|---|---|
| Record.JSON（3 字段） | 5801 / 6 | **494 / 2** | **−91% / −67%** |
| DeliverPipeline_NoCaller | 7054 / 1 | 1160 / 1 | −84% |
| DeliverPipeline_Discard | 1835 / 7 | 1090 / 7 | −41% |
| Record.JSON（无字段） | 469 / 3 | 372 / 2 | −21% / −33% |
| Kafka buildPayload（1 字段） | 573 / 3 | 570 / 3 | ≈ |
| Kafka buildPayload（5 base fields） | 1032 / 8 | 1014 / **3** | −2% / **−63%** |
| Logger.With（int, interface） | — | 223 / 4 | 基线 |
| Logger.WithInt（typed） | — | **125 / 3** | **−44% / −1 alloc vs With** |

**关键收益**：`Record.JSON` −91%（typed append 取代 map+反射+装箱，6→2 allocs）；typed `WithInt` 比接口版快 44% 且少 1 装箱 alloc；Kafka base-fields 路径 allocs −63%。

### 新增能力
- **类型化字段**：`Field` + `String/Int/Int64/Uint64/Bool/Float64/Duration/Time/ErrorField/Any` 构造器；`Logger.WithString/WithInt/.../WithAttrs`（实例+包级，零装箱）；旧 `With(key, interface{})` 内部 `fieldOf` 类型推断，常见标量自动零装箱，完全向后兼容。
- **logfmt**：`SetFormat(FormatLogfmt)` → `time=... level=... msg="..." key=val`，Loki/Promtail/docker 原生格式；`r.jsonBytes` 改名 `r.formattedBytes`（格式无关，JSON/logfmt 共用预序列化）。
- **slog.Handler**：`NewSlogHandler(logger)` 让标准库 slog（net/http、第三方库）日志进 log4go 管线（writer/溢出/告警/监控）。`slog.SetDefault(slog.New(log4go.NewSlogHandler(...)))`。
- **预设**：`NewProduction()`（JSON+INFO+采样+caller）/ `NewDevelopment()`（彩色 Text+DEBUG+funcname），对标 zap。
- **Panic/Fatal/Recover**：CRITICAL 输出 → `Sync()`（drain+flush）→ panic/exit；`Recover` 捕获 panic→日志（可挂 WebhookWriter sentry 风格）→ re-raise。

## 17. 类型覆盖与健壮性（对标 zap/zerolog/slog）

### 17.1 字段类型覆盖（fieldOf 自动推断 + typed 构造器）

| 类型 | 推断路径 | JSON 输出 | logfmt 输出 |
|---|---|---|---|
| string / bool | typed（零装箱） | 原生 | 原生/引号 |
| int / int8-64 | typed | 整数 | 整数 |
| uint / uint8-64 / uintptr | typed | 整数 | 整数 |
| float32 / float64 | typed | 小数（NaN/Inf→null） | 小数（NaN/Inf→`-`） |
| complex64 / 128 | typed→string | `"a+bi"`（NaN→null） | `a+bi` |
| time.Duration | typed | 纳秒整数 | 纳秒整数 |
| time.Time | typed | ISO 字符串 | ISO 字符串 |
| []byte | typed（base64） | base64 字符串 | base64 |
| error | typed | `.Error()` 字符串（typed-nil 安全） | 同 |
| 其他（struct/map/slice/chan/func…） | kindAny | codec marshal，失败→null | 失败→`-` |

### 17.2 健壮性保证（任何字段值都不击穿日志管线）

| 风险 | 处理 | 测试 |
|---|---|---|
| 自定义 `MarshalJSON` panic | `safeJSONMarshal` defer recover → null | Test_Field_AnyMarshalPanic |
| typed-nil error 接收者 panic | `safeErrorString` recover → null | Test_Field_TypedNilErrorDegrades |
| chan / func 不可 marshal | kindAny 失败 → null | Test_Field_UnmarshallableKinds |
| NaN / ±Inf（非法 JSON） | 位掩码检测 → null | Test_Field_NaNInfIsValidJSON |
| complex NaN 分量 | → null | Test_Field_Complex |

> 性能取舍：recover/检查只挂在 kindAny / kindError（慢路径）；标量热路径零额外开销；float 的 NaN/Inf 用**单次位掩码 AND**（比 IsNaN+IsInf 更快）。

### 17.3 Round A 新增路径实测（Apple M5，typed + 时间格式 + 批量转义）

> 三轮优化叠加：① typed append（消除 map/反射/装箱）② `appendISOTimeUTC`（消除 `time.Format` string 分配）③ 批量字符串转义（clean run 一次 append，取代逐字节）。JSON/logfmt 全部降到 **1 alloc**（只剩 buf 本身）。
>
>  **可信度说明**：本机 M5 长跑有热降频，ns/op 在多次 `-count` 间可漂移 2–5×（如 Record.Logfmt 267→2940）。**allocs/op 是热无关的稳定信号**（下表 allocs 列在所有重复中恒定）；ns 取**冷机隔离**最优值，作峰值吞吐参考。

| Benchmark | 峰值 ns/op | allocs | 说明 |
|---|---|---|---|
| **Record.Logfmt（3 字段）** | **~229** | **1** | typed append + 零 string alloc |
| **Record.JSON（3 字段）** | ~210–350 | **1** | typed + 批量转义（基线 5801/6，**−95% / allocs −83%**） |
| Record.JSON（无字段） | ~330 | 1 | |
| Kafka buildPayload（5 base fields） | ~560 | 2 | typed + 直接时间格式 |
| Kafka buildPayload（1 字段, ExtraFields） | ~540–1000 | 2 | 噪声大（map 路径）；typed 主路径更稳 |
| SlogHandler.Handle | ~1282 | 7 | slog 桥接（+19% vs 原生） |
| Logger.WithAttrs（3 typed） | ~160 | 3 | 零标量装箱 |
| Logger.WithInt（typed） vs With（interface） | ~118 vs ~136 | 3 vs 3 | typed 略快（Go 优化了变量 int 装箱，差距收窄） |
| Logger deliver（3 typed + JSON，端到端） | ~1386 | 6 | 结构化日志完整热路径 |
| Field append（float，NaN-safe） | ~42 | 0 | 位掩码 NaN 检查，零分配 |
| Field append（int，基线） | ~15–19 | 0 | 标量基线（批量转义后 key append 更快） |

**codec 对比（3 字段 Record.JSON）**：typed 后 goccy/std/sonic 均为 **240B/1 alloc、峰值 260–300ns**——标量不走 codec（只 `kindAny` 走），**codec 选择对标量记录几乎无影响**；只在大量任意类型字段时才有差异（sonic amd64 最快）。

**剩余 1 alloc（buf 本身）**：`Record.JSON` 的 `make([]byte,...)`。架构使然（record 池 + 多 writer 持有 `formattedBytes`，buf 生命周期跨异步 writer，难以安全 pool）。这是当前架构下的合理下限——zerolog 用扁平复用 buffer 才到 0 alloc，但少了多 writer/async/溢出/恢复。

## 18. 多核分片策略与多环境部署（含业内对比）

### 18.1 何时分片、分多少（两种工况）

ShardLogger 的收益取决于 **writer 是否把单 bootstrap 喂成瓶颈**：

| 工况 | writer 单次耗时 | 单 bootstrap | 分片收益 |
|---|---|---|---|
| **快 writer**（内存/discard/NetWriter loopback） | <1µs | 跟得上，非瓶颈 |  只增派发开销（round-robin 原子 + 多 channel），单 Logger 更优 |
| **慢 writer**（磁盘/Kafka/远程） | ~µs–ms | 成瓶颈 | ✓ 分片并行消费，吞吐近线性扩展到 ~GOMAXPROCS/2 |

实测（M5/10 核，slowWriter ~1µs/record，并行生产者，取最优）：

| shards | ns/op | 聚合 ops/s | 相对单分片 |
|---|---|---|---|
| 1 | ~3.3µs | ~300K | 1.0× |
| 2 | ~1.7µs | ~590K | **2.0×** |
| 4 | ~1.1µs | ~930K | **3.1×（peak）** |
| 8 | ~2.7µs | ~370K | 1.2×（竞争反噬） |

> 快 writer 工况下 discardWriter 实测分片反而略慢（派发开销）——这是正常的，不是 bug。**慢 writer 才是分片的用武之地**。

### 18.2 自动配置（`AutoShardCount`）

```
shards = max(2, GOMAXPROCS / 2)
```

- `/2`：每分片占 1 个 bootstrap goroutine，留另一半核给生产者（业务）+ runtime/OS
- 下限 `2`：分片至少 2 才有并行消费收益
- 无硬 cap：随核数 scale（/2 留生产者核，永不过订阅）；想固定数用 NewShardLogger(n)

| GOMAXPROCS（有效核） | auto shards | 典型环境 |
|---|---|---|
| 1 | 2 | 单核容器（floor） |
| 2 | 2 | 2 核 VM/容器 |
| 4 | 2 | 4 核云主机（常见） |
| 8 | 4 | 8 核 |
| 10 | 5 | 本机 M5 |
| 16 | 8 | 16 核 |
| 32 | 16 | 32 核（随核数 scale，无硬 cap） |
| 64 | 32 | 64 核（大机消化更强，分片更多） |
| 128 | 64 | 128 核 |

> **无硬 cap**：shard = max(2, GOMAXPROCS/2)，随核数线性。之前 cap=8 是 10 核实测（8 shard 占 80% 核→过订阅反噬）；但反噬点是 shard/核数比，不是绝对值——64 核 shard=8 只占 12%，反而消化不足（生产者 56 核 vs 8 消费者→背压）。/2 永不过订阅（留一半核给生产者）。想固定数用 `NewShardLogger(n)`。

用法：

```go
sl := log4go.NewShardLogger(0)   // 0 = 自动（推荐）
// 或显式：sl := log4go.NewShardLoggerAuto()
sl.RegisterFunc(func() log4go.Writer { /* 每分片独立 FileWriter */ })
```

### 18.3 多环境部署（本地 / VM / 云主机 / 容器 / k8s）

`AutoShardCount` 读 `runtime.GOMAXPROCS(0)`，其准确性因环境而异：

| 环境 | GOMAXPROCS 准确性 | 建议 |
|---|---|---|
| 本地 / 裸机 VM / 云主机 | ✓ = 物理核 | 直接用 auto |
| 容器 / k8s（Go 1.25+） | ✓ 尊重 cgroup CPU quota | 直接用 auto |
| 容器 / k8s（Go <1.25 或上报异常） |  可能 = 宿主机核数（过度分片） | `import _ "github.com/v8fg/kit4go/maxprocs"`（封装 automaxprocs，应用级一次性修正；log4go 自身不依赖） |

要点：
- **k8s CPU limit**：Go 1.25 起 GOMAXPROCS 默认按 cgroup quota 算（如 limit=4 → GOMAXPROCS=4 → 2 shards）；旧版本会上报宿主核数 → auto 读准确 GOMAXPROCS；建议 automaxprocs。
- **CPU burst（requests < limits）**：GOMAXPROCS 按 limit 算，分片偏多；/2 自适应（留半核给生产者）。
- **确定性需求**：`NewShardLogger(n)` 显式钉死，绕过 auto。

### 18.4 业内对比（定位）

| 维度 | log4go | zap | zerolog | slog（标准库） |
|---|---|---|---|---|
| 类型化字段（零装箱） | ✓ Round A | ✓ Field | ✓ | ✓ Attr |
| 单核 JSON 吞吐 | ~490 ns/3字段（typed append） | ~快（参考公开） | **最快**（零分配基准） | ~中等 |
| 零分配 record | ✓（无字段 / typed） | ✓ | ✓ 极致 | 部分 |
| 多目的地不同级别 | ✓ Writer 各自 level | ✓ Tee | ✓ | ✓ Handler |
| **溢出防 OOM（ring→file→drop）** | ✓ **内置** | 需自建 | 需自建 |  |
| **崩溃恢复（Drain 重投）** | ✓ **内置** |  |  |  |
| **多核分片（ShardLogger）** | ✓ **内置** |  多实例手搓 |  |  |
| **严格排序（unixNano+seq）** | ✓ |  |  |  |
| **告警 webhook（级别/阈值）** | ✓ WebhookWriter+RateAlerter | Hook 手搓 | Hook | — |
| slog 生态接入 | ✓ NewSlogHandler | 桥 |  | 原生 |
| 字段值健壮性（panic/NaN/typed-nil 安全） | ✓ |  |  |  |

> 性能数字：zerolog 是公认的零分配吞吐基准（参见 zerolog README benchmark）；zap 是结构化+高性能；slog 是 Go 1.21+ 标准库。log4go 在 typed 字段后标量零分配、JSON ~490ns/3字段，性能进入较快，而 **可靠性/运维栈（溢出/恢复/分片/排序/告警）是 zap/zerolog/slog 都需自建的独有优势**。跨库精确对比需同环境实测（各库版本/编码器差异大），上表为定位性参考。

**选型**：纯追极致 JSON 吞吐且自建运维 → zerolog/zap；要**开箱即用的生产级可靠性**（防 OOM、崩溃不丢、多核线性、错误告警）且性能较快 → **log4go**。

## 19. 监控指标与诊断（零热路径开销）

### 19.1 启动诊断（核数 / 分片）

`NewShardLogger`（含 `NewShardLoggerAuto`）启动时输出一行，让运维一眼看到 log4go 实际用到的并行度：

```
[log4go] ShardLogger started: GOMAXPROCS=8 shards=4
```

> GOMAXPROCS 在 Go 1.25+ 自动尊重 cgroup CPU quota；`shards = max(2, GOMAXPROCS/2)`（§18.2）。用标准 log 输出，避免 log4go 引导自身。

### 19.2 运行时指标（`RuntimeStats`，按需采集）

```go
m := log4go.RuntimeStats()
// m.GOMAXPROCS / m.NumGoroutine / m.HeapAlloc / m.HeapInuse / m.HeapObjects
// m.HeapSys / m.NumGC / m.StackInuse / m.GCCPUFraction
```

**性能保证**：`RuntimeStats` 调 `runtime.ReadMemStats`（有亚毫秒级 STW），**log4go 内部从不调用它**，只在监控采集时调（如 Prometheus scrape 每 10–30s），**绝不进入每条 record 的日志热路径**。

### 19.3 暴露给 Prometheus（示例）

```go
var (
	heapAlloc = promauto.NewGauge(prometheus.GaugeOpts{Name: "log4go_heap_alloc_bytes"})
	goroutines = promauto.NewGauge(prometheus.GaugeOpts{Name: "log4go_goroutines"})
	gcCPU     = promauto.NewGauge(prometheus.GaugeOpts{Name: "log4go_gc_cpu_fraction"})
)
// collector：scrape 时采集（ReadMemStats 的 STW 在 scrape 节点，不影响日志）
go func() {
    for range time.Tick(15 * time.Second) {
        m := log4go.RuntimeStats()
        heapAlloc.Set(float64(m.HeapAlloc))
        goroutines.Set(float64(m.NumGoroutine))
        gcCPU.Set(m.GCCPUFraction)
    }
}()
```

### 19.4 日志量指标（已有，per-level）

```go
m := log4go.Metrics()           // 各级别 record 计数（per-level counters）
kwm := kw.Metrics()             // KafKaWriter: Sent/Errored/Dropped/Spilled/Queued/SpillLen
```

> 推荐指标：`log4go_records_total{level}`、`log4go_heap_alloc_bytes`、`log4go_goroutines`（恒定=健康，上升=泄漏）、`log4go_gc_cpu_fraction`（持续 >0.2 需降配/扩容）、`kafka_dropped/spilled_total`（>0 触发告警）。

### 19.5 关于"看着比 zerolog 慢"

zerolog 是公认的零分配 JSON 吞吐基准（扁平 event buffer + 复用）。log4go 的 `Record` 抽象换来了 zerolog 没有的能力：多 writer 串行派发、async/溢出/崩溃恢复、池化复用、严格排序 —— 这层中转有固定开销。优化后单核 JSON ~460ns/1alloc（3 字段）、logfmt ~267ns/1alloc，进入较快；**可靠性/运维栈（溢出/恢复/分片/告警）是 zerolog/zap/slog 需自建的独有优势**。纯追 JSON p99 且无需运维栈 → zerolog；要开箱即用生产级可靠性 → log4go。

### 17.4 Pipeline alloc 下限（pprof 定位 + caller 缓存）

`LoggerInfo`（带 caller + 格式化参数）allocs 演进：7 →（caller 路径 `path.Base`→切片、`Itoa`→`AppendInt`）6 →**（caller 缓存）3**。pprof 定位并逐项消除：

| alloc 来源 | 处理 |
|---|---|
| ~~`runtime.Caller` 的 file 路径串~~ | ✓ 改 `runtime.Callers` 只取 PC（不分配 file 串） |
| ~~`fi.String()`（file:line 拼接）~~ | ✓ **caller 缓存**：PC→file:line memoize，同调用点首次后 0 alloc |
| `fmt.Sprintf(msg, args)` 结果串 | （用户格式串；无参 `Info("msg")` 已跳过） |
| variadic `interface{}` 装箱（`Info(fmt,i)` 装 i + args 切片） |  **Go 语言下限**（无参则 0） |

**caller 缓存**（`callerFileLine`）：`runtime.Callers(3, pcs)` 取 PC（零 file alloc），`map[callerKey]string` 按 `(pc, fullPath, withFunc)` 缓存 "base.go:line[ func]"。同调用点首次 miss（算一次 FileLine + 拼接），之后命中 = 一次 RLock map 查询。缓存条目数 = 日志调用点数（有限）。zap 同样缓存 caller。

**结论（实测）**：
| 路径 | allocs | B/op | 构成 |
|---|---|---|---|
| `hasCaller=true` + `Info(fmt, i)` | **3** | 49 | Sprintf(1) + variadic 装箱(2) — Go 下限 |
| `WithCaller(false)` + no-arg `Info("msg")` | **1** | ~17 | 仅 record 通道相关 |
| `Record.JSON`（3 字段） | **1** | 240 | buf（多 writer/async 架构下限） |

> 要突破 3 allocs（消除 variadic 装箱 + Sprintf）需 builder API（zerolog 链式 `Str()/Int()`，typed 参数不装箱）——与 `Info("msg")` 易用性冲突，故保留双模式的可能性但不替换现有 API。

## 20. 高性能 vs 易用 用法对比（cookbook）

> 对应 doc.go "# High-performance vs easy usage"。allocs 是热无关稳定信号，ns 为冷机峰值（M5/Go1.26）。**默认档已较快**，极致吞吐只需几个开关。

### 20.1 四档用法 + 实测

| 档 | 用法 | allocs | ns/op(峰) | QPS/核 | 易用 | 适用 |
|---|---|---|---|---|---|---|
| **A 易用** | `NewProduction()` + `Info("msg")` | **1** | ~1100 | ~910K | 高 | 99% 业务 |
| A 易用 | `NewProduction()` + `With(k,v).Info(fmt,i)` | 3 | ~1200 | ~830K | 高 | 结构化 |
| **B 高性能结构化** | `WithString/WithInt/WithAttrs(...).Info` | 2-3 | ~1250 | ~800K | 中 | 高频+字段 |
| B 序列化 | `Record.JSON`（3 typed 字段） | **1** | ~210-350 | — | — | JSON 基准 |
| **C 极致吞吐** | `WithCaller(false)` + 无参 `Info("x")` + async File | **1** | ~1100 | **~923K** | 低 | 超高频 |
| **D 多核线性** | `ShardLogger(0)` + `RegisterFunc` async File | — | shard×单核 | 4 shard ~3× | 低 | 10M 级 |

### 20.2 allocs 构成（为何这些是下限）

`Info(fmt, args)` 带 caller = **3 allocs**（Go 语言下限）：
- `fmt.Sprintf` 结果串（1）—— 用户格式串，不可避免；无参 `Info("msg")` 跳过 → 省
- variadic `interface{}` 装箱：args 切片 + i 装箱（2）—— Go 签名成本；无参则 0
- ~~caller（runtime.Caller file 串 + file:line 拼接）~~ → **已由 caller 缓存消除**（PC→file:line memoize，命中 0 alloc）

无参 `Info("msg")` + `WithCaller(false)` = **1 alloc**（record 通道相关，单核最大吞吐）。

### 20.3 开关收益排序

| 开关 | 收益 | 代价 |
|---|---|---|
| `WithCaller(false)` | −2 allocs | 丢 file:line |
| 无参 `Info("msg")`（vs `Info(fmt,i)`） | −2 allocs | 无格式化 |
| typed `WithString/WithInt`（vs `With(k,v)`） | 标量零装箱、类型安全 | 链式稍长 |
| `FileWriter{Async:true}` | daemon 隔离磁盘，不阻塞 bootstrap | 异步（flush 时机） |
| `OverflowPolicy:"spill"` | 防 OOM + 不丢热数据 | 多占 ring/file 空间 |
| `ShardLogger(0)` | 慢 writer 下多核线性 | 多 goroutine + 仅慢 writer 有效 |
| `ConsoleWriter{Buffered:true}` | bufio 减 syscall | 需定时 flush |

### 20.4 选型决策

- **默认**：`NewProduction()` + `With/Info` —— 较快性能 + 全套可靠性，**除非测出瓶颈否则别动**
- **单核 >900K QPS**：档 C（`WithCaller(false)` + 无参 + async File）
- **10M QPS 级 / 慢 writer**：档 D（`ShardLogger(0)` + `RegisterFunc`）
- **高频结构化**：档 B（typed 字段）

### 20.5 反模式

- 生产 unbuffered Console（阻塞 bootstrap）
- 跨 shard 共享 `*FileWriter`（race；必须 `RegisterFunc`）
- 高频 `With("count", i)`（装箱；用 `WithInt`）
- 每条 log `Sprintf` 大对象（预格式化或字段）
- NetWriter 高吞吐（RTT-bound；用 File + Kafka）
