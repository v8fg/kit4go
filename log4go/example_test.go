package log4go_test

// This file holds runnable Example* functions (rendered in godoc) that show
// idiomatic use of the log4go package API. Each example is self-contained and
// uses the package-level singleton, matching how most applications configure
// logging once at startup.

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/v8fg/kit4go/log4go"
)

// Example_basic shows the simplest setup: console output via the package
// singleton, then the level helpers (Debug/Info/Warn/Error).
func Example_basic() {
	// Configure once: a console writer at INFO level.
	_ = log4go.SetupLog(log4go.LogConfig{
		Level: log4go.LevelFlagInfo,
		ConsoleWriter: log4go.ConsoleWriterOptions{
			Enable: true,
			Color:  true,
			Level:  log4go.LevelFlagInfo,
		},
	})
	defer log4go.Close()

	log4go.Info("server started on :%d", 8080)
	log4go.Warn("cache miss rate high: %.2f", 0.42)
	log4go.Error("failed to connect to %s", "db")
}

// Example_fileWriter shows a daily-rotating file writer. The filename uses the
// %Y%M%D pattern so log4go opens the concrete dated file on first write.
func Example_fileWriter() {
	dir := filepath.Join(os.TempDir(), "log4go-example-file")
	_ = log4go.SetupLog(log4go.LogConfig{
		Level: log4go.LevelFlagInfo,
		FileWriter: log4go.FileWriterOptions{
			Enable:   true,
			Level:    log4go.LevelFlagInfo,
			Filename: filepath.Join(dir, "app-%Y%M%D.log"),
			Rotate:   true,
			Daily:    true,
			MaxDays:  7,
			// Async: true enables the bounded, overflow-safe daemon pipeline
			// (recommended for high write rates).
			Async:           true,
			AsyncBufferSize: 4096,
			OverflowPolicy:  "drop",
		},
	})
	defer log4go.Close()

	for i := 0; i < 10; i++ {
		log4go.Info("request %d handled", i)
	}
}

// Example_shardLogger shows a ShardLogger that fans records across multiple
// independent loggers to scale throughput with cores. Each shard has its own
// records channel + bootstrap goroutine; records are distributed round-robin
// and ordering is preserved within a shard.
func Example_shardLogger() {
	sl := log4go.NewShardLogger(4) // 4 shards
	defer sl.Close()
	sl.SetLevel(log4go.INFO)
	sl.Register(log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{
		Enable: true, Color: true, Level: log4go.LevelFlagInfo,
	}))

	for i := 0; i < 100; i++ {
		sl.Info("shard line %d", i)
	}
	time.Sleep(100 * time.Millisecond) // let bootstrap goroutines drain
}

// Example_noCaller shows the WithCaller(false) fast path: skipping
// runtime.Caller capture on every record maximizes throughput when the
// file:line field is not needed.
func Example_noCaller() {
	_ = log4go.SetupLog(log4go.LogConfig{
		Level:         log4go.LevelFlagInfo,
		ConsoleWriter: log4go.ConsoleWriterOptions{Enable: true, Color: true, Level: log4go.LevelFlagInfo},
	})
	defer log4go.Close()

	log4go.NewLogger().WithCaller(false) // max-throughput path
	log4go.Info("high-volume line without caller capture")
}

// Example_metrics shows reading the package-level Metrics snapshot for
// monitoring (per-level record counts).
func Example_metrics() {
	_ = log4go.SetupLog(log4go.LogConfig{
		Level:         log4go.LevelFlagInfo,
		ConsoleWriter: log4go.ConsoleWriterOptions{Enable: false},
	})
	defer log4go.Close()

	log4go.Info("counted")
	log4go.Warn("counted")
	time.Sleep(100 * time.Millisecond)

	m := log4go.Metrics()
	_ = m.Records[log4go.INFO]
	_ = m.Records[log4go.WARNING]
}

// Example_structuredFields shows With/WithField/WithFields building a
// request-scoped child logger. Fields render as a trailing JSON object on the
// text format (and as top-level keys in FormatJSON / KafkaWriter).
func Example_structuredFields() {
	_ = log4go.SetupLog(log4go.LogConfig{
		Level:         log4go.LevelFlagInfo,
		ConsoleWriter: log4go.ConsoleWriterOptions{Enable: true, Level: log4go.LevelFlagInfo},
	})
	defer log4go.Close()

	// chainable; each With returns a new child, parent is unaffected
	reqLog := log4go.With("trace_id", "t-123").WithField("user_id", 42)
	reqLog.Info("request handled")
	reqLog.With("route", "/api/v1").Info("routed") // adds route on top

	// WithFields attaches a whole map in one clone
	log4go.WithFields(map[string]any{"k1": "v1", "k2": 2}).Info("batch fields")
}

