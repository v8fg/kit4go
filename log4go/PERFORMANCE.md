# log4go 性能与架构设计

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
| `deliver` pipeline（discard writer） | 1084 | 923K | 395 | 8 | 调用方开销上限 |
| `Logger.Filtered`（被级别过滤） | 12 | 83M | 7 | 0 | 过滤几乎零成本 |
| ConsoleWriter（pipe→discard） | 1705 | 586K | 160 | 6 | 真实终端会慢 1~2 个数量级 |
| ConsoleWriter + Color | 1908 | 524K | 192 | 6 | ANSI 着色开销小 |
| FileWriter（bufio 8192） | 339 | **2.95M** | 144 | 5 | 写缓冲，定时 flush 落盘 |
| KafKaWriter.buildPayload（fast, 无字段） | 288 | 3.5M | 288 | 2 | 框架字段手动 append，零反射 |
| KafKaWriter.buildPayload（slow, ExtraFields） | 522 | 1.9M | 291 | 3 | 含 1 用户字段，仅值 marshal |
| KafKaWriter.buildPayload（base fields, 5 字段） | 1006 | 1.0M | 888 | 8 | Kafka→ES 典型：hostname/server_ip/app/es_index/trace_id |
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
- ✅ KafKaWriter：AsyncProducer + 有界 channel + Drop/Block/Spill(内存环/落盘) 溢出框架。
- ✅ 所有数据组件：`Metrics()` 快照 + `SetOnEvent` 实时 hook（监控预留）。
- ✅ `Logger.writers` data race 用 `atomic.Value` 修复。
- 💡 FileWriter：bufio 缓冲当前 8192，高频可调大（需改 `bufio.NewWriterSize`，预留接口）。
- 💡 ConsoleWriter：仅开发用；生产走 File/Kafka。

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

### 12.4 JSON 格式（FormatJSON，对标 zap/zerolog 最强标配）

`SetFormat(FormatJSON)` → 每条 record 输出一个 JSON 对象：`{"time":"...","level":"INFO","msg":"...","file":"...","fields":{...}}`。格式在 `deliverRecordToWriter` 决定一次并缓存到 `r.jsonBytes`，**所有 writer 直接输出预序列化字节**（不重复序列化）。time 用 `timestampLayout`（ISO，带时区），fields 为空时省略。

### 12.5 JSON 编解码器对比（Go 1.26.0 / Apple M5 10 核）

`SetJSONCodec(JSONCodecGoccy|JSONCodecStd|JSONCodecSonic)` 切换编码器。默认 **Goccy**（最快可移植，已依赖）。实测（含 3 个字段的 record）：

