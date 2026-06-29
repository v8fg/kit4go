# log4go 性能与架构

> 高性能、内存安全、可观测的 Go 日志库。涵盖架构、Writer 实测吞吐/内存、瓶颈与优化，
> 以及生产（含广告行业 100K / 10M QPS）配置。英文版：[PERFORMANCE.en.md](PERFORMANCE.en.md)。

## 1. 架构

```
 调用 goroutine                       单 bootstrap goroutine
 ┌──────────────────┐                ┌──────────────────────────────────┐
 │ Debug/Info/...() │   deliver      │  for rec := range records {       │
 │  format + Caller ├────records────>│    for _, w := range writers {    │
 │  (level filter)  │     chan(4096)  │       w.Write(rec)  ← 串行        │
 │  atomic counter  │                │    }                             │
 └──────────────────┘                │  } + flush/rotate 定时器          │
                                     └──────────────────────────────────┘
```

- **调用方只做轻活**：级别过滤、时间格式化、`runtime.Caller`、推入有界 `records` chan（4096）。实测 `deliver` ≈ **1080 ns/op ≈ 923K QPS/核**。
- **单 bootstrap goroutine，逐条串行**：端到端 QPS ≈ 1/Σ(writer Write 耗时)。任一 writer 慢则整体被拖。
- **OOM 防护**：`records` chan 有界；KafKaWriter 自带独立有界 chan + 多策略溢出框架。**绝不每条记录起一个 goroutine**（旧 KafKaWriter OOM 的根因，已修复）。

## 2. 各 Writer 吞吐与内存（单核，Go 1.26）

| Writer / 路径 | ns/op | ~QPS/核 | B/op | allocs | 说明 |
|---|---|---|---|---|---|
| `deliver` 管线（discard） | 1084 | 923K | 395 | 8 | 调用方侧上界 |
| `Logger.Filtered`（级别丢弃） | 12 | 83M | 7 | 0 | 近乎免费 |
| ConsoleWriter（pipe→discard） | 1705 | 586K | 160 | 6 | 真实终端慢 1-2 个数量级 |
| FileWriter（bufio 8192） | 339 | **2.95M** | 144 | 5 | 带缓冲、定时 flush |
| KafKaWriter.buildPayload（无字段） | 288 | 3.5M | 288 | 2 | 类型化 append，零反射 |
| KafKaWriter.buildPayload（5 基础字段） | 1014 | 1.0M | 800 | 3 | Kafka→ES 典型场景（类型化，allocs −63%） |
| RingSpiller.Push | 10 | 100M | 0 | 0 | 内存环 |
| FileSpiller.Push | 424 | 2.4M | 148 | 4 | 落盘溢出 |

> 端到端 = 调用方 deliver + 串行 writer。File + Kafka：bootstrap ≈ 339(File) + ~100(Kafka 入队) ≈ 440ns → ~2.2M QPS/核。

## 3. 瓶颈与修复

| 瓶颈 | 影响 | 修复 |
|---|---|---|
| ConsoleWriter 同步 stdout | 阻塞 bootstrap | 生产关闭（仅调试） |
| bootstrap 串行 | writer 越多/越慢 → 越低 | 只保留 File + Kafka |
| records chan 满 | 调用方阻塞（背压） | KafKaWriter drop/spill |
| 每条记录起 goroutine（旧） | goroutine 堆积 → OOM | **已修复**（零每条记录 goroutine） |

## 4. 调参

| 参数 | 默认 | 范围 | 作用 |
|---|---|---|---|
| `recordChannelSize` | 4096 | 4096–65536 | records chan 容量 |
| KafKaWriter `BufferSize` | 1024 | 8192–65536 | 有界 send chan |
| `OverflowPolicy` | drop | drop/spill/block | 满载策略 |
| `SpillType` / `SpillSize` / `SpillMaxBytes` | ring/1024/64MB | ring/file | 溢出存储 |
| `flushTimer` | 500ms | 200ms–1s | bufio flush 间隔 |

## 5. 线上启用

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

## 6. 场景配置（广告行业）

**100K QPS**（常规竞价日志）：单实例即可（File 2.95M / Kafka buildPayload 1.0–3.5M）。File 按天轮转 + Kafka spill 内存环。

**10M QPS**（完整曝光/点击流）：单 bootstrap 串行是天花板（~2–3M/核）。**水平分片**：每分片一个 KafKaWriter + `spill=file`，N 个 pod，Kafka 分区数 ≥ 并发度。

## 7–11. 监控、验证

- 拉取：`Metrics()`（按级别）、`kw.Metrics()`（Sent/Errored/Dropped/Spilled/Queued/SpillLen）。
- 推送：`SetOnEvent(name, delta)` 实时钩子 → Prometheus/statsd。
- 无需真实 Kafka 本地验证：`go test -bench . -benchmem -run '^$'`；sarama mock 做 e2e；noopAsyncProducer 做 bench。

## 12. 各 Writer 吞吐（全 writer，Go 1.26 / M5 10c）

| ConsoleWriter（带缓冲） | ~129ns | 7.8M |
| FileWriter（async+spill） | ~186ns | 5.4M |
| KafKaWriter（mock） | ~879ns | 1.1M |
| NetWriter（TCP 回环） | ~76ns | 13.1M（调用方侧；真实网络受 RTT 限制） |
| IOWriter（discard） | ~297ns | 3.4M |
| Record.JSON（goccy，3 字段） | ~210–350ns | 类型化 append |

## 13. 内存（100K 条，MemPerWriter）

所有 writer HeapAlloc < 0.005MB，goroutine 常驻 **4**（池复用 + 有界 chan + 有界 spiller）。1M 条：HeapAlloc 1.3MB，NumGC 7，goroutine 3。

## 16. Round A：类型化字段 + slog + logfmt + 预设 + Panic/Fatal

三层优化：① 类型化 append（无 map/反射/装箱）② `appendISOTimeUTC`（无 time.Format 字符串分配）③ 批量字符串转义（clean-run 单次 append）。JSON/logfmt 降到 **1 alloc**。

