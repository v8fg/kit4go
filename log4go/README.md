# log4go

High-performance, memory-safe, observable structured logging for Go.

[中文文档](doc_zh.md) | [性能文档](PERFORMANCE.en.md)

- Typed fields (zero-boxing, like zap.Field / slog.Attr)
- Multi-writer with per-writer level filtering (Console / File / Kafka / Net / Webhook)
- Overflow protection (ring -> file -> drop) + crash recovery
- Multi-core sharding (auto-sized to GOMAXPROCS)
- JSON / logfmt / text formats, slog.Handler bridge
- Strict ordering (unixNano + seq)
- Webhook alerting (Lark / DingTalk / WeCom) with rate gating

## Quick start

```go
package main

import "github.com/v8fg/kit4go/log4go"

func main() {
    lg := log4go.NewProduction()
    defer lg.Close()
    lg.With("trace_id", "t-1").Info("served")
}
```

## Log levels — industry comparison

log4go implements RFC 5424 (syslog) + TRACE, for 9 levels.

| Level | int | RFC 5424 | slog | zap | zerolog | What to log |
|---|---|---|---|---|---|---|
| TRACE | 8 | — | — | — | yes | Individual variable values, full payloads, per-iteration detail. Troubleshooting only. |
| DEBUG | 7 | DEBUG | Debug | Debug | Debug | Function entry/exit, intermediate results, cache hit/miss. Development. |
| INFO | 6 | INFO | Info | Info | Info | Business milestones: request served, order placed, user login. |
| NOTICE | 5 | NOTICE | — | — | — | Significant normal events: config reload, leader election, cache warm done. |
| WARNING | 4 | WARNING | Warn | Warn | Warn | Degraded but functional: retry, timeout, slow query, rate-limit near. |
| ERROR | 3 | ERROR | Error | Error | Error | Operation failed, process continues: payment failed, DB write error. |
| CRITICAL | 2 | CRITICAL | — | — | — | System-level failure. Used by Panic / Fatal internally. |
| ALERT | 1 | ALERT | — | — | — | Immediate human action: security breach, AML alert. |
| EMERGENCY | 0 | EMERG | — | — | — | System unusable. Regulatory / telecom grade. |

**Constants** (use in code, not magic strings):

```go
log4go.TRACE           // 8
log4go.DEBUG           // 7
log4go.INFO            // 6
log4go.NOTICE          // 5
log4go.WARNING         // 4
log4go.ERROR           // 3
log4go.CRITICAL        // 2

// string form for config files:
log4go.LevelFlagInfo      // "INFO"
log4go.LevelFlagWarning   // "WARNING"
log4go.LevelFlagError     // "ERROR"
```

## Console colors

**OFF by default.** ConsoleWriter outputs plain text with no ANSI escape codes —
safe for production (grep, copy, Filebeat/Fluentd collection). Enable color only
for local development terminals:

```go
// OFF (default, production-safe):
log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{Color: false})

// ON (development only):
log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{Color: true})

// Full line colored (not just the level flag):
log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{Color: true, FullColor: true})
```

Config file (JSON/YAML): `"color": false` (default) or `"color": true`.

When enabled, each level has a distinct ANSI color — severity hierarchy is
visible at a glance:

| Level | ANSI | Color | Sample |
|---|---|---|---|
| EMERGENCY | `[1;41m` | Red background (bold) | most prominent |
| ALERT | `[1;31m` | Bold red | second most urgent |
| CRITICAL | `[35m` | Magenta | serious, distinct from red |
| ERROR | `[31m` | Red | standard error |
| WARNING | `[33m` | Yellow | caution |
| NOTICE | `[32m` | Green | significant positive |
| INFO | `[36m` | Cyan | informational |
| DEBUG | `[34m` | Blue | development |
| TRACE | `[90m` | Dark grey | dimmest, finest detail |

Fixed elements: timestamp is cyan `[36m`, file:line is white-bg/black `[47;30m`,
message body has no color.

Full-color mode (`ConsoleWriterOptions{FullColor: true}`) renders the entire
line in the level color; default mode colors only the level flag bracket.

## Level combination strategies

