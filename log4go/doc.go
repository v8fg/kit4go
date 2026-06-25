// Package log4go is a structured logging library with console, file and kafka writers.
//
// Origin: github.com/xwi88/log4go (originally licensed under GPLv3).
// Integrated into kit4go by the original author (xwi88), who re-licenses this
// copy under the MIT License to match kit4go's license. See the repository
// root LICENSE for the full MIT text.
//
// # Async-writer lifecycle (formerly "sharp-edges", now hardened)
//
// The async FileWriter and KafKaWriter spawn a daemon goroutine in Init, which
// Logger.Register calls. Three failure modes that used to bite high-QPS
// configurations have been fixed; the notes below describe the fix for each so
// callers know what is now safe.
//
//  1. ShardLogger + async FileWriter: ShardLogger.Register(*FileWriter) is now
//     REJECTED at n>1 (it panics with a clear message) — registering one shared
//     async FileWriter across shards spawned N daemons racing the same
//     bufio/*os.File, corrupting output under load. To fan disk writes across
//     cores use ShardLogger.RegisterFunc(func() Writer { ... }) which builds an
//     INDEPENDENT FileWriter (own daemon + bufio + file) per shard. n==1 with a
//     single Register(fw) remains supported and safe (one shard owns it).
//
//  2. spill-policy async FileWriter shutdown is now race-free. Stop sets a
//     closing flag, closes a stop signal the daemon selects on, and waits for
//     the daemon to drain all queued records + the entire spill store, flush,
//     and exit. Stop never closes the messages channel, so there is no
//     close-vs-send race and no send-on-closed panic; the daemon's drainSpill
//     short-circuits while closing so it never re-injects during shutdown.
//     drop/block/spill policies are all shutdown-safe.
//
//  3. The package singleton is now reusable across Close cycles. Close swaps
//     the singleton to nil; the next package-level call (Register / SetupLog /
//     Debug / ...) rebuilds a fresh Logger with a live bootstrap goroutine and
//     open records channel via an atomic compare-and-swap. The earlier
//     one-shot behavior (Close orphaned the singleton, leaving writer daemons
//     on a dead bootstrap) is gone. Concurrent access is safe (atomic.Pointer).
//
// The simplest correct configuration remains ONE Logger with each async writer
// registered exactly once; but multi-shard fan-out, spill recovery, and Close
// reuse are all now first-class and safe.
//
// # Structured logging (vs zap/zerolog parity)
//
// log4go now covers the structured-logging capabilities expected of a modern
// logger, layered on a single clone() helper so child Loggers share the root's
// records channel and metrics counter:
//
//   - Structured fields: Logger.With(key, val) / WithField / WithFields(map)
//     return a child Logger carrying key/value pairs. Fields render in
//     Record.String (trailing JSON object) and are hoisted to top-level JSON
//     keys by KafKaWriter. No-With hot path is zero-cost.
//
//   - JSON format: SetFormat(FormatJSON) emits one JSON object per record
//     ({"time","level","msg","file","fields"}). The format is decided once per
//     record in deliverRecordToWriter and cached on r.jsonBytes, so every
//     registered writer emits the same bytes without re-marshaling. Default
//     encoder is goccy/go-json (~2-3x encoding/json); switch via
//     SetJSONCodec(JSONCodecGoccy|JSONCodecStd|JSONCodecSonic).
//
//   - Sampling: Logger.WithSampling(initial, thereafter) drops high-frequency
//     records per level (first initial pass, then one every thereafter).
//     Sampled-out records are dropped BEFORE Metrics increment.
//
//   - context.Context: Logger.WithContext(ctx) attaches fields extracted from
//     ctx (default probes trace/request/user/tenant keys; AddContextExtractor
//     stacks custom extractors incl. OpenTelemetry trace/baggage with no hard
//     otel dependency). zerolog-style binding via Logger.IntoContext /
//     FromContext, and RequestIDMiddleware for HTTP request correlation.
//
//   - NetWriter (TCP/UDP) + IOWriter: additional sinks. NetWriter is async +
//     bounded + overflow-safe (never blocks the caller on network I/O); IOWriter
//     is a thin sync adapter for any io.Writer.
//
// # Writers at a glance (see PERFORMANCE.md §12 for full measured numbers)
//
// All writers share one bootstrap goroutine that calls each registered writer
// serially per record, so end-to-end throughput ≈ 1 / Σ(writer Write time).
// Pick the minimum set of writers for your pipeline — extra slow writers drag
// every record. Measured single-core QPS (Apple M5, Go 1.26.0, sink isolated
// from I/O noise: pipe/buffer/mock/loopback):
//
//   - FileWriter (sync bufio):     ~140 ns/op,  ~7.1M QPS — fastest disk path.
//   - FileWriter (async + drop):   ~219 ns/op,  ~4.6M QPS — daemon isolates disk I/O.
//   - FileWriter (async + spill):  ~217 ns/op,  ~4.6M QPS — ring fallback, no hot-data loss.
//   - ConsoleWriter (buffered):    ~294 ns/op,  ~3.4M QPS — container stdout collection.
//   - ConsoleWriter (unbuffered):  ~1620 ns/op, ~617K QPS — dev/debug only (per-record syscall).
//   - ConsoleWriter (color):       ~2961 ns/op, ~338K QPS — dev/debug + ANSI color.
//   - KafKaWriter (mock producer): ~2582 ns/op, ~387K QPS — JSON buildPayload dominates.
//   - NetWriter (TCP loopback):    ~137 ns/op,  ~7.3M QPS caller-side; real network is RTT-bound
//     (~50K-200K same-DC, ~5K-50K cross-DC).
//   - IOWriter (io.Discard):       ~204 ns/op,  ~4.9M QPS — thinnest adapter, test/custom.
//   - Record.JSON (goccy, 3 flds): ~1021 ns/op, ~980K QPS — structured JSON, paid once per record.
//
// Memory: every writer holds <0.05 MB HeapAlloc over 100K records (pool reuse +
// bounded channels + bounded spiller) with a constant 4 goroutines — no OOM
// growth under high QPS. See PERFORMANCE.md §13.
//
// # Scenario recommendations (see PERFORMANCE.md §14 for the decision tree)
//
//   - Local dev:          ConsoleWriter (unbuffered), default config.
//   - Production disk:    FileWriter{Async:true, OverflowPolicy:"drop"} (~4.6M QPS);
//     add SpillType:"ring" to survive bursts without hot-data loss.
//   - Production Kafka:   KafKaWriter{BufferSize, OverflowPolicy:"spill", SpillType:"ring"}.
//   - Container stdout:   ConsoleWriter{Buffered:true} (bufio cuts syscalls).
//   - Remote collection:  NetWriter{Network:"tcp", OverflowPolicy:"drop"} — LOW VOLUME only;
//     high-throughput shipping must use FileWriter + Kafka.
//   - Structured JSON:    SetFormat(FormatJSON) + SetJSONCodec(JSONCodecGoccy) — all writers
//     emit the same pre-serialized r.jsonBytes (no re-marshaling).
//   - Multi-core scaling: NewShardLogger(GOMAXPROCS/2) + RegisterFunc per-shard FileWriter
//     (~1.6M+ QPS on 10 cores / 4 shards — the only way past single-core ~1M).
//   - Test / custom sink: IOWriter(bytes.Buffer | io.Discard) — thinnest adapter.
//   - Max throughput:     WithCaller(false) + FileWriter(async) — drops file:line, 16 B/op 1 alloc.
//
// Constraints: never register ConsoleWriter(unbuffered) in production (blocks
// bootstrap); never share one FileWriter across ShardLogger shards (use
// RegisterFunc); never rely on NetWriter for high volume (RTT-bound).
//
// SPDX-License-Identifier: MIT
package log4go