| Bench | 峰值 ns | allocs | 说明 |
|---|---|---|---|
| Record.JSON（3 字段） | ~195 | **1** | 对比 5801/6 基线（**−95% / −83%**） |
| Record.Logfmt（3 字段） | ~229 | **1** | |
| Kafka buildPayload（5 基础字段） | ~432 | 2 | |
| SlogHandler.Handle | ~1270 | 7 | slog 桥接（+19% vs 原生） |
| Logger.WithInt（类型化） vs With(iface) | ~138 vs ~140 | 3 vs 3 | 类型化避免 type-switch |
| Field append int / float | ~15 / ~45 | **0** | 零分配标量 |

**codec 对比**：goccy/std/sonic 对标量记录都是 240B/1alloc，codec 选择对标量无影响（只有 kindAny 才会命中）。

## 17. 类型覆盖与健壮性（对标 zap/zerolog/slog）

fieldOf 映射：string/bool/int*/uint*/uintptr/float/[]byte/complex/duration/time/error → 类型化（零装箱）；其余 kindAny。

| 风险 | 处理 |
|---|---|
| panic 的 MarshalJSON | `safeJSONMarshal` recover → null |
| typed-nil error | `safeErrorString` recover → null |
| chan/func 不可序列化 | kindAny 失败 → null |
| NaN/±Inf（非法 JSON） | 位掩码 → null |
| complex 的 NaN 分量 | → null |

> 只在 kindAny/kindError（慢路径）做 recover；标量热路径零开销；float NaN/Inf 用单次位掩码 AND 判定。

## 17.4 管线分配下限（pprof）

`LoggerInfo`（caller + format 参数）：7 → 6（path.Base→slice，Itoa→AppendInt）→ **3**（caller 缓存：PC→file:line 通过 runtime.Callers 记忆化）。

| 路径 | allocs | 组成 |
|---|---|---|
| hasCaller + `Info(fmt,i)` | **3** | Sprintf(1) + 可变参装箱(2)，Go 下限 |
| WithCaller(false) + 无参 `Info("msg")` | **1** | 单核最大吞吐 |
| Record.JSON（3 字段） | **1** | buf（多 writer/异步下限） |

> 低于 3 需要 builder API（zerolog 链式，避免可变参装箱），代价是牺牲 `Info("msg")` 的易用性；未采纳。

## 18. 分片策略与多环境（对标 zap/zerolog/slog）

当 **writer 成为单 bootstrap 的瓶颈**（慢 writer ~µs 级）时分片才划算。实测（M5/10c，slowWriter ~1µs）：1→2 分片 ~2×，4 分片 ~3.1×（峰值），8 分片退化（竞争）。快 writer（discard/内存）下分片只增加调度开销。

**AutoShardCount** = `max(2, GOMAXPROCS/2)`。`/2` 给生产者留核；下限 2。无硬上限，分片数随核数增长（64 核机器需要更多并行消费者，封顶 8 会成为瓶颈）。

| GOMAXPROCS | 分片 | 环境 |
|---|---|---|
| 1 | 2 | 单核容器 |
| 4 | 2 | 常见云 |
| 8 | 4 | |
| 16 | 8 | |
| 32 | 16 | 随核数增长 |
| 64 | 32 | 大机器，更多消费者 |
| 128 | 64 | |

多环境：Go 1.25+ 原生识别 cgroup CPU 配额；更老/异常环境 → `import _ "github.com/v8fg/kit4go/maxprocs"`。

| log4go | zap | zerolog | slog | |
|---|---|---|---|---|
| 类型化零装箱 | 是 | 是 | 是 | 是 |
| 溢出防 OOM | 是 内置 | 否 自建 | 否 | 否 |
| 崩溃恢复 | 是 内置 | 否 | 否 | 否 |
| 多核分片 | 是 内置+自动 | 否 | 否 | 否 |
| 严格顺序 | 是 | 否 | 否 | 否 |
| 告警 webhook | 是 | Hook | Hook | — |
| 字段健壮性 | 是 | 否 | 否 | 否 |

## 19. 监控与诊断

- 启动：`[log4go] ShardLogger started: GOMAXPROCS=N shards=M`。
- `RuntimeStats()` → GOMAXPROCS/NumGoroutine/HeapAlloc/HeapInuse/HeapObjects/NumGC/GCCPUFraction。会调 ReadMemStats（亚毫秒级 STW），**绝不在热路径上调用**；按采集周期取。
- Prometheus collector 示例见 PERFORMANCE.en.md §19.3。

## 20. 高性能 vs 易用（cookbook）

| 档位 | 用法 | allocs | 峰值 ns | QPS/核 | 易用 | 适用 |
|---|---|---|---|---|---|---|
| **A 易用** | `NewProduction()` + `Info("msg")` | **1** | ~1100 | ~910K | 高 | 99% 日志 |
| A | `With(k,v).Info(fmt,i)` | 3 | ~1200 | ~830K | 高 | 结构化 |
| **B 性能结构化** | `WithString/WithInt/WithAttrs` | 2-3 | ~1250 | ~800K | 中 | 高频+字段 |
| **C 最大吞吐** | `WithCaller(false)` + 无参 + 异步 File | **1** | ~1100 | **~923K** | 低 | 超高频 |
| **D 多核** | `ShardLogger(0)` + `RegisterFunc` | — | 分片×核 | 4 分片 ~3× | 低 | 10M 级 |

**收益排序**：`WithCaller(false)` −2 alloc（单项最大）→ 无参 `Info("msg")` −2 alloc → 类型化字段（标量零装箱）→ `FileWriter{Async:true}` → `OverflowPolicy:"spill"` → `ShardLogger(0)`。

**反模式**：生产用无缓冲 Console；跨分片共享一个 `*FileWriter`（用 RegisterFunc）；高频 `With("count",i)`（用 WithInt）；高量用 NetWriter。

> **默认档位已经很扎实**（类型化字段、caller 缓存、零反射，+ 溢出/恢复/分片/告警）。除非测出瓶颈（单核 >900K QPS 或 10M 级），否则用默认即可，把精力放在业务上。

## 21. 压测与持续负载验证（2026-06-28，Apple M5 10c，Go 1.26）

v0.1.0 多模块拆分 + golangci-lint 迁移后的完整压测复跑。除特别说明外所有数字取自
`go test -bench -benchmem`。

### Deliver 管线——多核扩展（广告行业热路径）

