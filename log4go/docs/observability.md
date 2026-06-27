# log4go Distributed Observability

How log4go fits into a distributed (multi-service, multi-language) observability
stack: **who does what**, and the one feature log4go adds — **source-side
tail sampling** — that cuts Kafka/ingest cost without losing error logs.

## Responsibility split (don't reinvent)

| Concern | Owner | Mechanism |
|---|---|---|
| Cross-service correlation, method-call trees, timing structure | **OpenTelemetry** | W3C `traceparent` propagation, spans, OTLP → Tempo/Jaeger |
| Log records (message, variables, context detail) | **log4go** | structured logs tagged with `trace_id`/`span_id`/`request_id`/`device_id` |
| Full-trace aggregation & export across services | **backend** (ES/Loki/Tempo/collector) | query by `trace_id` |

log4go does **not** build a tracer, a local `ExportTrace` API, a disk spill
store, or an OTLP exporter. Those are OTel / the backend's job. log4go tags
logs reliably, decides **what to ship**, and ships via standard protocols.

## A. Identifier correlation (core, mostly already present)

Every record carries stable correlation IDs so the backend can group a request's
logs across services:

- The built-in context extractor auto-attaches `trace_id`/`span_id`/
  `request_id`/`x-correlation-id`/`device_id`/`did`/`dpid` from `context.Context`
  (`log.go` `defaultContextTraceKeys`). Add a custom key via `AddContextExtractor`
  or `SetBaseField`.
- The cross-service "primary key" is the W3C `trace_id` (propagated by OTel SDKs
  in Go/Java/Python) — every service sees the same id → the backend's
  `trace_id=xyz` query reconstructs the full chain.
- Records already carry `seq` + `unix_nano` for exact cross-shard ordering.

## B. Source-side tail sampling (the one feature log4go adds)

**The problem it solves.** Two common strategies, both painful at scale:

- *Ship 100% → sample at the consumer*: never loses errors, but **wastes Kafka
  bandwidth** (ship everything, keep 10%).
- *Head-sample at the source* (decide per request at start): saves bandwidth, but
  a request not sampled at start **loses its error logs** when it errors later
  (the error wasn't known at decision time).

**Tail sampling** gets both: buffer the request's logs in-process, decide **at
request end** (error? sampled?), and only then ship-or-drop.

```
request in flight:  each log → in-process per-request ring buffer
                                    (bounded, non-blocking, ~ns append)
                              (NOT sent to Kafka yet — this is the bandwidth saving)
request end:        FlushTrace(ctx, errored)  |  or auto-expire (timeout)
                    ├─ errored OR sampled  → flush the whole trail to Kafka
                    └─ otherwise           → discard (Kafka never sees it)
```

- **Saves Kafka/ingest**: the ~90% of boring requests are dropped in-process —
  Kafka, the consumer, and ES only see interesting (error / sampled) trails.
- **Catches errors**: the full trail was buffered, so an error at the end flushes
  the complete in-process trail — no error logs lost.
- **Bounded & non-blocking**: per-request ring (record cap) + global memory-byte
  cap (drop oldest / overflow-count on pressure). Append is non-blocking; flush
  is async. Default-off → zero hot-path cost when disabled.
- **Net performance win**: the tiny ring-append cost is far smaller than the Kafka
  sends it eliminates for non-shipped requests.

**Cross-service scope (honest limit).** This is per-service tail: if *this*
service errors, it ships its whole in-process trail; a sibling service that
already finished (no local error) won't retroactively ship. The full cross-
service trace is still assembled by the **backend** querying `trace_id` across
all services' shipped logs (the existing consumer architecture). For full
cross-service tail decisions, run an OTel Collector / a trace-aware consumer —
not log4go.

## C. Shipping (reuse existing writers, zero new sinks)

Captured/error trails flush to the same async, bounded, overflow-safe writers
already in the box — no cloud SDK is wired into the library:

- `KafKaWriter` — JSON over Kafka (the common cross-language path).
- `NetWriter` — TCP / syslog.
- `WebhookWriter` — HTTP POST JSON.
- `FileWriter` — file → Loki / Promtail / Fluentd tail.
- `IOWriter` — any `io.Writer` (e.g. an S3 SDK writer the caller owns).

Backend switching = change the Kafka topic / HTTP endpoint / collector exporter;
log4go code does not change.

## D. Cross-language interop (use standards, not private formats)

- **W3C TraceContext** (`traceparent`): the `trace_id`/`span_id` primary key,
  understood by OTel SDKs in Go/Java/Python — so a Java/Python service consuming
  or producing the same trace correlates with log4go's logs natively.
- **Structured JSON** over Kafka/HTTP: language-agnostic; any consumer
  `json.Unmarshal`s.
- Backend query example (ES/Loki): `trace_id:"4a3f..."` returns the full chain
  across all services.

## What log4go deliberately does NOT do (non-over-engineering)

- No in-library distributed tracer / span tree (→ OTel).
- No local `ExportTrace(id)` API (unrealistic across 1–10 services; the backend
  correlates by id).
- No OTLP exporter adapter (the OTel Collector ingests JSON via Kafka/HTTP).
- No disk spill store for capture (memory-bounded only; disk is the backend's job).
- No cross-service full-trace tail decision (per-service tail here; cross-service
  tail at the collector/consumer).

## API sketch

```go
// tag identifiers (auto via extractor); add business ids:
log4go.SetBaseField("device_id", did)

// source-side tail sampling: buffer per request, flush only error/sampled
log4go.StartTailSampling(log4go.TailOpts{
    SampleRatio: 0.1,        // 10% of requests kept by trace_id hash
    KeepErrors:  true,       // flush any request that logged >= ERROR
    MaxPerRequest: 4096,     // records buffered per request (ring)
    MaxMemBytes:   64 << 20, // global memory cap
})
// at request end:
log4go.FlushTrace(ctx)       // auto-detects error level in the buffer
```

## Phasing

- **P0** — this design doc (+ interop/correlation notes).
- **P1** — `device_id`/`did`/`dpid` in default keys; **tail sampler** (per-request
  ring + memory cap + flush-on-error/sampled + `FlushTrace`); consistent-by-
  construction (a sampled/error trace ships its whole in-process trail). Hot-path
  concurrency + no-regression benchmark.
