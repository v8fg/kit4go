# log4go Design Principles & Implementation Guidelines
# log4go 设计原则与实现规范

---

## Overview | 概述

This document defines the **design principles** that guide every decision in
log4go — what to build, what to refuse, and how to evaluate trade-offs. It is
the arbiter when "should we add X?" is debated. Every contributor should read
this before proposing changes.

本文档定义指导 log4go 每个决策的**设计原则**——建什么、拒绝什么、如何取舍。
"要不要加 X" 有争议时以此为准。贡献者改动前必读。

---

## 1. log4go Does ONE Thing: Log | 只做一件事：日志

log4go is a **structured logging library**. It captures log records, serializes
them, and ships them to sinks (Kafka/file/console/net). Everything else is
**out of scope** and belongs to other systems.

log4go 是**结构化日志库**。它捕获记录、序列化、投递到 sink。其他一切**不在范围内**。

| log4go does | log4go does NOT do |
|-------------|-------------------|
| Capture + serialize + ship log records | Distributed tracing (→ OpenTelemetry) |
| Deterministic sampling (source-side) | Cross-service trace aggregation (→ backend) |
| Per-writer control (pause/resume/level) | ES index routing logic (→ consumer) |
| Metrics counters (Occurred/Written/Dropped) | Metric alerting channels (→ PagerDuty/Slack) |
| Correlation ID tagging (trace_id/request_id) | Error-code business semantics (→ service) |
| Status snapshot for ops dashboards | Big-data analysis (→ Spark/Flink) |
| Copy-on-write field management | Field validation / schema enforcement |

**When in doubt: "Is this a logging concern?" If no → refuse.**

**拿不准时问："这是日志的职责吗？" 不是 → 拒绝。**

---

## 2. Hot Path is Sacred | 热路径神圣不可侵犯

The delivery path (`deliverRecordToWriter`) runs **every record** at ad-tech
scale (100k–1M+/sec). Its performance budget is measured in **nanoseconds**.

投递路径（`deliverRecordToWriter`）对**每条记录**运行（100k–1M+/sec）。性能预算以
**纳秒**计。

### Rules

| Rule | Enforcement |
|------|-------------|
| Zero or ≤1 alloc per record | `TestAllocBudget_HotPath` CI gate |
| No mutex on the hot path | Use atomic ops (atomic.Int32, atomic.Pointer, atomic.Bool) |
| No I/O on the hot path | Writers are async + bounded + buffered |
| No blocking on full channel | enqueue selects on quit (never blocks indefinitely) |
| Optional features add zero cost when off | nil-check (predicted-not-taken) or atomic load |
| Benchmark regression gate | `go test -bench` vs baseline |

### What goes on the hot path (approved)

- `level.Load()` — atomic read
- `sampleDrop.Load()` — atomic read (sampling verdict, cached at WithContext)
- `priorityLevel.Load()` — atomic read (error protection threshold)
- `occurredByLevel[level]++` — atomic increment
- `sampler.Load()` → `allow(level)` — atomic load + atomic counter
- `enqueue(r)` — `select { case records <- r: case <-quit: }`
- Format (JSON/text) — cached on `r.formattedBytes` (once per record, shared by all writers)

### What does NOT go on the hot path

- `SetBaseField` / `RemoveBaseField` / `ClearBaseFields` — copy-on-write, but
  called infrequently (startup + occasional runtime change), not per-record.
- `Status()` — aggregates snapshots for ops display, not per-record.
- Strategy evaluation (`ShouldLog`) — evaluated **once at WithContext** (per
  request), cached as `sampleDrop`. Never per-record.

---

## 3. Orthogonality — No Implicit Side Effects | 正交性 — 无隐式副作用

Every operation should have **one clear effect**, with no surprise impact on
other state. If an operation has side effects beyond its stated purpose, it is
a design bug.

每个操作应只有**一个明确效果**，对其他状态无意外影响。有隐式副作用 = 设计 bug。

### Case study: SetBaseFields (removed)

```
SetBaseField(key, val)     → adds/updates ONE key; others untouched ✓
SetBaseFields(map)         → REPLACES ALL; others DELETED ✗ (surprise!)
```

The old `SetBaseFields` was removed because it violated this principle: a caller
who understood `SetBaseField` (upsert) would reasonably expect `SetBaseFields`
(batch upsert), not a full replace. The fix: three orthogonal single-key
operations (`SetBaseField` / `RemoveBaseField` / `ClearBaseFields`).

### How to evaluate a new API

1. Does it have exactly ONE effect? (If two, split into two methods.)
2. Is its behavior predictable from its name + the behavior of sibling methods?
3. Does it touch state beyond its stated scope?

If any answer is "no" → redesign.

---

## 4. Least Surprise — Names Must Tell the Truth | 最小惊讶 — 名字必须诚实

A method's name should **fully describe** its behavior. If you need to read the
doc comment to know what it does (beyond what the name says), the name is wrong.

方法名应**完整描述**其行为。需要读注释才能理解（超出名字字面意思）= 名字错了。

| Bad name | Why | Good name |
|----------|-----|-----------|
| `SetBaseFields(map)` | "Set" implies upsert (like SetBaseField), but it replaces all | (removed — use SetBaseField per key) |
| `Reload(config)` | Implies config-file reload; actually rebuilds the whole logger | (removed — SetupLog is the one-shot config entry) |
| `Pause()` on Logger | Which writer? Ambiguous. | `PauseWriter(name)` — explicit target |