// Example_jsonFormat shows structured JSON output (one JSON object per record),
// the convention Fluentd/Filebeat expect. Time uses the ISO layout; fields are
// hoisted into a \"fields\" object (omitted when empty).
func Example_jsonFormat() {
	_ = log4go.SetupLog(log4go.LogConfig{
		Level:         log4go.LevelFlagInfo,
		Format:        "json", // FormatJSON
		ConsoleWriter: log4go.ConsoleWriterOptions{Enable: true, Level: log4go.LevelFlagInfo},
	})
	defer log4go.Close()

	log4go.With("trace_id", "t-9").Info("json line")
	// {"time":"2026-06-25T15:04:05.000+0800","level":"INFO","msg":"json line",
	//  "file":"example_test.go:NN","fields":{"trace_id":"t-9"}}
}

// Example_sampling shows WithSampling protecting against a log storm: the first
// 10 records pass, then one every 100 thereafter. Sampled-out records are
// dropped before Metrics increment.
func Example_sampling() {
	_ = log4go.SetupLog(log4go.LogConfig{
		Level:         log4go.LevelFlagInfo,
		ConsoleWriter: log4go.ConsoleWriterOptions{Enable: false},
	})
	defer log4go.Close()

	sampled := log4go.WithSampling(10, 100)
	for i := 0; i < 1000; i++ {
		sampled.Info("high-frequency event %d", i)
	}
}

// Example_contextBinding shows the zerolog-style pattern: a middleware binds a
// request-scoped logger onto the context; handlers recover it via FromContext so
// every line carries the request id automatically.
func Example_contextBinding() {
	_ = log4go.SetupLog(log4go.LogConfig{
		Level:         log4go.LevelFlagInfo,
		ConsoleWriter: log4go.ConsoleWriterOptions{Enable: true, Level: log4go.LevelFlagInfo},
	})
	defer log4go.Close()

	// \"middleware\": build a logger, store it on the context
	reqLog := log4go.With("request_id", "r-1")
	ctx := reqLog.IntoContext(context.Background())

	// \"handler\": recover and log through it (carries request_id)
	log4go.FromContext(ctx).Info("handled request")
}

// Example_requestIDMiddleware shows the HTTP middleware: it resolves a request
// id (inbound header or generated), binds a child logger, and handlers log via
// FromContext.
func Example_requestIDMiddleware() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// every log line here carries request_id automatically
		log4go.FromContext(r.Context()).Info("served")
	})
	// wrap with the request-id middleware
	handler := log4go.RequestIDMiddleware(mux, log4go.RequestIDMiddlewareOpts{})
	_ = handler
	// http.ListenAndServe(":8080", handler)
}

// Example_netWriter shows shipping logs to a remote TCP collector. NetWriter is
// async + bounded + overflow-safe, so a slow/down remote never blocks the
// application. See PERFORMANCE.md for the net-throughput caveat.
func Example_netWriter() {
	nw := log4go.NewNetWriter(log4go.NetWriterOptions{
		Network: "tcp", Address: "collector.internal:514",
		BufferSize: 1024, OverflowPolicy: "drop", Timeout: 3 * time.Second,
	})
	log4go.Register(nw)
	defer log4go.Close()

	log4go.Info("shipped to collector")
}

// Example_kafkaWriter ships logs to Kafka via the async, bounded, overflow-safe
// KafkaWriter. BufferSize bounds the in-flight channel; under sustained pressure
// OverflowPolicy="spill" with SpillType="ring" absorbs bursts in an in-memory
// ring (re-sent once Kafka recovers) instead of dropping records or blocking the
// application. This is the foundation for Example_kafkaToES.
func Example_kafkaWriter() {
	kw := log4go.NewKafkaWriter(log4go.KafkaWriterOptions{
		Enable:         true,
		Level:          log4go.LevelFlagInfo,
		Brokers:        []string{"kafka-1:9092", "kafka-2:9092"},
		ProducerTopic:  "app-logs",
		BufferSize:     1 << 14, // 16384 in-flight records
		OverflowPolicy: "spill", // burst absorption (no drop, no OOM)
		SpillType:      "ring",
		SpillSize:      1 << 16,
	})
	log4go.Register(kw)
	defer log4go.Close()

	log4go.Info("shipped to kafka")
}