| Strategy | Threshold | What you get | When |
|---|---|---|---|
| Default | INFO | Business flow + warnings + errors | Most services |
| High-QPS | WARNING | Warnings + errors only (skip INFO noise) | RTB, impression streams (>100K QPS) |
| Compliance / audit | NOTICE | Significant events + above | Finance, healthcare, payment |
| Development | DEBUG | Full diagnostics | Local dev, staging |
| Troubleshooting | TRACE | Everything (temporarily) | Production hot-fix |
| Multi-writer split | per-writer | Kafka=INFO, Webhook=ERROR, Console=DEBUG | One logger, multiple sinks |

Multi-writer split:

```go
lg := log4go.NewLogger()
lg.Register(kafkaWriter)               // INFO+  -> Kafka -> ES (full volume)
lg.Register(consoleWriter)             // DEBUG+ -> stdout (dev visibility)
lg.Register(log4go.NewWebhookWriter(sink, log4go.WebhookWriterOptions{
    Level: log4go.LevelFlagError,      // ERROR+ -> Lark / DingTalk (alert)
}))
```

## Industry scenarios

### Ad-tech (RTB / programmatic advertising)

Ultra-high QPS (100K-10M), real-time bidding, sub-50ms budget.

```go
log4go.SetLevel(log4go.WARNING)                    // production: skip INFO noise at 1M QPS

log4go.Trace("bid req payload=%s", rawJSON)        // troubleshooting: full bid request
log4go.Debug("campaign match adv=%s", id)           // dev: matching logic
log4go.Info("bid won imp=%s price=%d", id, p)       // filtered at WARNING threshold
log4go.Warn("bid timeout exchange=%s", name)         // kept: exchange latency
log4go.Error("creative render failed imp=%s", id)    // kept: creative issue
log4go.Fatal("bidder unreachable: %v", err)          // CRITICAL + flush + exit
```

Recommended: **production WARNING**; staging INFO; troubleshooting TRACE + WithSampling.

### Finance (trading / payment / banking)

Audit trail, compliance, latency-sensitive.

```go
log4go.SetLevel(log4go.NOTICE)                      // compliance: capture significant events

log4go.Trace("orderbook tick sym=%s bid=%d", sym, bid)
log4go.Debug("risk calc var=%v exposure=%v", var95, exp)
log4go.Info("order filled id=%s qty=%d", id, q)     // filtered at NOTICE threshold
log4go.Notice("margin call account=%s shortfall=%v", acct, s)
log4go.Warn("slippage exceeded exp=%v actual=%v", exp, act)
log4go.Error("settlement failed trade=%s", id)
```

Recommended: **production NOTICE**; ERROR+ to webhook; TRACE temporarily for matching-engine debugging.

### E-commerce (order / inventory / payment)

High traffic, multi-step transactions, seasonal peaks.

```go
log4go.SetLevel(log4go.INFO)

log4go.Trace("product view sku=%s score=%f", sku, recScore)
log4go.Debug("cart updated user=%s items=%d", uid, n)
log4go.Info("order placed order=%s total=%v", orderID, total)
log4go.Notice("low stock sku=%s remaining=%d", sku, left)
log4go.Warn("payment retry order=%s attempt=%d", orderID, attempt)
log4go.Error("payment failed order=%s gateway=%s", orderID, gw)
log4go.Fatal("checkout DB unreachable: %v", err)
```

Recommended: **normal INFO**; peak season (11.11 / Black Friday) **WARNING** (10x less log volume); payment flow **ERROR+ to webhook**.

## Documentation

| File | Content |
|---|---|
| [doc.go](doc.go) | API reference (godoc, English) |
| [doc_zh.md](doc_zh.md) | API reference (Chinese) |
| [PERFORMANCE.en.md](PERFORMANCE.en.md) | Performance, architecture, benchmarks |
| [PERFORMANCE.md](PERFORMANCE.md) | Performance (Chinese) |

## Documentation

| File | Content |
|---|---|
| [USAGE.md](USAGE.md) | **Detailed usage guide** (setup, fields, writers, slog, sharding, monitoring, filters) |
| [doc.go](doc.go) | API reference (godoc) |
| [doc_zh.md](doc_zh.md) | API reference (Chinese) |
| [PERFORMANCE.en.md](PERFORMANCE.en.md) | Performance, architecture, benchmarks |
| [PERFORMANCE.md](PERFORMANCE.md) | Performance (Chinese) |

## License

MIT. Origin: github.com/xwi88/log4go (originally GPLv3), re-licensed MIT by the original author.