---

## 5. Industry-Aligned, Not NIH | 对标业界，不重新发明

log4go adopts industry standards rather than inventing its own:

log4go 采用业界标准，不重新发明：

| Concern | Standard log4go follows |
|---------|------------------------|
| Sampling algorithm | OpenTelemetry `TraceIDRatioBased` (hash id → uint64 → threshold) |
| Cross-service consistency | W3C `traceparent` sampled flag (head-based, propagated) |
| Correlation IDs | `trace_id` / `request_id` / `device_id` (ECS + OTel semconv) |
| Field naming | Elastic Common Schema (ECS) flat snake_case |
| Shutdown safety | `records` never closed; `quit` channel for retirement |
| Sampling quality | Google Dapper insight: "1% is enough for P99; quality > quantity" |

When choosing between a custom solution and a standard, **prefer the standard**
even if slightly less optimal — compatibility + familiarity + cross-language
consistency > micro-optimization.

---

## 6. Don't Over-Engineer | 不过度设计

This principle has been invoked repeatedly throughout log4go's evolution (and
has removed more code than it added):

此原则在 log4go 演进中被反复执行（删的代码比加的多）：

| Removed | Why |
|---------|-----|
| `Reload(config)` + `ReloadFile(path)` | Delete-and-rebuild model; mainstream libs use granular setters |
| `inheritRuntimeState` | Only needed by Reload; removed with it |
| `SetBaseFields(map)` | Inconsistent with SetBaseField; replaced by orthogonal ops |
| In-process tail sampling buffer | Backend/collector's job, not a logging library's |
| OTLP exporter adapter | OTel Collector ingests JSON via Kafka; adapter = redundant |
| Local `ExportTrace(id)` API | Unrealistic across 1–10 services; backend queries by ID |
| Per-writer `SamplingWriter` | ES is downstream of Kafka, not a log4go writer |
| Disk spill store for capture | Memory-bounded only; disk is the backend's job |

### Decision framework

Before adding a feature, ask:

1. **Is it logging?** (Principle 1) If no → refuse.
2. **Is there an industry standard?** (Principle 5) If yes → align, don't invent.
3. **Can the caller do it with existing primitives?** If yes → don't add.
4. **Does it add hot-path cost?** (Principle 2) If yes → is it truly justified?
5. **Does it violate orthogonality?** (Principle 3) If yes → redesign.
6. **Can it be done by the backend/consumer/OTel instead?** If yes → let them.

---

## 7. Verification is Mandatory | 验证是强制的

Code without verification is speculation. Every change must be verified:

没有验证的代码是猜测。每次改动必须验证：

| Verification type | How | Gate |
|-------------------|-----|------|
| Functional correctness | Unit tests | 100% coverage |
| Concurrency safety | `go test -race` | Zero data races |
| Stability | `go test -count=N` | Zero flaky failures (N≥5) |
| Allocation budget | `TestAllocBudget_HotPath` | ≤1 alloc/op |
| Performance | `go test -bench` | No regression vs baseline |
| Goroutine leak | `goleak.VerifyNone` | No leaked goroutines |

**A PR without all of the above is not ready.**

---

## 8. Shutdown Safety | 关停安全

log4go runs inside long-lived services. Shutdown must be:

log4go 运行在长生命周期服务里。关停必须：

- **Idempotent**: `Stop()` / `Close()` safe to call multiple times.
- **Race-free**: concurrent `Close()` + logging never panics (quit-based
  retirement, records never closed).
- **No leak**: every goroutine (daemon, bootstrap, drainer) exits on shutdown
  (verified by goleak).
- **Drain**: in-flight records are delivered before writers stop (bootstrap
  drains the records channel on quit).
- **Resource release**: file handles, network connections, Kafka producers —
  all closed (Stopper interface, called by Logger.Close).

---

## Decision Log | 决策日志

Key architectural decisions and their rationale:

| Decision | Rationale | Principle |
|----------|-----------|-----------|
| Remove Reload; use granular setters | Reload = delete-rebuild = mainstream libs don't do it | 1, 6 |
| Atomic.Pointer for sampler/strategy | Lock-free hot path | 2 |
| WithContext caches sampleDrop | Strategy evaluated once per request, not per record | 2 |
| Written counter in bootstrap | Single-writer, no caller-path contention | 2 |
| PriorityLevel(ERROR) threshold | Industry-standard error protection (Stripe/Dapper) | 5 |
| Remove SetBaseFields | Inconsistent with SetBaseField; violates orthogonality | 3, 4 |
| quit channel (not close records) | Eliminates send-on-closed race | 8 |
| OTel for tracing, not log4go | log4go logs; OTel traces — separation of concerns | 1, 5 |
| Kafka → consumer → ES (not log4go→ES) | Business systems don't write 3rd-party components directly | 1, 6 |

---

## Sources | 参考来源

- [Elastic Common Schema (ECS)](https://www.elastic.co/guide/en/ecs/current/ecs-field-reference.html)
- [OpenTelemetry Sampling](https://opentelemetry.io/docs/concepts/sampling/)
- [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/resource/)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
- [Google Dapper Paper](https://research.google/pubs/pub36356/)
- [uber-go/zap](https://pkg.go.dev/github.com/uber-go/zap) — AtomicLevel
- [Effective Go](https://go.dev/doc/effective_go) — Go naming + API conventions
