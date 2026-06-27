# log4go Distributed Observability

How log4go fits into a distributed (multi-service, multi-language) observability
stack. log4go stays **generic** — it provides two primitives (deterministic
id-based sampling + id-based routing) and leaves tracing, capture decisions,
and cross-service aggregation to OTel / a separate capture system / the backend.

## Responsibility split (don't reinvent)

| Concern | Owner | Mechanism |
|---|---|---|
| Cross-service correlation, method-call trees, timing | **OpenTelemetry** | W3C `traceparent`, spans, OTLP → Tempo/Jaeger |
| Log records (message, variables, context) | **log4go** | structured logs tagged with `trace_id`/`request_id`/`device_id` |
| "Which traces to capture" decisions (error/slow/rule) + download | **separate sampling/capture system** | reads the central sink; full cross-service visibility |
| Full-trace aggregation & export | **backend** (ES/Loki/Tempo) | query by `trace_id` |

log4go does **not** build a tracer, decide capture semantics (error detection,
tail decisions), export/download, or aggregate cross-service. Those are OTel /
a separate capture component / the backend.

## A. Identifier correlation (core, mostly already present)

- Built-in extractor auto-attaches `trace_id`/`span_id`/`request_id`/
  `x-correlation-id`/`device_id`/`did`/`dpid` from `context.Context`
  (`log.go` `defaultContextTraceKeys`). Custom keys via `AddContextExtractor` /
  `SetBaseField`.
- Cross-service "primary key" = W3C `trace_id` (propagated by OTel SDKs in
  Go/Java/Python) → backend `trace_id=xyz` reconstructs the full chain.
- Records carry `seq` + `unix_nano` for exact cross-shard ordering.

## B. Pluggable sampling strategy (decide what actually ships to Kafka)

Writing 100% of logs to Kafka wastes bandwidth/storage/consumer cost (ship all,
keep a fraction). Sampling cuts volume at the source. Rather than bake in one
algorithm, log4go exposes a **pluggable strategy** + common built-ins, and lets
the **business system decide** via a hook:

```go
// SamplingStrategy decides whether a record is actually written (shipped). It is
// called on the delivery path, so implementations must be fast + concurrency-safe.
type SamplingStrategy interface {
    ShouldLog(r *Record) bool
}
log4go.SetSamplingStrategy(strategy)   // switch: install to enable; nil = Full
```

Built-in strategies (industry-aligned with OpenTelemetry):
- `FullSampling` (**default**, backward-compatible; dev/test) — keep everything.
- `TraceIDRatioBased(ratio)` (**preferred**, matches OTel): compare the id
  (trace_id, or request_id if UUID/random) treated as a uint64 against
  `ratio * MaxUint64` — keep if below. No hash needed (the id is already random),
  high precision, deterministic → the whole chain for that id is kept/dropped
  together across all services. OTel SDKs implement this identically in
  Go/Java/Python, so ports agree by construction.
- **Honor W3C `traceparent` sampled flag** (ParentBased semantics): if the
  record's trace context carries a sampled flag, honor it (the entry service's
  head decision propagates) — automatic cross-service consistency without each
  service re-deciding. Ignoring this flag is a known cause of fragmented traces.
- `TailDigitSampling(idKey, modulus, keep)` — coarse, readable (e.g. the classic
  `hash(request_id) % 10 < N` pattern). MUST use a **documented fixed hash**
  (FNV-1a) — language-builtin hashes differ across Go/Java/Python and break
  cross-language consistency. Use only when 10%/1% granularity suffices.
- existing per-level `Sampler` — rate-limit per level (storm protection);
  per-service, not chain-consistent.

Two traps to avoid (why TraceIDRatioBased is preferred over `hash(id)%N`):
language-builtin string hashes are not portable (Go's is randomized; Java/Python
differ) → cross-service inconsistency; and `%N` is coarse. Treating a random id
as a number (OTel style) needs no hash and gives full precision.

Defaults are generic and carry no business logic. A business installs its own
`SamplingStrategy` for custom rules — log4go just calls it and honors the
verdict. Cross-language consistency: built-in deterministic strategies follow
the OTel spec exactly, so Java/Python ports decide identically for the same id.

## Two tracks in practice (high-volume op logs vs request-scoped business record)

Real high-throughput systems (e.g. ad serving at 100K+ log lines/sec) run two
distinct tracks, and log4go serves each differently:

- **System / operational logs**: record **only necessary changes and errors** —
  deliberately minimal (not voluminous INFO/DEBUG), so volume is low and they
  are kept in full; sampling is not needed for this track. The business code
  simply logs at the right level at change/error points.