| Benchmark | 1 CPU | 4 CPU | 8 CPU | allocs/op |
|---|---|---|---|---|
| `LoggerInfo`（caller + writer） | 6056 ns | 1492 ns | **1468 ns** | 3 |
| `LoggerInfoNoCaller`（writer） | 5995 ns | 1441 ns | **1374 ns** | **1** |
| `DeliverPipeline_Discard` | 5974 ns | 1473 ns | 1473 ns | 3 |
| `DeliverPipeline_SampledActive` | 5916 ns | 1537 ns | 1519 ns | 1 |
| `DeliverPipeline_Filtered` | 13.1 ns | 11.5 ns | 11.5 ns | **0** |
| `DeliverPipeline_SampledOut` | 3.27 ns | 3.27 ns | 3.27 ns | **0** |

- **1 → 4 CPU ≈ 4× 扩展**（~165K → ~690K rec/s/核），**4–8 CPU 进入平台期**，即文档所述
  的 channel 调度瓶颈。`ShardLogger` 是突破平台期的路径。
- 热路径 **≤1 alloc/op**（无 caller）；过滤/采样丢弃路径 **0 alloc**。

### 持续负载（Soak）——吞吐稳定性 + 泄漏

- **8 CPU 持续 10s**：`LoggerInfo` 1468 ns/op，`NoCaller` 1374 ns/op，**与 2s 基线一致**，
  持续负载下吞吐无衰减。