// Example_kafkaToES shows the end-to-end Kafka→Elasticsearch pipeline:
//
//	app (log4go) ──KafkaWriter──▶ Kafka ──Filebeat/Logstash──▶ Elasticsearch
//
// Field management is unified with Base Fields: SetBaseField registers the
// global static fields (hostname/server_ip/app/env) every record carries, and
// es_index is set as a Base Field so the Kafka payload itself names the target
// ES index — the consumer routes on it with no extra parsing. The payload also
// carries the strict-ordering keys (unix_nano + seq), so ES sorts by seq then
// unix_nano to reconstruct exact emit order across partitions/cores.
//
// The legacy KafkaMSGFields (MSG.ServerIP / MSG.ESIndex / MSG.ExtraFields) is
// kept only as a fallback: a Base Field of the same key always wins, so prefer
// SetBaseField(s) for new code.
func Example_kafkaToES() {
	// 1) global static fields — set once at startup, carried by EVERY record.
	log4go.SetBaseField("hostname", "adx-prod-01")
	log4go.SetBaseField("server_ip", "10.0.1.5")
	log4go.SetBaseField("app", "adx-dsp")
	log4go.SetBaseField("env", "prod")
	// es_index routes the record to its ES index. Static here; for time-sharded
	// indices (adx-logs-2026.06.26) prefer computing it on the consumer side
	// from @timestamp (see Filebeat config below) so the index rolls daily
	// without an app redeploy.
	log4go.SetBaseField("es_index", "adx-logs-2026.06")

	// 2) Kafka writer — async, bounded, spill-to-ring on backpressure.
	kw := log4go.NewKafkaWriter(log4go.KafkaWriterOptions{
		Enable:         true,
		Level:          log4go.LevelFlagInfo,
		Brokers:        []string{"kafka-1:9092"},
		ProducerTopic:  "adx-logs",
		BufferSize:     1 << 14,
		OverflowPolicy: "spill",
		SpillType:      "ring",
		SpillSize:      1 << 16,
	})
	log4go.Register(kw)
	defer log4go.Close()

	// 3) per-request business fields via With (a child logger).
	reqLog := log4go.With("trace_id", "a1b2c3")
	reqLog.Info("bid won")

	// Kafka record value (one JSON object — field order is fixed for the
	// framework/routing keys; user-field order follows map iteration):
	//
	//   {"unix_nano":1782392990123456789,"seq":42,"level":"INFO",
	//    "message":"bid won","timestamp":"2026-06-26T12:00:00.123456+08:00",
	//    "now":1782392990,
	//    "server_ip":"10.0.1.5","es_index":"adx-logs-2026.06",
	//    "hostname":"adx-prod-01","app":"adx-dsp","env":"prod","trace_id":"a1b2c3"}
	//
	// Filebeat consumer — route on es_index, or roll daily from @timestamp:
	//
	//   kafka: { topics: [adx-logs], group_id: filebeat }
	//   output.elasticsearch:
	//     indices:
	//       - index: "adx-logs-2026.06"
	//         when.equals: { es_index: "adx-logs-2026.06" }
	//     # or dynamic daily index derived from the record timestamp:
	//     # index: "adx-logs-%{+YYYY.MM.dd}"
}

