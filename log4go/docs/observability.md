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

## B. Deterministic id-based sampling (DEFAULT policy — whole-chain consistent)

A request's logs must be kept-or-dropped **together across all services**, or the
chain fragments. The decision is therefore a **pure function of the id** — every
service/language seeing the same id reaches the same verdict:

- **Honor the W3C `traceparent` sampled flag** (preferred, OTel-native): the
  entry service's head decision propagates in the flag; log4go keeps a record
  whose trace is flagged sampled. Automatic cross-language consistency — the flag
  is the single source of truth.
- **Deterministic hash fallback** (no OTel): `hash(trace_id) % 10000 < ratio*10000`
  with a **documented, fixed hash** (e.g. FNV-1a of the id string). Ports in
  Java/Python MUST use the identical algorithm so the same id decides the same
  everywhere — no coordination, no propagation needed.
- No id present (rare) → fall back to the existing per-record rate-limit sampler
  (storm protection).

This is a generic algorithm, no business logic. It replaces per-service random
sampling (which fragments chains) for id-bearing records.

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