- **堆（1,000,000 条 + GC）**：`前 603 KB → 后 607 KB，Δ +3 KB`（约 3 字节/条残留，分配器碎片），**无内存泄漏**。
- **goleak**（`shutdown_leak_test.go`）：Close 后 **0 goroutine 泄漏**。
- **stress/**（`TestStress_AllClients`、`TestStress_ConcurrentSafety`）：10K ops × 5 种 client，
  **`-race` 下通过**，无数据竞争。

### codec 与字段（关键处零分配）

| Benchmark | ns/op | B/op | allocs |
|---|---|---|---|
| `Field_IntJSON` | 15.2 | 0 | **0** |
| `Field_FloatJSON` | 37.0 | 0 | **0** |
| `KafKaCodec_JSON` | 196 | 384 | 2 |
| `KafKaCodec_Proto` | 250 | 288 | 3 |
| `Logger_DeliverTypedFields`（6 字段） | 1542 | 696 | 6 |

**结论**：所有性能声明成立——带 writer 时 ~700K rec/s/核，丢弃路径零分配，多核线性扩展，
持续负载稳定，无泄漏，无竞争。满足 100k–1M+ QPS 广告行业目标。仓库级数据（微服务基建 + 各 client）
见根目录 `BENCHMARKS.md`。

## 22. KafKaWriter 批处理守护模式（2026-06-29，Apple M5 10c，Go 1.26）

`BatchMode`（可选，默认**关闭**）让守护 goroutine 攒批，在 `BatchSize`（**默认 1024**，
`DefaultKafkaBatchSize`，原为 100；压测矩阵显示 1024 是最佳默认：franz-go 需 batch ≥1024 才接
近峰值，sarama 持平，无额外内存，见 `kafka/STRESS_MATRIX.md`）/ `BatchFlushInterval`（50ms）/
关机时通过 `producer.SendBatch` 一次性 flush，而不是每条记录一次 `producer.Send`。**Write 热路径
不变**（攒批完全在守护 goroutine 内）；`Producer`/`SyncProducer` 接口和逐条默认路径均不动。

### 管线基准——完整路径（Write → channel → 守护 → mock producer），`block` 策略

| Benchmark | ns/op | B/op | allocs |
|---|---|---|---|
| `KafKaWriter_Pipeline_PerRecord` | 624 | 1033 | 2 |
| `KafKaWriter_Pipeline_Batch`（size 200, 10ms） | 696 | 1152 | 2 |

**如实解读：对 no-op mock，批处理慢约 11%。** 当 `producer.Send` 几乎瞬时，没有真实的每次调用
成本可摊销，攒批的开销（slice 累积 + flush）是纯额外开销。

### 真实 broker 吞吐（apache/kafka 3.8.0 KRaft，默认 10ms linger，`block` 策略）

| 模式 | rec/s（3 次） | batch / per-record |
|---|---|---|
| per-record（默认） | 35,154 / 36,072 / 35,465 | — |
| batch（size 200, 10ms） | 35,607 / 35,291 / 35,554 | **0.98–1.01×（噪声范围内，≈持平）** |

**定论：在 kafka 后端默认 10ms linger 下，log4go 批处理模式吞吐无可见收益**（跨次 0.98–1.01×，
≈ 持平，本机单 broker 均 ~35K rec/s）。后端的 linger 会把逐条 `Send` 和 `SendBatch` 合并成同样的
broker 级批次，因此守护调 Send 还是 SendBatch 对 broker 不可见。**这就是 `BatchMode` 默认关闭的
原因**：在默认 linger 下是纯额外开销（mock −11%；broker ≈ 持平），换不来吞吐。

### BatchMode 何时才划算——关键在每次调用成本

异步生产者的 `Send` 是近乎免费的入队，`SendBatch` 没有每次调用成本可摊销，因此持平。但当
**生产者调用本身有成本时（同步生产者、broker 过载导致守护积压、或单次调用做重活），攒批收益极大**。
由 `Test_KafKaWriter_BatchFasterWhenSendCostly` 证明：mock 对每次*调用*（每次 Send/SendBatch，非
每条记录）收费 50µs：

| 模式（5000 条，50µs/调用） | rec/s | 提速 |
|---|---|---|
| per-record | 13,754 | — |
| batch（size 100） | 266,535 | **19.4×** |

所以 BatchMode 是正确且有价值的，它摊销每次调用成本，只是对 log4go 默认的免费入队异步生产者
没什么可摊销的。当守护的生产者调用（或可能）成为瓶颈时开启它。

### `ProducerLinger` 旋钮（在 `KafKaWriterOptions` 上暴露）

`ProducerLinger` 调的是 kafka **后端**的批次 flush 延迟（延迟/吞吐旋钮）：`0` = 后端默认（10ms）；
`>0` = 显式 linger；`kafka.LingerOff` = 关闭后端时间攒批。它**不会**让 log4go 的 `BatchMode` 更快，
后端对两种模式都按条数攒批（`Flush.Messages` = `MaxBufferedRecords`），所以 Send 与 SendBatch 仍 ≈ 持平。

**`LingerOff` 死锁坑（sarama 后端，已修复）**：只设 `Flush.Frequency=0` 而 `Flush.Messages` 保持默认
1000 时，最后的不满一批永远到不了条数阈值，又没有定时器 flush → 在 `OverflowPolicy:"block"` 下
writer 死锁（实测卡在 offset 9257/10000）。log4go 做了防护：`ProducerLinger=kafka.LingerOff` **同时**
强制 `MaxBufferedRecords=1`（`Flush.Messages=1`），后端收到即 flush，真正"不攒批"，安全。所以
`LingerOff` = 逐条 flush（最低延迟、最高逐条开销），不是批处理吞吐杠杆。

数据丢失：硬崩溃时未 flush 的那批会丢；`Stop()` 在优雅关机时 flush（由
`Test_KafKaWriter_BatchMode_ShutdownFlush` 验证）。保持 `BatchFlushInterval` 较小。监控：
`WriterMetrics.Batches`（flush 次数）和 `BatchMax`（最大单批），逐条模式下为 0。

**kafka→log4go 监控桥**：`WriterMetrics` 还暴露底层 `kafka.Producer` 的实时缓冲深度：
`InFlight`（linger 积压条数）、`BufferedBytes`（缓冲内存）、`Backend`（"sarama"/"franz-go"），
这些是 `Queued`（channel 深度）单独看不到的。完整深度（UTC `Timestamp`、`Linger`、
`MaxBufferedRecs`、各字节计数器）调 `KafKaWriter.ProducerSnapshot()` → `kafka.ProducerSnapshot`
（nil 安全；对生产者做 `kafka.SnapshotHistory` 类型断言可取趋势样本）。由
`Test_KafKaWriter_ProducerMetricsBridge` 验证。

随附的健壮性修复：`k.run` 现在在守护 goroutine 启动*之前*于 `Start()` 中设置，使 `Stop()` 在
`Start()` 返回后立即可用（之前若守护尚未调度，Stop 会 no-op 并丢弃未 flush 的批次，这是 shutdown
测试在 `-cover` 下暴露的竞争）。

## 23. 溢出策略：drop vs spill——性能与数据影响（2026-06-29）

### 热路径（channel 未满，99%+ 时间）

**drop 与 spill 完全一致，零差异。** 两者都做一次非阻塞 channel send。策略只在 channel 溢出时才有区别。

### 溢出行为（channel 满）

| | drop | spill（ring） | spill（file） |
|---|---|---|---|
| Write 成本 | O(1)：计数器自增 | O(1)：mutex + slice append | O(disk)：文件写 |
| 数据 | **永久丢失** | 存入内存环 | **写入磁盘** |
| 进程崩溃 | 丢失 | 丢失 | **存活**（下次 Start 恢复） |
| 恢复成本 | 无 | 守护从环重投 | 守护从文件重投 |
| 内存 | 零 | 环容量 × 记录大小 | 零（在盘上） |
| 磁盘 | 零 | 零 | 受 SpillMaxBytes 限制 |

### 100K QPS 下真实数据丢失（BufferSize=1024）

| 事件 | 时长 | 积压 | drop 丢 | spill(ring 1024) 丢 | spill(file) 丢 |
|---|---|---|---|---|---|
| GC 暂停 | 10ms | 1,000 | 0 | 0 | 0 |
| broker 变慢 | 100ms | 10,000 | ~9,000 | ~8,000 | **0** |
| broker 宕机 | 10s | 1,000,000 | ~999,000 | ~998,000 | 受 SpillMaxBytes 限制 |
| 进程崩溃 | — | — | 全部 | 全部（内存） | **磁盘记录存活** |

### 配置常量（用这些代替魔法字符串）

```go
// 溢出策略
OverflowPolicyDrop    // "drop"  — 快、丢数据。默认。广告/RTB 日志。
OverflowPolicyBlock   // "block" — 背压，可能拖慢热路径。不适合 RTB。
OverflowPolicySpill   // "spill" — 持久化恢复。金钱/关键数据。

// spill 类型（policy == spill 时）
SpillTypeRing         // "ring"  — 内存、快、崩溃丢失。默认。
SpillTypeFile         // "file"  — 落盘、抗崩溃。
SpillTypeChain        // "chain" — ring→file→drop。兼顾两者。
```

### 按档位推荐配置

```go
// RTB 日志：drop（最快，可丢）
log4go.KafKaWriterOptions{
    OverflowPolicy: log4go.OverflowPolicyDrop,
    BufferSize:     1024,
    Acks:           kafka.AcksLeader,
}

// 转化/花费：spill(file) + acks=all（持久）
log4go.KafKaWriterOptions{
    OverflowPolicy: log4go.OverflowPolicySpill,
    SpillType:      log4go.SpillTypeFile,
    SpillDir:       "/var/log/kafka-spill",
    SpillMaxBytes:  256 << 20, // 256MB
    BufferSize:     8192,
    Acks:           kafka.AcksAll,
}
```

## 24. 广告事件策略、漏斗与容量估算（2026-06-30）

广告事件类型在量、价值、丢得起程度上各不相同。log4go + kafka 栈支持按 writer 配置，让每条流
得到合适的持久化/吞吐权衡。本节把事件价值与行业漏斗比例对应起来，在三个规模上估算
payload / 磁盘 / 内存 / 带宽 / CPU，最后给出具体配置与合规说明。

### 事件价值矩阵——各档位为何如此

| 事件类型 | 量级 | 单条价值 | 丢得起程度 | 计费影响 | 档位 |
|---|---|---|---|---|---|
| RTB 竞价 RR（请求/响应） | 极大（~1M QPS） | 仅调试 | 高 | 无 | 档位 1：最大吞吐 |
| 曝光 | 很高（~100K-500K/s） | 低（CPM 聚合） | 中（MRC 允许 1-5% 丢失） | 单条极小 | 档位 1：最大吞吐 |
| 点击 | 中（~1K-10K/s） | 中（CPC 计费 + 反欺诈） | 低 | 每次点击收入 | 档位 2：均衡 |
| 转化 | 低（~10-100/s） | 高（CPA 收入归因） | 很低 | 直接收入损失 | 档位 3：持久 |
| 预算/花费更新 | 很低（~1-10/s） | 关键（超支风险） | 无 | 直接金钱损失 | 档位 3+：最大持久 |

### 来源

- [AWS RTB Fabric](https://aws.amazon.com/blogs/industries/next-generation-programmatic-advertising-how-aws-rtb-fabric-redefines-the-game/): 90% 不出价；OpenRTB 竞价请求均值 ~3KB
- [Digital Applied](https://www.digitalapplied.com/blog/programmatic-advertising-statistics-2026-data-points): DSP 胜出率 6.3%（公开竞价），14.7%（PMP）
- [SmartInsights 2024](https://www.smartinsights.com/internet-advertising/internet-advertising-analytics/display-advertising-clickthrough-rates/): 展示广告 CTR 0.46%
- [CXL](https://cxl.com/guides/click-through-rate/benchmarks/): 展示广告 CTR 0.57%
- [Enstacked](https://enstacked.com/average-conversion-rate-for-google-ads/): 展示广告点击到留资 CVR 0.89%
- [Google Ad Manager](https://support.google.com/admanager/answer/1733124): 压缩后 25-35 字节/事件
- [AdMonsters LLD](https://www.admonsters.com/use-cases-log-level-data/): LLD 0.5-2KB Avro，2-5KB JSON
- [IAB OpenRTB 2.6](https://iabtechlab.com/wp-content/uploads/2022/04/OpenRTB-2-6_FINAL.pdf): 规范

### 行业漏斗——已核实的转化率

| 步骤 | 从 → 到 | 区间 | 均值 | 占顶层 | 来源 |
|---|---|---|---|---|---|
| 竞价 | 请求 → 响应 | 5-40% | **15%** | 15% | AWS: 90% 不出价 |
| 胜出 | 响应 → 胜出 | 1-20% | **6.3%** | 0.95% | Digital Applied |
| 投递 | 胜出 → 曝光 | 85-99% | **93%** | 0.88% | 行业共识 |
| CTR | 曝光 → 点击 | 0.08-1.5% | **0.46%** | 0.0041% | SmartInsights/CXL |
| CVR | 点击 → 转化 | 0.5-5% | **1.5%** | 0.000061% | Enstacked |
| 回传 | 转化 → 回传 | 100-300% | **200%** | 0.00012% | 行业共识 |

### 各阶段 payload 大小

| 阶段 | Payload | 典型字段 | 来源 |
|---|---|---|---|
| RTB RR 日志 | **500B** | req_id, SSP, auction_id, bid_price, creative_id, user_hash, device, ts（15-20 字段） | 由 3KB OpenRTB 压缩（AWS） |
| 竞价响应日志 | **300B** | req_id, resp_price, creative, deal_id, ts（10-12 字段） | RR 的子集 |
| 胜出通知 | **150B** | req_id, clearing_price, currency, ts（5-7 字段） | 无 |
| 曝光 | **400B** | imp_id, creative, placement, user_hash, price, geo, device, ts（15 字段） | GAM: 200-500B 未压缩 |
| 点击 | **750B** | click_id, imp_id, user_hash, landing_url, referrer, device, ts（20 字段） | LLD: 0.5-2KB Avro |
| 转化 | **1,500B** | conv_id, click_id, imp_id, revenue, currency, product, attribution, PII, ts（25 字段） | LLD: 2-5KB JSON |
| 回传 | **750B** | postback_id, conv_id, partner, event_type, SKAN, ts（15 字段） | 无 |

### 三个规模估算：100K / 500K / 1M 竞价 QPS

漏斗比例恒定，绝对量随竞价 QPS 线性增长。

#### 100K 竞价 QPS（中小 DSP）

| 阶段 | 转化 | QPS | Payload | MB/s | GB/天 | GB/天 RF3+LZ4 | 内存 | TTL | 磁盘 |
|---|---|---|---|---|---|---|---|---|---|
| RTB RR | — | 100K | 500B | **47.7** | 4,111 | 6,166 | 3.9MB | 1天 | **6.2TB** |
| 竞价响应 | 15% | 15K | 300B | 4.3 | 370 | 555 | 2.4MB | 1天 | 0.6TB |
| 胜出 | 6.3% | 945 | 150B | 0.13 | 11.6 | 17.4 | 1.7MB | 7天 | 0.12TB |
| 曝光 | 93% | 879 | 400B | 0.34 | 29.1 | 43.6 | 4.9MB | 30天 | 1.3TB |
| 点击 | 0.46% | 4.0 | 750B | 0.003 | 0.25 | 0.38 | 4.6MB | 90天 | 34GB |
| 转化 | 1.5% | 0.06 | 1.5KB | 0.0001 | 0.008 | 0.012 | 9.2MB | 365天 | 4.4GB |
| 回传 | 200% | 0.12 | 750B | 0.0001 | 0.008 | 0.012 | 2.3MB | 365天 | 4.4GB |
| **合计** | | | | **52.5** | **4,522** | **6,784** | **29MB** | | **~8.3TB** |

#### 500K 竞价 QPS（中大 DSP）

| 阶段 | 转化 | QPS | Payload | MB/s | GB/天 | GB/天 RF3+LZ4 | 内存 | TTL | 磁盘 |
|---|---|---|---|---|---|---|---|---|---|
| RTB RR | — | 500K | 500B | **238.4** | 20,553 | 30,830 | 3.9MB | 1天 | **30.8TB** |
| 竞价响应 | 15% | 75K | 300B | 21.4 | 1,849 | 2,774 | 2.4MB | 1天 | 2.8TB |
| 胜出 | 6.3% | 4,725 | 150B | 0.67 | 57.9 | 86.9 | 1.7MB | 7天 | 0.6TB |
| 曝光 | 93% | 4,394 | 400B | 1.68 | 144.8 | 217.2 | 4.9MB | 30天 | 6.5TB |
| 点击 | 0.46% | 20.2 | 750B | 0.014 | 1.24 | 1.86 | 4.6MB | 90天 | 167GB |
| 转化 | 1.5% | 0.30 | 1.5KB | 0.0004 | 0.039 | 0.059 | 9.2MB | 365天 | 22GB |
| 回传 | 200% | 0.60 | 750B | 0.0004 | 0.039 | 0.059 | 2.3MB | 365天 | 22GB |
| **合计** | | | | **262.2** | **22,637** | **33,911** | **29MB** | | **~40.9TB** |

#### 1000K（1M）竞价 QPS（大型 DSP）

| 阶段 | 转化 | QPS | Payload | MB/s | GB/天 | GB/天 RF3+LZ4 | 内存 | TTL | 磁盘 |
|---|---|---|---|---|---|---|---|---|---|
| RTB RR | — | 1M | 500B | **476.8** | 41,106 | 61,660 | 3.9MB | 1天 | **61.7TB** |
| 竞价响应 | 15% | 150K | 300B | 42.7 | 3,698 | 5,548 | 2.4MB | 1天 | 5.5TB |
| 胜出 | 6.3% | 9,450 | 150B | 1.34 | 115.8 | 173.7 | 1.7MB | 7天 | 1.2TB |
| 曝光 | 93% | 8,789 | 400B | 3.35 | 289.6 | 434.4 | 4.9MB | 30天 | 13.0TB |
| 点击 | 0.46% | 40.4 | 750B | 0.029 | 2.47 | 3.71 | 4.6MB | 90天 | 334GB |
| 转化 | 1.5% | 0.61 | 1.5KB | 0.0009 | 0.078 | 0.117 | 9.2MB | 365天 | 43GB |
| 回传 | 200% | 1.21 | 750B | 0.0009 | 0.078 | 0.117 | 2.3MB | 365天 | 43GB |
| **合计** | | | | **524.3** | **45,270** | **67,621** | **29MB** | | **~81.8TB** |

### 并排对比

| 指标 | 100K QPS | 500K QPS | 1M QPS | 规模 |
|---|---|---|---|---|
| RTB RR MB/s | 47.7 | 238.4 | 476.8 | 1× / 5× / 10× |
| 曝光 QPS | 879 | 4,394 | 8,789 | |
| 点击 QPS | 4.0 | 20.2 | 40.4 | |
| 转化 QPS | 0.06 | 0.30 | 0.61 | |
| 总 GB/天（RF3+LZ4） | 6,784 | 33,911 | 67,621 | |
| 总磁盘（含 TTL） | 8.3TB | 40.9TB | 81.8TB | |
| 总 log4go 内存 | 29MB | 29MB | 29MB | **恒定** |
| 所需 Kafka broker | 3-5 | 6-12 | 12-20 | 随 RTB 量增长 |

**内存在所有规模下恒定（29MB）**，buffer/spill 大小由配置固定，不随 QPS 变。只有 Kafka 磁盘和
broker 数随竞价 QPS 增长。

### 各阶段配置（所有规模通用）

| 阶段 | Batch | Size | Buffer | 溢出 | Spill | Acks | 内存 |
|---|---|---|---|---|---|---|---|
| RTB RR | true | 2048 | 8192 | drop | — | leader | 3.9MB |
| 竞价响应 | true | 1024 | 8192 | drop | — | leader | 2.4MB |
| 胜出 | true | 128 | 4096 | spill | 8192 | leader | 1.7MB |
| 曝光 | true | 128 | 4096 | spill | 8192 | all | 4.9MB |
| 点击 | true | 8 | 2048 | spill | 4096 | all | 4.6MB |
| 转化 | false | — | 4096 | spill | 2048 | all | 9.2MB |
| 回传 | false | — | 2048 | spill | 1024 | all | 2.3MB |

```go
// 档位 1（RTB/曝光）：drop + leader，永不阻塞竞价循环
log4go.KafKaWriterOptions{
    ProducerTopic:  "impressions",
    OverflowPolicy: log4go.OverflowPolicyDrop,
    BufferSize:     4096,
    BatchMode:      true,
    BatchSize:      128,
    Acks:           kafka.AcksLeader, // 默认；接受偶发 leader 故障丢数据
}

// 档位 3（转化/预算）：spill(chain) + acks=all，持久化的金钱数据
log4go.KafKaWriterOptions{
    ProducerTopic:  "conversions",
    OverflowPolicy: log4go.OverflowPolicySpill,
    SpillType:      log4go.SpillTypeChain, // ring -> file -> drop
    SpillSize:      2048,
    SpillDir:       "/mnt/spill",          // 容器内必须是挂载卷
    SpillMaxBytes:  128 << 20,             // 128MB
    BufferSize:     4096,
    BatchMode:      false,                 // 量小；逐条发送
    Acks:           kafka.AcksAll,         // 等待所有 ISR 副本
}
```

### 各档位设计理由

- **档位 1（drop + leader）**：最快路径，永不阻塞竞价循环。广告服务端日志本身按 MRC 就有
  ~1-5% 丢失，丢一条曝光日志不会让 CPM 聚合失真。Kafka RF=2，min.insync=1。
- **档位 2（点击，大 buffer + 小 batch）**：点击量比曝光低 10-100×，更大的 buffer 能在 broker
  卡顿时多扛一会儿不丢；更小的 batch 降低反欺诈管线的延迟。Kafka RF=3，min.insync=1（点击丢失
  可通过与广告服务端对账发现）。
- **档位 3（spill + acks=all）**：每条转化在 ack 前复制到 RF=3。spill(chain) 在 broker 宕机期间
  缓冲记录不丢；小批量/不攒批让归因延迟更低。Kafka RF=3，min.insync=2，预算状态用
  cleanup.policy=compact。

### BatchMode 阈值取决于 acks

acks=all 的每次调用往返（~2ms）比 acks=leader（~0.3ms）慢约 7×。因此 acks=all 下攒批在低得多的
QPS 就开始划算：

| acks | 每次调用 | BatchMode 在此之上划算 | 例子 |
|---|---|---|---|
| leader | ~0.3ms | ~5K/s | 曝光、RTB |
| all | ~2ms | ~50/s | 点击，乃至低量 |

### SpillSize 公式

SpillSize = QPS × 最大停顿秒数。QPS 越低，同样的环能扛更长的停顿。

| 阶段 | QPS | 目标停顿 | SpillSize | 环内存 |
|---|---|---|---|---|
| 曝光 | 25K | 500ms | 16384 | 6.5MB |
| 点击 | 200 | 30s | 8192 | 6.1MB |
| 转化 | 6 | 10 分钟 | 4096 | 6.1MB |

### 容器环境——持久化策略

容器（Docker/K8s）里，本地文件 spill 除非在挂载卷上，否则不可靠。正确的持久化模型：

| 层 | 保护什么 | 容器内安全？ |
|---|---|---|
| kafka acks=all + RF=3 | broker ack 后的记录 | ✅ 始终安全 |
| log4go spill(ring) | broker 宕机期间的记录（进程存活） | ✅ 内存 |
| log4go spill(file) 在 overlay FS | broker 宕机期间的记录（进程存活） | ⚠️ 容器重建即丢 |
| log4go spill(file) 在挂载 emptyDir | broker 宕机 + 容器重启期间的记录 | ✅ 仅同 pod |
| log4go spill(file) 在挂载 PV（EBS/NFS） | 记录在 pod 重调度后存活 | ✅ 完全持久 |

**建议**：对金钱/关键数据，以 kafka acks=all + RF=3 作为主要持久化保证。用 spill(ring) 做
broker 宕机时的临时缓冲。只有在 broker 预计长时间不可达时，才在正确挂载的 PV 上用 spill(file)。

### Kafka 集群侧配置（非 kit4go，但策略需要）

| 档位 | RF | min.insync | compaction | 说明 |
|---|---|---|---|---|
| 档位 1（曝光） | 2 | 1 | delete（TTL 3-7天） | 高吞吐、短留存 |
| 档位 2（点击） | 3 | 1 | delete（TTL 30天） | 反欺诈窗口 |
| 档位 3（转化） | 3 | 2 | delete（TTL 90天+） | 审计/合规留存 |
| 预算状态 | 3 | 2 | compact | 只留最新状态（不留历史） |

### 磁盘：1 天 vs 全留存（RF=3，LZ4 压缩）

**1 天量（所有阶段、所有规模）**

| 阶段 | 100K QPS | 500K QPS | 1M QPS | TTL |
|---|---|---|---|---|
| RTB RR | 6,166 GB | 30,830 GB | 61,660 GB | 1天 |
| 竞价响应 | 555 GB | 2,774 GB | 5,548 GB | 1天 |
| 胜出 | 2.5 GB | 12.4 GB | 24.8 GB | 1天 |
| 曝光 | 1.5 GB | 7.2 GB | 14.5 GB | 1天 |
| 点击 | 0.004 GB | 0.021 GB | 0.041 GB | 1天 |
| 转化 | 0.00003 GB | 0.00016 GB | 0.00032 GB | 1天 |
| 回传 | 0.00003 GB | 0.00016 GB | 0.00032 GB | 1天 |
| **1 天合计** | **6,725 GB** | **33,624 GB** | **67,248 GB** | |
| | **6.6 TB** | **32.8 TB** | **65.7 TB** | |

**全留存量（各阶段按自身 TTL）**

| 阶段 | TTL | 100K QPS | 500K QPS | 1M QPS |
|---|---|---|---|---|
| RTB RR | 1天 | 6.2 TB | 30.8 TB | 61.7 TB |
| 竞价响应 | 1天 | 0.6 TB | 2.8 TB | 5.5 TB |
| 胜出 | 7天 | 17.4 GB × 7 = 0.12 TB | 86.9 GB × 7 = 0.6 TB | 173.7 GB × 7 = 1.2 TB |
| 曝光 | 30天 | 43.6 GB × 30 = 1.3 TB | 217.2 GB × 30 = 6.5 TB | 434.4 GB × 30 = 13.0 TB |
| 点击 | 90天 | 0.38 GB × 90 = 34 GB | 1.86 GB × 90 = 167 GB | 3.71 GB × 90 = 334 GB |
| 转化 | 365天 | 0.012 GB × 365 = 4.4 GB | 0.059 GB × 365 = 22 GB | 0.117 GB × 365 = 43 GB |
| 回传 | 365天 | 0.012 GB × 365 = 4.4 GB | 0.059 GB × 365 = 22 GB | 0.117 GB × 365 = 43 GB |
| **总磁盘** | | **8.3 TB** | **40.9 TB** | **81.8 TB** |

**要点**：RTB RR 日志在任何规模都占磁盘 >90%。压缩 RTB RR 或把它的 TTL 从 1 天降到 12 小时，
总磁盘近乎减半。所有下游漏斗阶段（胜出到回传）加起来即使在 1M QPS 也 <3%。

### 网络带宽（生产者 → broker，未压缩）

| 阶段 | Payload | 100K QPS | 500K QPS | 1M QPS |
|---|---|---|---|---|
| RTB RR | 500B | 47.7 MB/s（381 Mbps） | 238 MB/s（1,905 Mbps） | 477 MB/s（3,810 Mbps） |
| 竞价响应 | 300B | 4.3 MB/s（34 Mbps） | 21.4 MB/s（172 Mbps） | 42.7 MB/s（342 Mbps） |
| 胜出 | 150B | 0.13 MB/s（1.1 Mbps） | 0.67 MB/s（5.4 Mbps） | 1.34 MB/s（10.7 Mbps） |
| 曝光 | 400B | 0.34 MB/s（2.7 Mbps） | 1.68 MB/s（13.4 Mbps） | 3.35 MB/s（26.8 Mbps） |
| 点击 | 750B | 0.003 MB/s | 0.014 MB/s | 0.029 MB/s |
| 转化 | 1.5KB | 0.0001 MB/s | 0.0004 MB/s | 0.0009 MB/s |
| 回传 | 750B | 0.0001 MB/s | 0.0004 MB/s | 0.0009 MB/s |
| **合计** | | **52.5 MB/s（420 Mbps）** | **262 MB/s（2,100 Mbps）** | **524 MB/s（4,194 Mbps）** |

LZ4（~50%）下线路带宽减半。RF=3 复制：broker 间 × 3。

### CPU 估算（log4go 守护 goroutine）

| 阶段 | QPS | Acks | 每次调用 | 不攒批 CPU | 攒批后 | Batch |
|---|---|---|---|---|---|---|
| RTB RR | 500K | leader | ~0.3ms | 15,000% 不可能 | **7%** | 2048 |
| 竞价响应 | 75K | leader | ~0.3ms | 2,250% 不可能 | **11%** | 1024 |
| 胜出 | 4.7K | leader | ~0.3ms | 142% 偏紧 | **7%** | 128 |
| 曝光 | 4.4K | all | ~2ms | 880% 不可能 | **7%** | 128 |
| 点击 | 20 | all | ~2ms | 4% 可接受 | **1%** | 8 |
| 转化 | 0.3 | all | ~2ms | 0.06% | 0.06% | off |
| 回传 | 0.6 | all | ~2ms | 0.12% | 0.12% | off |

### Kafka 分区（生产者侧建议）

| 阶段 | QPS 区间 | 分区 | 理由 |
|---|---|---|---|
| RTB RR | 100K-1M | 30-100 | 并行写；按 SSP 或小时分区 |
| 竞价响应 | 15K-150K | 12-48 | 同 topic 或与 RTB 共用 |
| 胜出 | 945-9.5K | 6-24 | 按推广计划或 SSP 分区 |
| 曝光 | 879-8.8K | 6-24 | 按版位或 user_hash 分区 |
| 点击 | 4-40 | 3-12 | 量小；按推广计划分区 |
| 转化 | 0.06-0.6 | 3-6 | 极小；按 advertiser_id 分区 |
| 回传 | 0.12-1.2 | 1-3 | 微量；单分区即可 |

经验：分区 ≈ 峰值 QPS / 1000（每个分区 ~1K QPS 上限）。

### TCP 连接（每个 DSP 实例）

7 个 KafKaWriter 实例 × 各 1 个生产者 = 7 条 bootstrap 连接 + 每个生产者最多 2 条
分区 leader 连接 = **~7-21 条 TCP 连接**。对现代内核可忽略。

### 各阶段数据丢失风险

| 阶段 | 风险来源 | 缓解 | 残余风险 |
|---|---|---|---|
| RTB RR | 溢出即 drop | 无（可丢） | 突发时可丢到 buffer 满 |
| 胜出 | broker 不可达 | spill(ring 8192) | 4.7K/s 下宕机 >1.7s 即溢出 |
| 曝光 | broker 不可达 | spill(ring 8192) + acks=all | 宕机 >1.9s 即溢出 |
| 点击 | broker 不可达 | spill(ring 4096) + acks=all | 宕机 >203s（3.4 分钟）即溢出 |
| 转化 | broker 不可达 | spill(ring 2048) + acks=all | 宕机 >17 小时即溢出 |
| 进程崩溃 | 所有阶段 | broker ack 后 acks=all；裸金属用 spill(file) | 内存丢失；挂载 PV 则文件存活 |

### 云成本粗估（AWS，量级估算）

| 项目 | 100K QPS | 500K QPS | 1M QPS |
|---|---|---|---|
| Kafka broker（m5.2xl × RF=3） | 3 台（$870/月） | 9 台（$2,600/月） | 18 台（$5,200/月） |
| EBS 存储（gp3，全留存） | 8.3 TB（$190/月） | 40.9 TB（$940/月） | 81.8 TB（$1,880/月） |
| S3 冷存档（RTB RR，>1天） | 3 TB（$70/月） | 15 TB（$340/月） | 30 TB（$690/月） |
| 网络出口 | 含 | 含 | ~$200/月 |
| **合计（粗估）** | **~$1,130/月** | **~$3,880/月** | **~$7,970/月** |

### 行业合规说明

- MRC（媒体评级委员会）：曝光测量允许广告服务端日志 1-5% 丢失。档位 1（drop + acks=leader）
  在此容忍范围内。
- IAB ads.txt/sellers.json：点击级数据完整性对供应链反欺诈重要。档位 2（更大 buffer）减少点击丢失。
- GDPR/CCPA：转化数据可能含 PII。档位 3（acks=all + RF=3）确保合规审计所需的数据完整性。
- SOC 2 / ISO 27001：金融事件日志（转化、预算）需要持久、可验证的投递。acks=all + RF=3 + min.insync=2
  达标。

### 单个 DSP 实例总内存（全部 7 个 writer）

合计 ~29MB，在 1-4GB 容器内游刃有余。任何规模都无内存压力，因为内存恒定——buffer/spill 大小
由配置固定，不随 QPS 变。

## 25. Buffer 与 BatchSize——关系与默认值（2026-06-29）

### 三层缓冲

```
Write() → [BufferSize channel] → 守护 → [BatchSize 攒批] → SendBatch → [Linger 后端] → broker
```

| 层 | 配置 | 默认 | 作用 |
|---|---|---|---|
| Buffer（channel） | BufferSize | 1024 | 解耦 Write 与守护，守护短暂卡顿时 Write 不阻塞 |
| Batch（守护） | BatchSize | 1024 | 摊销生产者调用，攒 N 条后一次 SendBatch |
| Linger（后端） | ProducerLinger | 10ms | 摊销 broker RPC，后端攒到 10ms 发一次请求 |

Buffer 与 BatchSize 相互独立。Buffer 防止 Write 阻塞；BatchSize 降低每次调用开销。内存：各自
持有自己的记录（1024 × 200B 约 200KB）。

### 默认配置（广告行业，BatchMode 关闭）

```go
// 隐式默认，无需任何选项
log4go.KafKaWriterOptions{
    // BufferSize:          1024,   // 默认
    // BatchMode:           false,  // 默认（逐条 Send）
    // OverflowPolicy:      "drop", // 默认
    // Acks:                "",     // 默认 = leader
}
```

BatchMode 关闭：BufferSize 是唯一的 buffer。1024 条在 100K QPS 下 ≈ 10ms 余量。drop 策略下
10ms 的守护停顿零丢失；100ms 停顿约丢 9K 条（曝光可接受，转化不可）。

### 何时改默认值

| 场景 | BufferSize | BatchSize | BatchMode | 原因 |
|---|---|---|---|---|
| RTB 日志/曝光（默认） | 1024 | 1024 | false | 可丢、最简 |
| 高吞吐曝光 | 4096 | 1024 | true | BatchMode 减少生产者调用 |
| 点击（反欺诈敏感） | 4096 | 512 | true | 更大 buffer、更小 batch 降延迟 |
| 转化（金钱） | 8192 | 256 | true | 最大 buffer 余量、spill(chain) + acks=all |
| BatchMode 关 + block 策略 | 8192+ | 无 | false | block 需要大 buffer 以免过早背压 |

经验：BufferSize >= BatchSize。Buffer 吸收守护停顿；BatchSize 控制生产者调用频率，两者不共享内存。