| 编解码器 | ns/op | B/op | allocs | 说明 |
|---|---|---|---|---|
| **Goccy（默认）** | **792** | 993 | 6 | 最快可移植，无 cgo |
| Std（encoding/json）| 1028 | 1120 | 12 | 标准库，兼容性最强（慢 ~30%，allocs 2x）|
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
| **结构化字段**（With/WithField） | ✅ | ✅（ SugaredLogger）| ✅（Context）| ✅（Fields）| log4go: 子 logger 链式累加，热路径零开销（无 With 不付费）|
| **JSON 格式** | ✅ goccy/std/sonic 可切换 | ✅（强项，零反射）| ✅（强项，零 alloc）| ✅（encoding/json）| log4go 默认 goccy ~980K/s；zap/zerolog ~数百万/s（零反射优势）|
| **采样（Sampling）** | ✅ WithSampling(initial, thereafter) | ✅（core level）| ✅（Sampling）| ❌ | log4go 按 level 独立 atomic 计数，采样丢弃不计 Metrics |
| **context.Context** | ✅ IntoContext/FromContext + RequestIDMiddleware | ⚠️（需 helper）| ✅（zerolog.Ctx）| ❌ | log4go 无硬 otel 依赖，AddContextExtractor 可叠加 trace/baggage |
| **日志旋转（Rotate）** | ✅ 内置 daily/hourly/minutely + MaxDays/MaxHours | ❌（需 lumberjack）| ❌（需 lumberjack）| ❌（需 lumberjack）| log4go 内置，无需第三方 |
| **多 sink 并发**（多 writer）| ✅ Register 多 writer（bootstrap 串行）| ✅（core chain）| ⚠️（Multi-level）| ✅（MultiHook）| log4go: 串行保全局顺序；慢 writer 拖累全部（建议 async writer）|
| **Kafka writer** | ✅ 内置（AsyncProducer + 溢出框架）| ❌（需自建）| ❌（需自建）| ❌（需自建）| log4go 内置 drop/block/spill(ring/file/chain) 防 OOM |
| **NetWriter（TCP/UDP）** | ✅ 内置 async + 溢出 | ❌（需自建）| ❌（需自建）| ❌（需自建）| log4go 内置，async + 有界 + drop/spill 防阻塞 |
| **溢出/防 OOM**（drop/spill/block）| ✅ 泛型 Spiller[T]（ring→file→drop 多级）| ❌ | ❌ | ❌ | log4go 独有：channel 满 ring 兜底 → file 兜底 → drop，总空间硬上限 |
| **多核分片**（ShardLogger）| ✅ NewShardLogger(n) round-robin | ⚠️（多实例自管）| ⚠️（多实例自管）| ❌ | log4go: 10 核 4 shard ~1.6M QPS，唯一突破单核 ~1M 正路 |
| **告警钩子**（SetOnEvent / AlertSink）| ✅ 实时事件 + webhook（lark/dingtalk/feishu）| ⚠️（hooks 弱）| ⚠️ | ✅（Hook）| log4go: 溢出首次 + 每 N 次告警，不刷屏不递归 |
| **崩溃恢复**（file spill drain）| ✅ 启动 Drain 重投 | ❌ | ❌ | ❌ | log4go: 中断后从 spill.log 续投（ring 不参与恢复）|
| **Metrics** | ✅ 各级别计数 + writer 运行指标 | ⚠️（需自建）| ⚠️ | ⚠️ | log4go: Logger.recordsByLevel + FileWriter/KafKaWriter/NetWriter Metrics 快照 |
| **吞吐（单核，JSON）** | ~980K（goccy）/ ~7.1M（File text）| ~数百万/s | ~数百万/s（零 alloc）| ~100K-300K | zap/zerolog 零反射 JSON 最快；log4go goccy 次之；logrus 最慢 |
| **吞吐（多核）** | ~1.6M（4 shard）| 需自管多实例 | 需自管多实例 | — | log4go ShardLogger 内置分片，线性扩展 |
| **内存（100K 条）** | < 0.05MB HeapAlloc / 恒 4 goroutine | ~零 alloc（zerolog 更优）| ~零 alloc | 较高 | log4go 池复用 + 有界 channel，OOM 防护闭环 |
| **依赖** | goccy/go-json（可选 sonic）| 仅 zapcore | 零依赖 | 多依赖 | log4go 默认 goccy（可移植），sonic 仅 amd64 最优 |

### 选型总结

- **选 zap/zerolog**：纯追求极致 JSON 吞吐 + 零 alloc，且不介意自建 rotate/kafka/net/溢出。
- **选 log4go**：需要**开箱即用**的完整生产栈（rotate + kafka + net + 溢出防 OOM + 多核分片 + 告警 + 崩溃恢复），且吞吐够用（单核 File ~7M / JSON ~980K，多核 4 shard ~1.6M）。
- **选 logrus**：已用且性能不敏感（吞吐最低，~100K-300K）。

> log4go 的**差异化优势**是内置的**溢出防 OOM 框架**（ring→file→drop 多级 + 告警 + 崩溃恢复）和**多核 ShardLogger**——这两项 zap/zerolog 均需自建且易踩 OOM/竞争坑。
