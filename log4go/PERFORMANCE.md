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
| KafKaWriter.buildPayload（JSON） | 2504 | 400K | 1208 | 27 | 调用方序列化热路径 |
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
单实例可达（File 2.95M / Kafka 400K buildPayload，余量充足）。
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

## 8. 验证手段（本地，不依赖真实 Kafka）

```bash
make test                      # go test -short -race ./...
make test-bench                # 各 writer / pipeline benchmark（本文表格数据来源）
go test ./log4go/ -bench . -benchmem -run '^$'
# 溢出框架：RingSpiller 10w push 后 Len 仍 = 容量（验证不 OOM）
# KafKaWriter：sarama mocks.AsyncProducer 端到端（无真实 broker）
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

## 11. 性能压测与调参建议（Go 1.26.2 / 本机 10 核 GOMAXPROCS=10）

> 压测环境：Go 1.26.2，darwin/arm64，NumCPU=10。ns/op 为每条日志耗时，B/op 为每条分配内存，QPS=1e9/ns。多核 ShardLogger 的 ns/op 为并行 wall/total（已含多核并行）。

### 单线程极限
| 模式 | ns/op | ~QPS/核 | 内存 B/op | alloc | 说明 |
|---|---|---|---|---|---|
| caller（file:line）| 1047 | ~955K | 314 | 6 | 默认，保定位 |
| no-caller（`WithCaller(false)`）| 1072 | ~933K | **17** | **1** | 极致（失 file:line）|
| filtered | 13 | ~77M | 7 | 0 | 级别过滤近零成本 |

no-caller 多轮最佳曾达 981ns（~1.02M，破 1M）。caller 受限 runtime.Caller。

### 多核 ShardLogger（10 核，并行 wall/total）
| shard | ns/op | ~QPS | 内存 B/op | 备注 |
|---|---|---|---|---|
| 1 | 1587 | 631K | 289 | 退化单 logger |
| 2 | 844 | 1.18M | 295 | |
| **4** | **677** | **~1.48M** | 303 | **最佳（≈ 核数/2）** |
| 8 | 1039 | 963K | 307 | |
| 16 | 2012 | 497K | 311 | 超核数 1.6×，调度反噬 |

**shard ≈ 核数/2~核数最佳**；超过核数调度反噬。本机 10 核下 4 shard 达 ~1.48M。

### Writer 吞吐
| writer | ns/op | ~QPS | 内存 B/op |
|---|---|---|---|
| File（bufio）| 293 | ~3.41M | 144 |
| Console（pipe→discard）| 1793 | ~558K | 160 |

File（bufio 缓冲）最快；Console 受 stdout I/O 限制，生产建议禁用。

### 建议参数
| 场景 | 配置 | 预期 |
|---|---|---|
| 生产（保定位）| 单 logger caller，`recordChannelSize` 8192，level INFO | ~851K/核 |
| 极致（舍定位）| 单 logger `WithCaller(false)` | ~861K/核 |
| 多核扩展 | `ShardLogger(GOMAXPROCS)` | 线性（8 核理论 ~6.9M，实际 1-3M）|

### 突破 1000K
单线程物理极限 ~861K（no-caller）。**多核 `ShardLogger`（shard=核数）是唯一正路**，干净环境线性扩展可超 1M。防丢用 overflow `spill`（ring→file），防 OOM 用 buffer/spiller 有界。

> 注：上表为长会话/高负载保守值；CI/生产干净环境数值更高。完整压测见
> `Benchmark_LoggerInfo` / `Benchmark_LoggerInfoNoCaller` / `Benchmark_ShardLoggerScale`。