- **Business / request tracking (1 record per request)**: a rich, structured
  record accumulated across the request's stages, then emitted once, shipped, and
  extracted downstream by a dedicated consumer to follow "which step did this
  request reach, did it finish." Volume scales with QPS (100k QPS ⇒ ~100k small
  records/sec) — not low in absolute terms, but each record is small and Kafka
  handles it. Kept **full by design** (its purpose is to track every request);
  sampling it would lose requests and defeats the purpose, so it is only sampled
  (`TraceIDRatioBased`) as an explicit cost trade-off. Expressed with existing
  log4go primitives — a request-scoped child logger that accumulates `With`
  fields and emits one record at the end:

  ```go
  rs := log4go.WithContext(ctx).WithString("request_id", rid).WithString("device_id", did)
  rs = rs.WithString("step", "bid_received").WithBool("matched", true).WithInt("price", p)
  // ... each stage adds fields ...
  rs.Info("request done")   // one JSON record carrying the whole trail
  ```

  That single record IS the "big struct"; log4go does not need a typed
  accumulator primitive (that would push business-specific structure into the
  library = over-engineering). Both tracks carry `request_id`/`trace_id`/
  `device_id`, so the backend correlates them by id.

  The record terminates with a business **error code** (the outcome / which step
  it ended at). Error-code sets are environment-specific, and panics map to a
  code+description too — so a request always produces a complete record. The
  error-code system is **business-domain** (code sets, panic→code mapping,
  semantics); log4go only (a) carries it as a normal field (`WithString(
  "error_code", …)`) and (b) ensures a panic still emits the record via
  `log4go.Recover` (log+stack at CRITICAL in the deferred handler, then the
  business sets the code and the record ships). log4go does not define codes.

  **Business records must not be lost — they land on disk.** So the business
  track uses a **disk-backed writer with the spill overflow policy** (log4go
  `OverflowSpill` → `FileSpiller`; full → spill to disk, never drop), not the
  drop policy. Flow: full business records → disk / data lake (durable, no loss)
  → big-data analysis (Spark/Flink/Hive); a **sampled copy** (`TraceIDRatioBased`
  by request_id) → ES for interactive analysis. Sampling here never drops
  business data — it only takes a subset copy for ES; the full set is always on
  disk. (Contrast: the operational track's non-error logs may use the drop
  policy — those are loss-tolerant.)

## Operations (dev vs prod, and the enabler)

- **Dev/test**: `FullSampling` (default) — see everything, no surprises.
- **Prod**: install a deterministic strategy (`ProbabilisticSampling` /
  `TailDigitSampling`) to cut Kafka/ingest cost. Because the decision is a pure
  function of the id, a sampled id keeps its **whole chain** (no loss for what
  you keep) while the rest is never shipped (cost win).
- **The enabler** — the entry point (e.g. the ad bid request handler, which
  already has `request_id`/`device_id`) must **capture the id and propagate it**
  on the request context (and across services via W3C `traceparent` / a header).
  Every downstream log then carries it, so deterministic-by-id sampling is
  lossless for sampled ids. Without propagation, sampling can fragment a chain —
  so propagation is the prerequisite, not a log4go feature.

## C. id-based routing (capture mode — generic filter, no business logic)

When you want a specific id's full chain captured, log4go **routes** — it does
not decide:

- Configure a capture rule (an id set / a predicate, e.g. `device_id=…`,
  `request_id=…`), and a **central sink** (a Kafka topic / collector endpoint).
- Every service's log4go, on a record whose id matches, **also writes it to that
  central sink** (in addition to normal output). This is a generic
  match-and-route filter.
- A **separate sampling/capture system** reads that sink, applies the real
  capture policy (keep error traces, slow traces, N per device, …) with full
  cross-service visibility, and downloads/exports. That system owns the business
  rules — log4go does not.

This keeps log4go free of business detail: the "which ids" is config, the
"what to keep" is the capture system.

## D. Shipping (reuse existing writers, zero new sinks)

Routed/normal logs go to the same async, bounded, overflow-safe writers already
in the box — no cloud SDK in the library: `KafKaWriter` (JSON/Kafka, the common
cross-language path), `NetWriter` (TCP/syslog), `WebhookWriter` (HTTP JSON),
`FileWriter` (→ Loki/Promtail/Fluentd), `IOWriter` (any `io.Writer`). Backend
switching = change topic/endpoint/exporter; log4go unchanged.

## E. Cross-language consistency (identical algorithm + logic)

Regardless of language (Go/Java/Python), the **sampling algorithm and routing
logic must be identical** so every component treats the same id the same way:
- Same deterministic hash + ratio formula (documented; or honor W3C flag).
- Same id-match → route-to-central-sink behavior.
- Standard formats only (W3C `traceparent`, structured JSON) — no private format.

## What log4go deliberately does NOT do (non-over-engineering)

- No distributed tracer / span tree (→ OTel).
- No error detection / tail decisions / capture semantics (→ separate capture
  system with full visibility).
- No local `ExportTrace`/download API (the capture system reads the sink).
- No OTLP exporter adapter (Collector ingests JSON via Kafka/HTTP).
- No disk spill / per-request capture buffer (memory-bounded writers suffice;
  capture storage is the capture system's job).
- No business rules (which device, which error policy) — those are config /
  the capture system.

## Phasing

- **P0** — this design doc (done).
- **P1** — `device_id`/`did`/`dpid` in default keys; **deterministic id sampler**
  (honor W3C sampled flag + documented hash/ratio fallback); multi-instance
  consistency test (same id → same decision). Generic, no business logic.
- **P2** — **id-match router** (capture mode): match id → extra central sink;
  bounded, non-blocking, default-off. The capture/download is a separate system.

