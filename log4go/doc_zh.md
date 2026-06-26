# log4go 中文文档

> 结构化、高性能、内存安全、可观测的 Go 日志库。
> 英文版（godoc）见 [`doc.go`](doc.go)；性能与架构细节见 [`PERFORMANCE.md`](PERFORMANCE.md) / [`PERFORMANCE.en.md`](PERFORMANCE.en.md)。

## 简介

log4go 是一个生产级 Go 日志库：多 writer（Console/File/Kafka/Net/IO/Webhook）、结构化字段（对标 zap.Field / slog.Attr 的**类型化零装箱**）、JSON/logfmt/text 三格式、溢出防 OOM（ring→file→drop 多级 + 崩溃恢复）、多核分片（ShardLogger）、严格排序（unixNano+seq）、告警（WebhookWriter + RateAlerter）、slog 生态接入。

- **来源**：`github.com/xwi88/log4go`（原 GPLv3），由原作者 xwi88 集成进 kit4go 并改 MIT 许可。
- **跨平台**：纯 Go，无 cgo；Mac/Linux/Windows/容器通用。

## 能力一览（对标 zap / zerolog / slog）

| 能力 | log4go | zap | zerolog | slog |
|---|---|---|---|---|
| 类型化字段（零装箱） | ✓ | ✓ | ✓ | ✓ |
| 单核 JSON 吞吐 | ~210-350ns/3字段 | 快 | 最快(基准) | 中 |
| 多目的地不同级别 | ✓ | ✓ Tee | ✓ | ✓ |
| **溢出防 OOM（ring→file→drop）** | ✓ 内置 | 需自建 |  |  |
| **崩溃恢复（Drain 重投）** | ✓ 内置 |  |  |  |
| **多核分片（ShardLogger + auto）** | ✓ 内置 |  |  |  |
| **严格排序（unixNano+seq）** | ✓ |  |  |  |
| **告警 webhook（级别/阈值）** | ✓ | Hook | Hook | — |
| slog 生态接入 | ✓ NewSlogHandler | 桥 |  | 原生 |
| 字段健壮性（panic/NaN/typed-nil 安全） | ✓ |  |  |  |

> **独有优势**：内置的可靠性/运维栈（溢出/恢复/分片/排序/告警）是 zap/zerolog/slog 都需自建的。

## 异步 writer 生命周期（已加固）

三个曾坑过高 QPS 配置的问题已修复：

