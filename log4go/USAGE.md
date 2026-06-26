# log4go Usage Guide

Detailed examples for every feature, organized by use case. Each section is a
copy-paste-ready code block you can drop into your application.

## Table of contents

- [Setup](#setup)
  - [Config file (JSON)](#config-file-json)
  - [Code (full control)](#code-full-control)
  - [Presets](#presets)
- [Structured fields](#structured-fields)
  - [Typed fields (zero-boxing)](#typed-fields-zero-boxing)
  - [Base fields (global static)](#base-fields-global-static)
  - [Context binding (request-scoped)](#context-binding-request-scoped)
  - [OpenTelemetry trace extraction](#opentelemetry-trace-extraction)
- [Formats](#formats)
- [Writers](#writers)
  - [ConsoleWriter](#consolewriter)
  - [FileWriter (async + overflow)](#filewriter-async--overflow)
  - [KafKaWriter (Kafka -> ES)](#kafkawriter-kafka--es)
  - [NetWriter (TCP/UDP)](#netwriter-tcpudp)
  - [WebhookWriter (Lark / DingTalk)](#webhookwriter-lark--dingtalk)
  - [Multi-writer with per-writer level](#multi-writer-with-per-writer-level)
- [slog bridge](#slog-bridge)
- [Sampling](#sampling)
- [Sharding (multi-core)](#sharding-multi-core)
- [Panic / Fatal / Recover](#panic--fatal--recover)
- [Monitoring](#monitoring)
- [Filters](#filters)

---

## Setup

### Config file (JSON)

Create `/etc/app/log.json`:

```json
{
    "level": "INFO",
    "format": "json",
    "console_writer": {
        "enable": false
    },
    "file_writer": {
        "enable": true,
        "level": "INFO",
        "filename": "/var/log/app-%Y%M%D.log",
        "rotate": true,
        "daily": true,
        "max_days": 14,
        "async": true,
        "async_buffer_size": 8192,
        "overflow_policy": "spill",
        "spill_type": "ring",
        "spill_size": 8192
    },
    "kafka_writer": {
        "enable": true,
        "level": "INFO",
        "brokers": ["kafka-1:9092", "kafka-2:9092"],
        "producer_topic": "app-logs",
        "buffer_size": 65536,
        "overflow_policy": "spill",
        "spill_type": "ring",
        "spill_size": 65536
    }
}
```

Load in code:

```go
package main

import "github.com/v8fg/kit4go/log4go"

func main() {
    if err := log4go.SetLogWithConf("/etc/app/log.json"); err != nil {
        panic(err)
    }
    defer log4go.Close()

    log4go.Info("server started")
}
```

You can also pass raw bytes:

```go
_ = log4go.SetLog([]byte(`{"level":"INFO","format":"json"}`))
```

### Code (full control)

```go
lg := log4go.NewLogger()
lg.SetLevel(log4go.INFO)
lg.SetFormat(log4go.FormatJSON)
lg.WithCaller(true)

// File writer: async + spill ring (survives bursts, no OOM)
lg.Register(log4go.NewFileWriterWithOptions(log4go.FileWriterOptions{
    Enable:         true,
    Level:          log4go.LevelFlagInfo,
    Filename:       "/var/log/app-%Y%M%D.log",
    Rotate:         true,
    Daily:          true,
    MaxDays:        14,
    Async:          true,
    AsyncBufferSize: 8192,
    OverflowPolicy:  "spill",
    SpillType:       "ring",
    SpillSize:       8192,
}))

// Kafka writer: async + spill (full volume to ES)
lg.Register(log4go.NewKafKaWriter(log4go.KafKaWriterOptions{
    Enable:         true,
    Level:          log4go.LevelFlagInfo,
    Brokers:        []string{"kafka-1:9092"},
    ProducerTopic:  "app-logs",
    BufferSize:     65536,
    OverflowPolicy: "spill",
    SpillType:      "ring",
    SpillSize:      65536,
}))

defer lg.Close()

lg.Info("server started on :%d", 8080)
```

### Presets

One-liner configurations mirroring `zap.NewProduction()` / `NewDevelopment()`:

```go
// Production: JSON + INFO + sampling (100/100) + caller + console
lg := log4go.NewProduction()
defer lg.Close()

// Development: colored text + DEBUG + caller + funcname + console
lg := log4go.NewDevelopment()
defer lg.Close()
```

Register additional writers on the returned logger as needed.

---

## Structured fields

### Typed fields (zero-boxing)

Typed field constructors avoid `interface{}` boxing on the hot path — the same
technique as `zap.Field` / `slog.Attr`:

```go
// Chain typed constructors:
lg.WithString("trace_id", "abc-123").
    WithInt("status", 200).
    WithInt64("user_id", 9950).
    WithBool("cached", true).
    WithFloat64("score", 0.87).
    WithDuration("elapsed", 3*time.Millisecond).
    WithTime("created_at", time.Now()).
    WithBytes("raw", payload).
    WithError("cause", err).
    Info("request served")

// Batch attach via WithAttrs (one clone):
lg.WithAttrs(
    log4go.String("route", "/api/v1/orders"),
    log4go.Int("items", 3),
    log4go.Duration("db_query", 12*time.Microsecond),
).Info("order placed")
```

The plain `With(key, interface{})` API also works — `fieldOf` infers the scalar
kind internally, so common types (string, int, bool, float, time.Duration,
time.Time, error, []byte) are allocation-free without typed constructors:

```go
lg.With("request_id", reqID).With("latency_ms", 42).Info("ok")
```

### Base fields (global static)

Set once at startup; every subsequent record carries them. Useful for
environment context (hostname, server IP, app name) and routing keys (es_index):

```go
log4go.SetBaseFields(map[string]interface{}{
    "hostname":  os.Getenv("HOSTNAME"),
    "server_ip": localIP,
    "app":       "payment-svc",
    "env":       "prod",
    "version":   buildVersion,
})

// Or individually:
log4go.SetBaseField("es_index", "payment-logs-2026.06")

// Base fields propagate to child loggers (With, WithContext):
log4go.With("trace_id", "t-1").Info("started")
// JSON output:
// {"hostname":"...","server_ip":"10.0.1.5","app":"payment-svc",
//  "env":"prod","es_index":"payment-logs-2026.06","trace_id":"t-1", ...}
```

### Context binding (request-scoped)

Bind a child logger onto the context in middleware; handlers recover it so
every line carries the request ID automatically:

```go
// Middleware:
func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        reqLog := log4go.With("request_id", extractOrGenID(r))
        ctx := reqLog.IntoContext(r.Context())
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// Handler:
func handleOrder(w http.ResponseWriter, r *http.Request) {
    lg := log4go.FromContext(r.Context())
    lg.With("user_id", userID).Info("order processing started")
    lg.Info("order completed")
    // Both lines carry request_id + user_id automatically.
}
```

Built-in middleware (generates a request ID if the header is absent):

```go
handler := log4go.RequestIDMiddleware(mux, log4go.RequestIDMiddlewareOpts{
    Header: "X-Request-Id", // inbound header to read (default X-Request-Id)
})
```

### OpenTelemetry trace extraction

Stack a custom extractor to pull trace_id / span_id from the OTel context. No
hard dependency on `go.opentelemetry.io/otel` — you only pay if you import it:

```go
import "go.opentelemetry.io/otel/trace"

log4go.AddContextExtractor(func(ctx context.Context) map[string]interface{} {
    sc := trace.SpanFromContext(ctx).SpanContext()
    if !sc.IsValid() {
        return nil
    }
    return map[string]interface{}{
        "trace_id": sc.TraceID().String(),
        "span_id":  sc.SpanID().String(),
    }
})

// Now WithContext(ctx) auto-extracts trace_id + span_id:
log4go.FromContext(ctx).Info("span logged")
```

---

## Formats

```go
// JSON — one object per record (default for NewProduction, Filebeat/Fluentd native):
log4go.SetFormat(log4go.FormatJSON)
// {"unix_nano":1782...,"seq":42,"time":"2026-06-27T12:00:00.000000Z",
//  "level":"INFO","msg":"started","fields":{"trace_id":"t-1"}}

// logfmt — key=value (Loki/Promtail/docker native):
log4go.SetFormat(log4go.FormatLogfmt)
// time=2026-06-27T12:00:00.000000Z level=INFO msg=started trace_id=t-1

// Text — human-readable (default):
log4go.SetFormat(log4go.FormatText)
// 2026/06/27 12:00:00 [INFO] <svc.go:42> started {"trace_id":"t-1"}
```

JSON codec selection (only affects `kindAny` values; scalars always use typed
append):

```go
log4go.SetJSONCodec(log4go.JSONCodecGoccy) // default, ~2-3x encoding/json
log4go.SetJSONCodec(log4go.JSONCodecStd)    // standard library
log4go.SetJSONCodec(log4go.JSONCodecSonic)  // bytedance/sonic (fastest on amd64)
```

---

## Writers

### ConsoleWriter

```go
// Plain text (default, production-safe — no ANSI codes):
lg.Register(log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{
    Enable: true,
    Level:  log4go.LevelFlagInfo,
}))

// Colored (development terminals only):
lg.Register(log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{
    Enable:    true,
    Color:     true,       // color the level flag
    FullColor: false,      // set true to color the entire line
    Level:     log4go.LevelFlagDebug,
}))

// Buffered (container stdout collection — reduces syscalls):
lg.Register(log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{
    Enable:    true,
    Buffered:  true,
    BufferSize: 8192,
    Level:     log4go.LevelFlagInfo,
}))
```

### FileWriter (async + overflow)

```go
lg.Register(log4go.NewFileWriterWithOptions(log4go.FileWriterOptions{
    Enable:          true,
    Level:           log4go.LevelFlagInfo,
    Filename:        "/var/log/app-%Y%M%D.log", // date pattern (auto-rename)
    Rotate:          true,
    Daily:           true,
    MaxDays:         14,
    // Async pipeline (recommended for high write rates):
    Async:           true,
    AsyncBufferSize: 8192,        // bounded channel capacity
    OverflowPolicy:  "spill",     // "drop" | "block" | "spill"
    SpillType:       "ring",      // "ring" (memory) | "file" (disk)
    SpillSize:       8192,        // ring capacity (records)
    // For disk spill:
    // SpillDir:       "/var/log/spill",
    // SpillMaxBytes:  1 << 30,   // 1GB cap
}))
```

Overflow policy behavior:

| Policy | Channel full | Use case |
|---|---|---|
| `drop` | Discard (counted in metrics) | Default; lowest latency |
| `block` | Block caller (backpressure) | Never lose data; can stall |
| `spill` | Buffer in ring/file, re-send on recovery | Survive bursts without loss |

### KafKaWriter (Kafka -> ES)

```go
kw := log4go.NewKafKaWriter(log4go.KafKaWriterOptions{
    Enable:         true,
    Level:          log4go.LevelFlagInfo,
    Brokers:        []string{"kafka-1:9092", "kafka-2:9092"},
    ProducerTopic:  "app-logs",
    BufferSize:     65536,           // bounded async send channel
    OverflowPolicy: "spill",
    SpillType:      "ring",
    SpillSize:      65536,
})
kw.SetOnEvent(func(name string, delta int64) {
    // name: "sent" | "error" | "dropped" | "spilled"
    // Forward to Prometheus / statsd
})
lg.Register(kw)
```

Kafka record value (one JSON object per message):

```json
{
    "unix_nano": 1782392990123456789,
    "seq": 42,
    "level": "INFO",
    "message": "bid won",
    "timestamp": "2026-06-27T12:00:00.000000Z",
    "server_ip": "10.0.1.5",
    "es_index": "adx-logs-2026.06",
    "hostname": "prod-01",
    "trace_id": "a1b2c3"
}
```

Filebeat consumer config (routes on `es_index`):

```yaml
kafka:
  topics: [adx-logs]
output.elasticsearch:
  indices:
    - index: "adx-logs-2026.06"
      when.equals: { es_index: "adx-logs-2026.06" }
```

### NetWriter (TCP/UDP)

```go
nw := log4go.NewNetWriter(log4go.NetWriterOptions{
    Network:        "tcp",          // "tcp" or "udp"
    Address:        "collector.internal:514",
    Level:          log4go.LevelFlagWarning, // WARN+ only (RTT-bound, keep low volume)
    BufferSize:     1024,
    OverflowPolicy: "drop",
    Timeout:        3 * time.Second,
})
lg.Register(nw)
```

### WebhookWriter (Lark / DingTalk)

```go
// 1. Create the webhook sink (async, retry, rate-limited):
sink := log4go.NewWebhookAlertSink(
    "https://oapi.example.com/lark/bot-xxx",
    256,                              // async queue size
    log4go.LarkTextFormatter(""),      // or DingtalkTextFormatter / WechatTextFormatter
)
sink.SetRateLimit(10)   // at most 10 posts/sec
sink.SetMaxRetries(2)   // retry failed posts twice

// 2. Wrap as a writer with level + filter + rate gate:
webhook := log4go.NewWebhookWriter(sink, log4go.WebhookWriterOptions{
    Level: log4go.LevelFlagError,     // ERROR+ only
    Filter: log4go.AllOf(
        log4go.MatchField("domain", "payment"), // only payment errors
        log4go.MatchKeyword("fail"),             // containing "fail"
    ),
    Gate:          log4go.NewRateAlerter(time.Minute, 5), // >=5/min fires ~1/min
    RateFormatter: log4go.DefaultRateWebhookFormatter,    // "[5 in window] ..."
})
lg.Register(webhook)
```

### Multi-writer with per-writer level

One logger, multiple sinks at different levels:

```go
lg := log4go.NewLogger()
lg.SetLevel(log4go.DEBUG)

// Console: DEBUG+ (dev, colored)
lg.Register(log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{
    Enable: true, Color: true, Level: log4go.LevelFlagDebug,
}))

// Kafka: INFO+ (full volume -> ES)
lg.Register(log4go.NewKafKaWriter(log4go.KafKaWriterOptions{
    Enable: true, Level: log4go.LevelFlagInfo,
    Brokers: []string{"kafka:9092"}, ProducerTopic: "app",
    BufferSize: 1 << 14,
    OverflowPolicy: "spill", SpillType: "ring", SpillSize: 1 << 16,
}))

// Webhook: ERROR+ (Lark alert, filtered + rate-gated)
sink := log4go.NewWebhookAlertSink("https://oapi.example.com/lark/xxx", 256,
    log4go.LarkTextFormatter(""))
lg.Register(log4go.NewWebhookWriter(sink, log4go.WebhookWriterOptions{
    Level: log4go.LevelFlagError,
}))

defer lg.Close()

// One call, three sinks:
lg.Error("payment failed: %v", err)
// -> Console (red, DEBUG+ pass) + Kafka (INFO+ pass) + Lark (ERROR+ pass)
```

---

## slog bridge

Route the standard library `log/slog` (and any library using it — net/http, gRPC)
through log4go's pipeline:

```go
lg := log4go.NewProduction()
defer lg.Close()

slog.SetDefault(slog.New(log4go.NewSlogHandler(lg)))

// All slog calls now flow into log4go:
slog.Info("server started", "port", 8080)
slog.Error("db error", "err", err, "retry", 3)

// WithAttrs / WithGroup propagate:
slog.With("service", "api").Info("request handled", "method", "GET")
// -> {"service":"api","method":"GET","msg":"request handled",...}
```

---

## Sampling

Protect against log storms — the first `initial` records per level pass, then
one in every `thereafter`:

```go
sampled := log4go.WithSampling(100, 100)
for i := 0; i < 100000; i++ {
    sampled.Info("high frequency event %d", i)
    // First 100 pass, then 1 in 100 -> ~100 + 999 = 1099 logged
}
```

---

## Sharding (multi-core)

ShardLogger fans records across N independent Loggers, each with its own
bootstrap goroutine. Throughput scales with cores when the writer is slow
(disk, Kafka):

```go
// Auto (recommended): max(2, GOMAXPROCS/2), no hard cap
sl := log4go.NewShardLogger(0)

// Or structured options:
sl := log4go.NewShardLoggerWithOptions(log4go.ShardLoggerOptions{
    Shards:      0,                       // 0=auto, >=1=pin
    Level:       log4go.LevelFlagInfo,
    ChannelSize: 8192,                    // per-shard records channel
})

// Each shard MUST get its own writer (use RegisterFunc, not Register):
sl.RegisterFunc(func() log4go.Writer {
    return log4go.NewFileWriterWithOptions(log4go.FileWriterOptions{
        Enable: true, Async: true,
        Filename: "/var/log/shard-%Y%M%D.log",
        OverflowPolicy: "spill", SpillType: "file",
    })
})

defer sl.Close()

// Records distributed round-robin; ordering preserved within a shard.
sl.Info("sharded line %d", i)
```

**When to shard**: multi-core + slow writer (disk/Kafka) + >1M QPS target.
With a fast writer (discard/memory), a single Logger already saturates —
sharding only adds dispatch overhead.

---

## Panic / Fatal / Recover

```go
// Fatal: log CRITICAL -> flush all writers -> os.Exit(1)
log4go.Fatal("cannot bind :%d: %v", port, err)

// Panic: log CRITICAL -> flush -> panic(msg) (can be recovered upstream)
log4go.Panic("invariant violated: %v", val)

// Recover: defer in main / goroutine entry — captures panic into the log
// pipeline (optionally alerts via WebhookWriter), then re-raises:
defer log4go.Recover(func() *log4go.Logger { return lg })
// ... code that might panic ...
```

---

## Monitoring

### Per-level metrics (pull model)

```go
m := log4go.Metrics()
// m.Records[log4go.INFO], m.Records[log4go.ERROR], m.Records[log4go.DEBUG], ...
```

### Writer-specific metrics

```go
kwm := kw.Metrics()
// kwm.Sent      — records delivered to Kafka
// kwm.Errored   — send failures
// kwm.Dropped   — records dropped (overflow drop policy)
// kwm.Spilled   — records buffered to spill store
// kwm.Queued    — current channel depth (alert at 80% of BufferSize)
// kwm.SpillLen  — current spill store depth
```

### Real-time event hook (push model)

```go
kw.SetOnEvent(func(name string, delta int64) {
    switch name {
    case "sent":
        promKafkaSent.Add(float64(delta))
    case "dropped":
        promKafkaDropped.Add(float64(delta))  // alert if > 0
    case "spilled":
        promKafkaSpilled.Add(float64(delta))
    }
})
```

### Runtime stats (scrape cadence only — ReadMemStats has sub-ms STW)

```go
go func() {
    for range time.Tick(15 * time.Second) {
        rs := log4go.RuntimeStats()
        // rs.GOMAXPROCS, rs.NumGoroutine, rs.HeapAlloc, rs.HeapInuse,
        // rs.HeapObjects, rs.NumGC, rs.GCCPUFraction
        promHeapAlloc.Set(float64(rs.HeapAlloc))
        promGoroutines.Set(float64(rs.NumGoroutine))   // constant = healthy
        promGCCPUFraction.Set(rs.GCCPUFraction)         // >0.2 = investigate
    }
}()
```

**Recommended metrics**:

| Metric | Alert |
|---|---|
| `log4go_records_total{level}` | rate spike = traffic anomaly |
| `kafka_dropped_total` | >0 = data loss (scale or fix broker) |
| `kafka_queued` | >80% BufferSize = backpressure |
| `kafka_spill_len` | growing = sustained overload |
| `log4go_goroutines` | rising = goroutine leak |
| `log4go_gc_cpu_fraction` | >0.2 = GC pressure (tune allocs) |

---

## Filters

Built-in filter constructors for `WebhookWriterOptions.Filter`:

```go
// Field equality (exact then string-form tolerant):
log4go.MatchField("domain", "payment")
log4go.MatchFieldIn("domain", "payment", "risk", "auth")

// Keyword (case-insensitive substring on message):
log4go.MatchKeyword("fail")
log4go.MatchKeywordIn("timeout", "refused", "panic")

// Combinators:
log4go.AllOf(f1, f2, f3)  // AND
log4go.AnyOf(f1, f2)      // OR
log4go.NotMatch(f)         // NOT
```

Example — alert only on payment-domain errors containing "fail":

```go
Filter: log4go.AllOf(
    log4go.MatchField("domain", "payment"),
    log4go.MatchKeyword("fail"),
),
```
