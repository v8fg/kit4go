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
// # Net/HTTP writer performance caveat
//
// NetWriter (and any remote sink) is bounded by network RTT and the remote —
// throughput is far below File/Console (~50K-200K QPS for TCP vs ~3M for async
// File). NetWriter is async with a bounded channel + OverflowPolicy so a
// network stall cannot back-pressure the application, but it is intended for
// LOW-VOLUME log collection (ship to a sidecar collector). For high-throughput
// log shipping use FileWriter + Kafka instead. See PERFORMANCE.md §13.
//
// SPDX-License-Identifier: MIT
package log4go
