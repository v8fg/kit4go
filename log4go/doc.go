// Package log4go is a structured logging library with console, file and kafka writers.
//
// 中文文档 (Chinese docs): doc_zh.md. Performance: PERFORMANCE.md (中) /
// PERFORMANCE.en.md (en). Distributed observability (trace correlation, source-side
// tail sampling, cross-language interop): docs/observability.md.
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
// reuse are all now solid and safe.
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
//     record in deliverRecordToWriter and cached on r.formattedBytes, so every
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
// # Typed fields, slog, logfmt, presets, Panic/Fatal (Round A)
//
// Layered on the structured-fields core, with the same "easy by default, typed
// when you need the speed" split as zap:
//
//   - Typed fields (zero-boxing): Logger.With(key, val) maps common Go scalars
//     to an unboxed internal kind automatically (string / bool / all int widths /
//     all uint widths / uintptr / float / []byte / complex / time.Duration /
//     time.Time / error) — so the everyday API is unchanged but allocation-free
//     for scalars. When you want compile-time-typed fields, use the typed
//     constructors Logger.WithString/WithInt/WithInt64/WithUint64/WithBool/
//     WithFloat64/WithDuration/WithTime/WithBytes/WithError, or build Fields with
//     log4go.String/Int/.../Bytes/Complex128 and attach them via WithAttrs.
//
//   - Robust by construction: ANY field value degrades safely instead of
//     crashing the pipeline. A panicking json.MarshalJSON, a typed-nil error, an
//     unmarshallable chan/func, or NaN/±Inf all render as null (JSON) / "-"
//     (logfmt) — output is always valid. []byte is base64; complex is "a+bi".
//
//   - logfmt: SetFormat(FormatLogfmt) emits "time=... level=... msg=\"...\" k=v"
//     (Loki/Promtail/docker native). FormatText/FormatJSON/FormatLogfmt all
//     pre-serialize once into r.formattedBytes, shared by every writer.
//
//   - slog.Handler: NewSlogHandler(logger) adapts log4go to the standard
//     log/slog.Handler interface — slog.SetDefault(slog.New(log4go.NewSlogHandler(lg)))
//     routes net/http and any slog-using library through the log4go pipeline
//     (its writers, overflow protection, alerting, metrics).
//
//   - Presets: NewProduction() (JSON + INFO + sampling + caller) and
//     NewDevelopment() (colored text + DEBUG + funcname), mirroring zap.
//
//   - Panic/Fatal/Recover: Panic/Fatal log at CRITICAL, Sync() (drain + flush),
//     then panic/exit so no record is lost. Recover captures a panic into the
//     pipeline (optionally a WebhookWriter for sentry-style alerting) and
//     re-raises it.
//
// # Writers at a glance (see PERFORMANCE.md §12 for full measured numbers)
//
// All writers share one bootstrap goroutine that calls each registered writer
// serially per record, so end-to-end throughput ≈ 1 / Σ(writer Write time).
// Pick the minimum set of writers for your pipeline — extra slow writers drag
// every record. Measured single-core QPS (Apple M5, Go 1.26.0, sink isolated
// from I/O noise: pipe/buffer/mock/loopback):
//
//   - FileWriter (sync bufio):     ~127 ns/op, ~7.9M QPS — fastest disk path.
//   - FileWriter (async + drop):   ~201 ns/op, ~5.0M QPS — daemon isolates disk I/O.
//   - FileWriter (async + spill):  ~206 ns/op, ~4.9M QPS — ring fallback, no hot-data loss.
//   - ConsoleWriter (buffered):    ~118 ns/op, ~8.5M QPS — container stdout collection.
//   - ConsoleWriter (unbuffered):  ~1620 ns/op, ~617K QPS — dev/debug only (per-record syscall).
//   - ConsoleWriter (color):       ~2961 ns/op, ~338K QPS — dev/debug + ANSI color.
//   - KafKaWriter (mock producer): ~578 ns/op, ~1.7M QPS — typed buildPayload (was 2582ns pre-typed).
//   - NetWriter (TCP loopback):    ~112 ns/op, ~8.9M QPS caller-side; real network is RTT-bound
//     (~50K-200K same-DC, ~5K-50K cross-DC).
//   - IOWriter (io.Discard):       ~204 ns/op,  ~4.9M QPS — thinnest adapter, test/custom.
//   - Record.JSON (goccy, 3 flds): ~494 ns/op, ~2.0M QPS — typed append (was 5801ns / 6 allocs pre-typed).
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
//     emit the same pre-serialized r.formattedBytes (no re-marshaling).
//   - Multi-core scaling: NewShardLogger(GOMAXPROCS/2) + RegisterFunc per-shard FileWriter
//     (~1.6M+ QPS on 10 cores / 4 shards — the only way past single-core ~1M).
//   - Test / custom sink: IOWriter(bytes.Buffer | io.Discard) — thinnest adapter.
//   - Max throughput:     WithCaller(false) + FileWriter(async) — drops file:line, 16 B/op 1 alloc.
//
// Constraints: never register ConsoleWriter(unbuffered) in production (blocks
// bootstrap); never share one FileWriter across ShardLogger shards (use
// RegisterFunc); never rely on NetWriter for high volume (RTT-bound).
//
// # Performance tiers
//
// The default configuration is already fast (typed fields, caller caching, no
// reflection). The knobs below exist for measured bottlenecks only. Numbers are
// from an M5 / Go 1.26 dev machine; allocations are stable across runs while
// nanoseconds drift with thermal state, so treat allocations as the signal.
//
//	default          NewProduction() + Info/With           1-3 allocs/op
//	typed fields     WithString/WithInt/WithAttrs          avoids scalar boxing
//	max single-core  WithCaller(false) + no-arg + async    1 alloc/op, ~923K/s
//	multi-core       NewShardLogger(0) + RegisterFunc      scales with cores
//
// Default — start here:
//
//	lg := log4go.NewProduction()
//	defer lg.Close()
//	lg.Info("server started")                        // no args: 1 alloc
//	lg.With("trace_id", id).Info("served %s", route) // with args: 3 allocs
//
// With(key, interface{}) infers the scalar kind internally (fieldOf), so it
// avoids boxing for common types. A no-arg Info("msg") is 2 allocs cheaper than
// Info(fmt, args): no fmt.Sprintf and no variadic boxing.
//
// Typed fields (avoid boxing, mirrors zap.Field):
//
//	lg.WithString("trace_id", id).WithInt("status", 200).WithDuration("elapsed", dt).Info("served")
//	lg.WithAttrs(log4go.String("k","v"), log4go.Int("n",1)).Info("x")
//
// JSON uses typed append (Record.JSON is 1 alloc for 3 fields). Scalars bypass
// the codec, so SetJSONCodec has no effect on scalar-only records.
//
// Max single-core throughput (drop caller, no args):
//
//	lg := log4go.NewLogger()
//	lg.WithCaller(false)            // drops file:line, saves 2 allocs
//	lg.SetFormat(log4go.FormatJSON)
//	lg.Register(/* async FileWriter with spill */)
//	lg.Info("x")                    // 1 alloc
//
// Multi-core (sharding helps when the writer is slow — disk, kafka):
//
//	sl := log4go.NewShardLogger(0)  // 0 = auto (max(2, GOMAXPROCS/2))
//	// or structured (config-file friendly):
//	//   sl := log4go.NewShardLoggerWithOptions(log4go.ShardLoggerOptions{
//	//       Shards: 0, Level: log4go.LevelFlagInfo, ChannelSize: 8192,
//	//   })
//	sl.RegisterFunc(func() log4go.Writer { return /* per-shard FileWriter */ })
//	sl.WithCaller(false).Info("imp")
//
// With a slow (~1us) writer, 4 shards reach ~3x one shard. With an instant
// (discard/memory) writer, sharding only adds dispatch overhead — a single
// bootstrap already keeps up. AutoShardCount reads GOMAXPROCS, which Go 1.25+
// derives from the cgroup quota; on older runtimes import
// _ "github.com/v8fg/kit4go/maxprocs".
//
// Knobs, roughly by effect:
//
//	WithCaller(false)            -2 allocs (loses file:line)
//	no-arg Info("msg")           -2 allocs vs Info(fmt, args)
//	WithString/WithInt           zero scalar boxing
//	FileWriter{Async:true}       isolates disk I/O from bootstrap
//	OverflowPolicy:"spill"       ring->file->drop, no OOM, no hot-data loss
//	ShardLogger(0)               linear with cores for slow writers
//	ConsoleWriter{Buffered:true} fewer syscalls on stdout
//
// Pitfalls:
//
//	unbuffered ConsoleWriter in production (blocks bootstrap)
//	sharing one *FileWriter across ShardLogger shards (use RegisterFunc)
//	high-rate With("count", i) (boxes i; use WithInt)
//	NetWriter for high volume (RTT-bound; use File + Kafka)
//
// SPDX-License-Identifier: MIT
package log4go

// verified