// Example_multiWriterAlerts shows one logger fanning out to several destinations
// at different severities — the multi-sink routing pattern. Each Writer filters
// by its own level, so a single log call is delivered to exactly the right
// sinks:
//
//	kafka  (INFO+)  → full volume, async + spill-to-ring, feeds the Kafka→ES
//	                  pipeline (see Example_kafkaToES)
//	net    (WARN+)  → hot lines to a TCP/UDP collector
//	webhook(ERROR+) → severe lines only, pushed to lark/dingtalk
//
// The webhook layers two trigger controls on top of its ERROR level: a Filter
// (only forward records the predicate accepts) and a RateAlerter gate (forward
// at most ~once/min once errors exceed 10/min — storm suppression). defer
// log4go.Close() flushes every writer and, because WebhookWriter implements
// io.Closer, also stops the webhook sink daemon.
func Example_multiWriterAlerts() {
	// 1) Kafka — INFO+, full volume, burst-safe (spill-to-ring on backpressure).
	kafka := log4go.NewKafkaWriter(log4go.KafkaWriterOptions{
		Enable:         true,
		Level:          log4go.LevelFlagInfo,
		Brokers:        []string{"kafka-1:9092"},
		ProducerTopic:  "app-logs",
		BufferSize:     1 << 14,
		OverflowPolicy: "spill",
		SpillType:      "ring",
		SpillSize:      1 << 16,
	})
	log4go.Register(kafka)

	// 2) Net — WARN+ to a TCP collector (async, bounded, drop-on-full).
	nw := log4go.NewNetWriter(log4go.NetWriterOptions{
		Network:        "tcp",
		Address:        "collector.internal:514",
		Level:          log4go.LevelFlagWarning,
		BufferSize:     1024,
		OverflowPolicy: "drop",
	})
	log4go.Register(nw)

	// 3) Webhook — ERROR+ to lark, with a filter + a rate gate.
	sink := log4go.NewWebhookAlertSink(
		"https://oapi.example.com/lark/xxx", 256, log4go.LarkTextFormatter(""))
	sink.SetRateLimit(5)  // sink-level guard: at most 5 posts/sec
	sink.SetMaxRetries(2) // retry a failed post twice

	webhook := log4go.NewWebhookWriter(sink, log4go.WebhookWriterOptions{
		Level: log4go.LevelFlagError,
		// Forward only payment-domain errors (MatchField) that mention a failure
		// keyword (MatchKeyword). AllOf composes the two predicates.
		Filter: log4go.AllOf(
			log4go.MatchField("domain", "pay"),
			log4go.MatchKeyword("fail"),
		),
		Gate:          log4go.NewRateAlerter(time.Minute, 10), // >=10/min, ~1 fire/min
		RateFormatter: log4go.DefaultRateWebhookFormatter,     // payload: "[N in window] ..."
	})
	log4go.Register(webhook)

	defer log4go.Close() // flushes kafka/net; closes the webhook sink

	log4go.Info("started")                               // → kafka only
	log4go.Warn("cache degraded")                        // → kafka + net
	log4go.Error("db timeout")                           // → kafka + net; webhook filter rejects (not domain=pay)
	log4go.With("domain", "pay").Error("payment failed") // → kafka + net + webhook (domain=pay + "fail", past threshold)
}

// Example_presets shows the one-line production / development configurations
// (mirrors zap.NewProduction / NewDevelopment).
func Example_presets() {
	prod := log4go.NewProduction() // JSON + INFO + sampling + caller + console
	defer prod.Close()
	prod.Info("production line")

	dev := log4go.NewDevelopment() // colored text + DEBUG + funcname + console
	defer dev.Close()
	dev.Debug("debug line")
}

// Example_typedFields shows the allocation-free typed field constructors
// (WithString/WithInt/...), the counterpart to zap.Field / slog.Attr. Scalars
// never box into any.
func Example_typedFields() {
	_ = log4go.SetupLog(log4go.LogConfig{
		Level:         log4go.LevelFlagInfo,
		Format:        "json",
		ConsoleWriter: log4go.ConsoleWriterOptions{Enable: true, Level: log4go.LevelFlagInfo},
	})
	defer log4go.Close()

	log4go.WithString("trace_id", "t-1").
		WithInt("status", 200).
		WithDuration("elapsed", 3*time.Millisecond).
		Info("served")
	// {"...","msg":"served","fields":{"trace_id":"t-1","status":200,"elapsed":3000000}}
}

// Example_slogBridge routes the standard library log/slog (and with it net/http
// and any third-party lib using slog) through the log4go pipeline, so its
// writers, overflow protection and alerting all apply.
func Example_slogBridge() {
	lg := log4go.NewProduction()
	defer lg.Close()
	slog.SetDefault(slog.New(log4go.NewSlogHandler(lg)))

	slog.Info("via slog", "route", "/api/v1", "status", 200)
	// net/http and other slog-using libs now log through log4go too.
}

// Example_logfmt shows the Loki/Promtail/docker-friendly key=value format.
func Example_logfmt() {
	lg := log4go.NewLogger()
	defer lg.Close()
	lg.SetFormat(log4go.FormatLogfmt)
	lg.SetLevel(log4go.DEBUG)
	lg.Register(log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{Enable: true}))

	lg.WithString("trace_id", "t-1").Info("logfmt line")
	// time=2026-06-26T... level=INFO msg="logfmt line" trace_id=t-1
}
