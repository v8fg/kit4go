# log4go Field Schema & Industry Best Practices
# log4go 字段体系与行业最佳实践

---

## Overview | 概述

This document defines the **standard field schema** for log4go records shipped to
Kafka / Elasticsearch / big-data pipelines, grounded in industry standards (ECS,
OTel semconv) and cross-domain practices (ad-tech, fintech, blockchain).

本文档定义 log4go 记录写入 Kafka / ES / 大数据管道的**标准字段体系**，对标行业标准
（ECS、OTel 语义约定）并结合跨行业实践（广告、金融、区块链）。

---

## Industry Standards Reference | 行业标准参考

### Elastic Common Schema (ECS) — for Kafka → ES pipelines
### Elastic 通用架构（ECS）—— Kafka → ES 管道的最相关标准

[ECS](https://www.elastic.co/guide/en/ecs/current/ecs-field-reference.html) defines
standard field names for the Elastic Stack. When log4go records flow through
Kafka → consumer → ES, field names should align with ECS so Kibana dashboards,
index templates, and queries work out of the box.

| ECS field | Meaning | log4go equivalent |
|-----------|---------|-------------------|
| `@timestamp` | event time | `time` / `unix_nano` (auto) |
| `service.name` | service identity | `service_name` |
| `service.version` | service version | `service_version` |
| `host.name` | hostname | `host` |
| `host.ip` | host IP | `ip` |
| `log.level` | severity | `level` (auto) |
| `event.action` | event type | `log_type` |
| `event.outcome` | success/failure | `error_code` (business) |
| `cloud.region` | cloud region | `region` |
| `cloud.provider` | cloud vendor | `cloud_provider` |
| `container.id` | container ID | `instance_id` |
| `trace.id` | trace correlation | `trace_id` (auto via extractor) |

log4go uses **flat snake_case** (`service_name`) instead of ECS dotted notation
(`service.name`) for JSON ergonomics. The consumer (Fluent Bit / Logstash) can
rename to ECS dotted format via a `modify` filter if the ES mapping requires it.

log4go 使用 **flat snake_case**（`service_name`）而非 ECS 点号格式
（`service.name`），便于 JSON 处理。消费端（Fluent Bit / Logstash）可通过
`modify` filter 重命名为 ECS 点号格式。

### OpenTelemetry Semantic Conventions — for cross-language consistency
### OpenTelemetry 语义约定 —— 跨语言一致性

[OTel Resource semconv](https://opentelemetry.io/docs/specs/semconv/resource/)
defines resource attributes that are highly aligned with ECS:

| OTel attribute | ECS equivalent |
|----------------|----------------|
| `service.name` | `service.name` |
| `host.name` / `host.id` | `host.name` |
| `cloud.region` | `cloud.region` |
| `cloud.provider` | `cloud.provider` |
| `deployment.environment` | `service.environment` |

Both standards converge on the same concepts. log4go's flat constants map to both.

两个标准在概念上高度一致。log4go 的 flat 常量与两者都对齐。

---

## Field Hierarchy | 字段分层

Every record = **BASE fields** (startup, all records) + **PER-REQUEST fields**
(each request differs).

每条记录 = **BASE 字段**（启动时设、所有记录都有）+ **PER-REQUEST 字段**（每请求不同）。

```
┌─────────────────────────────────────────────────────┐
│ Record JSON (to Kafka)                               │
├──────────────┬──────────────────────────────────────┤
│ Framework    │ time, level, msg, file, unix_nano,  │ ← log4go auto (always present)
│ (framework)  │ seq, trace_id, span_id, request_id   │
├──────────────┼──────────────────────────────────────┤
│ BASE         │ service_name, host, ip, region,      │ ← SetBaseFields (startup)
│ (infra)      │ country, env, es_index, log_type...  │
├──────────────┼──────────────────────────────────────┤
│ PER-REQUEST  │ ad_id, campaign_id, impression_id,   │ ← WithField / WithContext (per request)
│ (business)   │ device_id, error_code, bid_price...  │
└──────────────┴──────────────────────────────────────┘
```

### log4go's role

| Layer | What | log4go provides |
|-------|------|-----------------|
| **BASE (infra)** | Standardized field key constants + preset templates | Constants + `ESRouting.Apply()` / `BigData.Apply()` |
| **PER-REQUEST (business)** | Carry any business fields to Kafka JSON verbatim | `WithString` / `WithInt` / `With` (zero-alloc typed) |
| **Framework** | Auto-injected (time, level, trace_id, seq...) | Always present, zero config |

log4go **does NOT** define business field constants (ad_id, tx_hash,
transaction_id — those are your domain). It standardizes **infrastructure** keys
and **carries** everything else.

log4go **不定义**业务字段常量（ad_id、tx_hash、transaction_id —— 那是你的领域）。
它只标准化**基础设施**键名 + **原样携带**其他字段。

---

## Base Field Management API | BASE 字段管理 API

Three operations, each with clearly different semantics. Understanding the
difference is critical to avoid accidentally losing fields.

三个操作，语义完全不同。理解差异至关重要——避免误删字段。

| API | Semantics | What happens to existing fields |
|-----|-----------|---------------------------------|
| `SetBaseField(key, val)` | **UPSERT** single key | Same key → value replaced; other keys → KEPT |
| `SetBaseFields(map)` | **REPLACE ALL** ⚠️ | ⚠️ Old fields GONE — the map IS the new complete set |
| `ClearBaseFields()` | **CLEAR ALL** | Everything removed; records carry no base fields |

### ⚠️ CRITICAL: SetBaseFields overwrites everything | SetBaseFields 覆盖全部

```go
// Starting state: base fields = {"a": 1, "b": 2, "c": 3}

log4go.SetBaseField("d", 4)
// → {"a":1, "b":2, "c":3, "d":4}  — "d" added, a/b/c KEPT ✓

log4go.SetBaseFields(map[string]interface{}{"x": 99})
// → {"x": 99}  — ⚠️ a, b, c, d are ALL GONE. Only "x" remains.

log4go.ClearBaseFields()
// → {}  — everything gone.

// To ADD without losing others — use SetBaseField (NOT SetBaseFields):
log4go.SetBaseField("y", 5)  // adds "y", existing fields kept
```

### When to use which | 何时用哪个

| Scenario | Use |
|----------|-----|
| Startup: set all infra fields at once from config | `SetBaseFields(fullMap)` |
| Runtime: add/update one field (e.g. new tag) | `SetBaseField(key, val)` |
| Runtime: change service_name (canary deploy) | `SetBaseField("service_name", newName)` |
| Shutdown/reset: clear everything | `ClearBaseFields()` |
| Remove one specific field | `SetBaseFields` without that key (rebuild from current) |

### Concurrency safety | 并发安全

All three are **atomic + copy-on-write** — they never mutate the snapshot the
bootstrap goroutine is reading. Safe to call concurrently with logging (verified
by `-race`).

三个操作都是**原子 + copy-on-write**——绝不修改 bootstrap 正在读取的快照。可与日志
投递并发调用（`-race` 验证通过）。

---

## Flexibility & Evolution | 灵活性与演进

Fields are **fully dynamic key-value** — log4go treats all keys as opaque
strings and carries them verbatim to Kafka JSON. There is no schema validation,
no required keys, no key-name enforcement. The constants above (`FieldServiceName`
etc.) are **convenience constants** (IDE autocomplete + cross-service consistency),
not constraints. A service can use any key name it wants.

字段是**完全动态的 key-value**——log4go 把所有 key 当作不透明字符串，原样序列化到
Kafka JSON。没有 schema 校验、没有必需的 key、没有 key 名强制。上面的常量
（`FieldServiceName` 等）是**便利常量**（IDE 补全 + 跨服务一致），不是约束。服务可以
用任意 key 名。

### Can fields change at runtime? | 字段能运行时改吗？

YES — all base field operations are atomic and take effect on the next record:

可以——所有 base field 操作都是原子的，对下一条记录立即生效：

```go
log4go.SetBaseField("new_tag", "value")   // immediately on next record
log4go.SetBaseField("env", "staging")      // change env at runtime (canary)
// No code change, no restart, no log4go release needed.
```

### Can fields evolve over time? | 字段能随版本演进吗？

YES — change `SetBaseFields` at startup; the consumer adapts:

可以——在启动时改 `SetBaseFields`；消费端适配：

| v1 fields | v2 fields | Migration |
|-----------|-----------|-----------|
| `server_ip` | `ip` (ECS aligned) | Consumer handles both (or transition period) |
| no `country` | add `country` (GDPR) | Old records lack it; new records have it |
| no `schema_version` | add `schema_version: "2.0"` | Consumer branches on version |

```go
// Optional: add a schema version for consumer-side migration
log4go.SetBaseField("schema_version", "2.0")
```

---

## Standard Field Constants (Infrastructure) | 标准字段常量（基础设施）

```go
// log4go/base_fields.go

const (
    // —— Service identity (ECS: service.*, OTel: service.*) ——
    FieldServiceName    = "service_name"     // REQUIRED. e.g. "bidder"
    FieldServiceVersion = "service_version"   // for canary/rollback tracking
    FieldEnv            = "env"              // prod / staging / dev

    // —— Host / source (ECS: host.*, OTel: host.*) ——
    FieldHostName   = "host"                // hostname
    FieldHostIP     = "ip"                  // host IP
    FieldInstanceID = "instance_id"         // Pod / container / process ID

    // —— Cloud / geo (ECS: cloud.*, geo.*; CRITICAL for cross-border) ——
    FieldCloudProvider = "cloud_provider"   // aws / gcp / aliyun / tencent
    FieldCloudRegion   = "region"           // us-east-1 / eu-west-1 / cn-beijing
    FieldCountry       = "country"          // ISO 3166-1 alpha-2 (GDPR routing)
    FieldCluster       = "cluster"          // Kubernetes cluster name

    // —— ES routing (ad-tech / custom consumer; not in ECS) ——
    FieldESIndex = "es_index"               // index name (time-based: adx-logs-2026.06)
    FieldType    = "log_type"               // business / system / impression / click

    // —— Trace correlation (auto-extracted, listed for reference) ——
    // trace_id, span_id, request_id, device_id → auto via context extractor
)
```

### Why cross-border fields matter | 跨国字段为什么重要

For global services (US + EU + APAC):
- **GDPR compliance**: EU personal data must stay in EU. Consumer routes by
  `country=DE` → EU ES cluster; `country=US` → US cluster.
- **Data residency**: some countries (China, Russia) require data to stay
  in-country. `country=CN` → China cluster.
- **Latency**: queries hit the nearest cluster by `region`.

**log4go only carries the fields**; routing is the consumer's job.

**log4go 只携带字段**；路由是消费端的事。

---

## Preset Templates | 预设模板

### ES Routing (Kafka → consumer → ES)

For services whose logs flow to Elasticsearch and need index routing:

适用于日志流向 ES 并需要索引路由的服务：

```go
log4go.ESRouting{
    ESIndex:       "adx-logs-" + time.Now().Format("2006.01"),
    ServiceName:   "bidder",
    Host:          hostname,
    IP:            localIP,
    Region:        "ap-southeast-1",
    Country:       "CN",
    Env:           "prod",
    CloudProvider: "aliyun",
    InstanceID:    podName,
}.Apply()
```

### Big Data (Kafka → Spark / Flink)

For services whose logs flow to big-data analysis (no ES fields needed):

适用于日志流向大数据分析（不需要 ES 字段）：

```go
log4go.BigData{
    ServiceName:   "impression",
    Env:           "prod",
    Region:        "us-east-1",
    CloudProvider: "aws",
}.Apply()
```

### Custom (no preset)

For services with unique requirements:

有独特需求的服务：

```go
log4go.SetBaseFields(map[string]interface{}{
    log4go.FieldServiceName: "click-service",
    log4go.FieldEnv:         "prod",
    "custom_field":          "value",  // any key
})
```

---

## Industry-Specific Business Fields | 行业业务字段

These are **PER-REQUEST** fields (set via `WithString` / `WithInt` per request).
log4go carries them verbatim — it does NOT define constants (they are business
domain). Follow the naming convention: **snake_case, full words, no ambiguous
abbreviations**.

这些是 **PER-REQUEST** 字段（每请求通过 `WithString` / `WithInt` 设置）。log4go 原样
携带 —— **不定义常量**（业务域）。命名约定：**snake_case，完整单词，无歧义缩写**。

### Ad-Tech (OpenRTB) | 广告

Standard: [IAB OpenRTB 2.5/3.0](https://iabtechlab.com/standards/openrtb/)

```go
lg.WithString("ad_id", adID).
    WithString("campaign_id", campID).
    WithString("creative_id", crID).
    WithString("placement_id", pid).
    WithString("impression_id", impID).
    WithString("deal_id", dealID).
    WithString("device_id", deviceID).
    WithString("supply_chain", schain).
    WithFloat64("bid_price", price).
    WithString("publisher_id", pubID).
    WithString("advertiser_id", advID).
    WithString("error_code", code)
```

### Fintech (PCI-DSS / ISO 20022) | 金融

Standards: PCI-DSS (data masking), ISO 20022 (message fields),
ISO 4217 (currency codes).

```go
lg.WithString("transaction_id", txID).
    WithString("masked_account", maskAcct).    // NEVER log raw account! | 绝不写明文账号！
    WithFloat64("amount", amt).
    WithString("currency", "USD").              // ISO 4217
    WithString("merchant_id", mid).
    WithString("payment_method", "card").       // card / wallet / bank_transfer
    WithInt("risk_score", score).
    WithString("compliance_flag", "passed").    // KYC / AML
    WithString("channel", "api").               // online / pos / api
    WithString("reference_id", refID)
```

**Compliance note**: PCI-DSS requires account numbers to be masked/truncated
(first 6 + last 4 digits max). log4go does NOT do masking — that is the
service's responsibility before passing the value to WithString.

**合规注意**：PCI-DSS 要求账号脱敏（最多前 6 + 后 4 位）。log4go **不做脱敏** ——
服务在传入 WithString 前自行脱敏。

### Blockchain (EVM / Bitcoin) | 区块链

Standards: Ethereum JSON-RPC, EIP-155 (chain ID), ERC standards.

```go
lg.WithString("tx_hash", hash).
    WithInt64("block_number", block).
    WithString("from_address", from).
    WithString("to_address", to).
    WithString("contract_address", addr).
    WithInt64("gas_price", gas).
    WithInt64("gas_used", gasUsed).
    WithInt("chain_id", 1).                     // 1=mainnet, 137=polygon, 56=bsc
    WithString("token_id", tokenID).            // ERC-721 / ERC-1155
    WithString("event_name", "Transfer").       // smart contract event
    WithString("wallet_id", wallet)
```

---

## Cross-Industry Field Matrix | 跨行业字段矩阵

| Category | Ad-Tech | Fintech | Blockchain |
|----------|---------|---------|------------|
| **Identity** | ad_id, campaign_id, creative_id | transaction_id, merchant_id | tx_hash, block_number |
| **User/Device** | device_id, user_id | masked_account, customer_id | wallet_id, from_address |
| **Value** | bid_price, deal_id | amount, currency | gas_price, value |
| **Routing** | placement_id, publisher_id | channel, payment_method | chain_id, contract_address |
| **Outcome** | error_code, win/loss | status, risk_score | event_name, success/reverted |
| **Compliance** | device_consent (GDPR) | masked_account (PCI-DSS) | — |

---

## Recommended RTB Configuration | RTB 推荐配置

### Startup (BASE fields)

```go
// Cross-border ad-tech with ES routing
log4go.ESRouting{
    ESIndex:       "adx-logs-" + time.Now().Format("2006.01"),
    ServiceName:   "bidder",
    Host:          os.Getenv("HOSTNAME"),
    IP:            localIP(),
    Region:        os.Getenv("DEPLOY_REGION"),  // ap-southeast-1
    Country:       os.Getenv("DEPLOY_COUNTRY"), // CN
    Env:           os.Getenv("ENV"),            // prod
    CloudProvider: os.Getenv("CLOUD"),          // aliyun
    InstanceID:    os.Getenv("POD_NAME"),
}.Apply()
```

### Per-request (business fields)

```go
lg := log4go.WithContext(ctx).
    WithString("ad_id", bid.AdID).
    WithString("campaign_id", bid.CampaignID).
    WithString("placement_id", bid.PlacementID).
    WithString("impression_id", bid.ImpID).
    WithString("device_id", bid.DeviceID).
    WithFloat64("bid_price", bid.Price)
// ... process ...
lg.WithString("error_code", code).Info("bid served")
```

### Volume control (three layers)

```go
// 1. Level (primary, manual)
log4go.SetLevel(log4go.INFO)

// 2. Priority (error protection — industry standard)
log4go.SetPriorityLevel(log4go.ERROR)

// 3. Sampling (adjustable ratio)
log4go.SetSamplingStrategy(log4go.TraceIDRatioBased{Ratio: 0.001}) // 0.1%

// Emergency: temporary 10% for 30 min
stop := log4go.SetSamplingStrategyFor(
    log4go.TraceIDRatioBased{Ratio: 0.1}, 30 * time.Minute)
```

---

## What log4go Does NOT Do | log4go 不做的事

| Concern | Owner | Why |
|---------|-------|-----|
| ES index routing logic | Consumer | log4go carries `es_index`, consumer writes to the right index |
| Data masking (PCI-DSS) | Service | Compliance is the service's legal responsibility |
| Business field validation | Service | log4go carries any key-value; validation is business logic |
| Cross-cluster routing | Consumer | `country`/`region` fields are data; routing is infrastructure |
| Trace span tree | OpenTelemetry | log4go correlates via trace_id; spans are OTel's domain |

---

## Sources | 参考来源

- [Elastic Common Schema (ECS)](https://www.elastic.co/guide/en/ecs/current/ecs-field-reference.html)
- [OpenTelemetry Resource Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/resource/)
- [OTel Semantic Conventions for Logs](https://github.com/open-telemetry/semantic-conventions/blob/main/docs/general/logs.md)
- [IAB OpenRTB 2.5/3.0](https://iabtechlab.com/standards/openrtb/)
- [PCI-DSS Requirements](https://www.pcisecuritystandards.org/)
- [Ethereum JSON-RPC](https://ethereum.org/en/developers/docs/apis/json-rpc/)
- [Google Dapper Paper](https://research.google/pubs/pub36356/) — sampling at scale
- [Fluent Bit modify filter](https://docs.fluentbit.io/manual/pipeline/filters/modify) — ECS field renaming