1. **ShardLogger + async FileWriter**：`ShardLogger.Register(*FileWriter)` 在 n>1 时会 panic（明确报错）——跨分片共享一个 async writer 会 race bufio/*os.File。正确做法用 `RegisterFunc(func() Writer{...})` 每分片建独立 FileWriter。n==1 单分片仍安全。
2. **spill 策略关闭无 race**：Stop 设 closing 标志 + 关 stop 信号 + 等 daemon 排空 spill + flush + 退出；从不关 messages channel（无 close-vs-send race）。drop/block/spill 都关闭安全。
3. **单例可跨 Close 复用**：Close 把单例置 nil；下次包级调用（Register/Info/...）经 atomic CAS 重建一个新 Logger（活 bootstrap + 开 channel）。

## 结构化日志

- **Console 颜色**：默认**关闭**（`Color: false`）。生产环境纯文本无 ANSI 码（安全 grep/复制/采集）；开发终端 `Color: true` 开启，9 级各有颜色（详见 [README](README.md)）。
- **字段**：`With(key,val)` / `WithField` / `WithFields(map)` 返回子 Logger，携带键值对。文本格式末尾追加 JSON 对象；KafKaWriter 提升为顶层 JSON key。
- **JSON 格式**：`SetFormat(FormatJSON)` 每条一个 JSON 对象，预序列化到 `r.formattedBytes`，多 writer 共用（零重复序列化）。默认编码器 goccy（~2-3× encoding/json），`SetJSONCodec` 可切 std/sonic。
- **采样**：`WithSampling(initial, thereafter)` 每级别丢高频（前 initial 条全过，之后每 thereafter 一条）。采样在 Metrics 计数前丢。
- **context.Context**：`WithContext(ctx)` 从 ctx 提取字段（默认探 trace/request/user/tenant；`AddContextExtractor` 叠加自定义，含 OTel trace/baggage 无硬依赖）。zerolog 风格 `IntoContext`/`FromContext` + `RequestIDMiddleware`。

## 类型化字段 / slog / logfmt / 预设 / Panic-Fatal（Round A）

- **类型化字段（零装箱）**：`With(key,val)` 内部 `fieldOf` 把常见标量映射到无装箱 kind（string/bool/int 全宽/uint 全宽/uintptr/float/[]byte/complex/duration/time/error）；要编译期类型化用 `WithString/WithInt/.../WithBytes/WithError` 或 `WithAttrs(String(...),Int(...))`。
- **健壮性**：任何字段值安全降级——panic 的 MarshalJSON、typed-nil error、不可 marshal 的 chan/func、NaN/±Inf 一律 → null(JSON)/`-`(logfmt)，输出永远合法。[]byte→base64，complex→"a+bi"。
- **logfmt**：`SetFormat(FormatLogfmt)` → `time=... level=... msg="..." k=v`（Loki/Promtail/docker 原生）。
- **slog.Handler**：`NewSlogHandler(logger)` 适配标准库 slog——`slog.SetDefault(slog.New(log4go.NewSlogHandler(lg)))` 让 net/http 等进 log4go 管线。
- **预设**：`NewProduction()`（JSON+INFO+采样+caller）/ `NewDevelopment()`（彩色 text+DEBUG+funcname），对标 zap。
- **Panic/Fatal/Recover**：CRITICAL 输出 → `Sync()`（drain+flush）→ panic/exit；`Recover` 捕获 panic→日志→re-raise。

## Writer 速览（详见 PERFORMANCE.md §12 实测）

单 bootstrap goroutine 串行调每个 writer，端到端 QPS ≈ 1/Σ(writer Write)。挑最少 writer 集（慢 writer 拖累全部）。单核 QPS（M5/Go1.26，sink 隔离 I/O 噪音）：

| Writer | ns/op | ~QPS/核 |
|---|---|---|
| NetWriter (TCP loopback) | ~112 | 8.9M |
| FileWriter (sync) | ~127 | 7.9M |
| FileWriter (async+spill) | ~206 | 4.9M |
| ConsoleWriter (buffered) | ~118 | 8.5M |
| KafKaWriter (mock) | ~578 | 1.7M |
| ConsoleWriter (unbuffered) | ~1590 | 629K |

内存：100K 条每 writer HeapAlloc <0.005MB，goroutine 恒定 4（池复用 + 有界 channel + 有界 spiller）。

## 高性能 vs 易用 对比（cookbook）

API 按"易用 ⇄ 性能"分层，**默认已是较快**，极致吞吐只需几个开关。实测（M5/Go1.26，allocs 是热无关稳定信号，ns 为冷机峰值）：

| 档 | 用法 | allocs | ns/op | QPS/核 | 易用 | 适用 |
|---|---|---|---|---|---|---|
| **A 易用** | `NewProduction()` + `Info("msg")` | **1** | ~1037 | ~965K | 高 | 99% 业务 |
| A | `With(k,v).Info(fmt,i)` | 3 | ~1065 | ~939K | 高 | 结构化 |
| **B 高性能结构化** | `WithString/WithInt/WithAttrs` | 2-3 | ~1250 | ~800K | 中 | 高频+字段 |
| **C 极致吞吐** | `WithCaller(false)` + 无参 + async File | **1** | ~1037 | **~965K** | 低 | 超高频 |
| **D 多核线性** | `ShardLogger(0)` + `RegisterFunc` | — | shard×单核 | 4 shard ~3× | 低 | 10M 级 |

### 档 A — 易用（默认）
```go
lg := log4go.NewProduction()        // JSON + INFO + 采样 + caller + console
defer lg.Close()
lg.Info("server started")           // 无参：1 alloc（跳过 Sprintf）
lg.With("trace_id", id).Info("served %s", route)  // 带参：3 alloc
```
> `With(k, interface{})` 内部已对标量做类型推断（常见类型零装箱），所以"易用档"已经很快。无参 `Info("msg")` 比 `Info(fmt,args)` 少 2 alloc。

### 档 B — 高性能结构化（typed，对标 zap.Field）
```go
lg.WithString("trace_id", id).
    WithInt("status", 200).
    WithDuration("elapsed", dt).
    Info("served")
// 批量：
lg.WithAttrs(log4go.String("k","v"), log4go.Int("n",1)).Info("x")
```
> JSON 走 typed append（Record.JSON 1 alloc/3字段）。标量不经 codec，SetJSONCodec 对纯标量几乎无影响。

### 档 C — 极致单核吞吐（消 caller + 无参）
```go
lg := log4go.NewLogger()
lg.WithCaller(false)                // 消 caller：-2 allocs（丢 file:line）
lg.SetFormat(log4go.FormatJSON)
fw := log4go.NewFileWriterWithOptions(log4go.FileWriterOptions{
    Enable: true, Async: true, OverflowPolicy: "spill", SpillType: "ring",
})
lg.Register(fw)
lg.Info("x")                        // 无参 + 无 caller = 1 alloc/~16B
```
> hasCaller + `Info(fmt,args)` = 3 allocs（Go variadic 装箱下限）；`WithCaller(false)` + 无参 = 1 alloc，单核最大吞吐（~923K QPS/核）。

### 档 D — 多核线性（分片，10M 级）
```go
sl := log4go.NewShardLogger(0)      // 0 = auto: max(2, GOMAXPROCS/2)
// 或结构化（配置文件友好）：
//   sl := log4go.NewShardLoggerWithOptions(log4go.ShardLoggerOptions{
//       Shards: 0, Level: log4go.LevelFlagInfo, ChannelSize: 8192,
//   })
sl.RegisterFunc(func() log4go.Writer {  // 每分片独立 writer（必须工厂）
    return log4go.NewFileWriterWithOptions(log4go.FileWriterOptions{
        Enable: true, Async: true, OverflowPolicy: "spill", SpillType: "file",
    })
})
defer sl.Close()
sl.WithCaller(false).Info("imp")
```
> 慢 writer（磁盘/kafka）下分片近线性（4 shard ~3×）。快 writer（内存/discard）分片无收益。auto 读 GOMAXPROCS（Go1.25+ 尊重 cgroup）；旧环境 `import _ "github.com/v8fg/kit4go/maxprocs"`。

### 性能开关（大致按收益）
- `WithCaller(false)` — 消 caller，**-2 allocs**（最大单项收益，丢 file:line）
- 无参 `Info("msg")`（vs `Info(fmt,i)`）— -2 allocs（跳 Sprintf + 装箱）
- typed `WithString/WithInt`（vs `With(k,v)`）— 标量零装箱、类型安全
- `FileWriter{Async:true}` — daemon 隔离磁盘，不阻塞 bootstrap
- `OverflowPolicy:"spill"` — 防 OOM + 不丢热数据
- `ShardLogger(0)` — 慢 writer 下多核线性
- `ConsoleWriter{Buffered:true}` — bufio 减 syscall
- `FormatJSON` 预序列化 — `r.formattedBytes` 一次序列化多 writer 共用

### 反模式（避免）
- 生产用 unbuffered Console（阻塞 bootstrap）
- 跨 shard 共享 `*FileWriter`（race；必须 `RegisterFunc`）
- 高频 `With("count", i)`（装箱；用 `WithInt`）
- 每条 log `Sprintf` 大对象（预格式化或字段）
- NetWriter 高吞吐（RTT-bound；用 File + Kafka）

## 监控诊断

- **启动**：`NewShardLogger` 输出 `[log4go] ShardLogger started: GOMAXPROCS=N shards=M`。
- **运行时**：`log4go.RuntimeStats()` → GOMAXPROCS/NumGoroutine/HeapAlloc/HeapInuse/NumGC/GCCPUFraction（按需采集，调 ReadMemStats 有 STW，**不进热路径**）。
- **日志量**：`Metrics()` per-level 计数；`kw.Metrics()` Kafka Sent/Errored/Dropped/Spilled。
- **事件**：`SetOnEvent` 实时增量 hook（Prometheus/statsd）。

## 快速开始
```go
package main

import "github.com/v8fg/kit4go/log4go"

func main() {
    lg := log4go.NewProduction()
    defer lg.Close()
    lg.With("trace_id", "t-1").Info("served")
}
```
