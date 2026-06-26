package log4go_test

// This file follows the Go standard library convention: ExampleXxx functions
// appear in godoc as runnable examples. Unlike the _test.go examples in
// example_test.go (which use the package singleton and are for quick reference),
// these focus on constructor patterns, per-type API surface, and stdlib-style
// ExampleNewXxx / ExampleType_Method naming.

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/v8fg/kit4go/log4go"
)

// ExampleNewLogger shows the base constructor + level/format/caller knobs.
func ExampleNewLogger() {
	lg := log4go.NewLogger()
	defer lg.Close()
	lg.SetLevel(log4go.INFO)
	lg.SetFormat(log4go.FormatJSON)
	lg.WithCaller(true)
	lg.Register(log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{
		Enable: true,
		Level:  log4go.LevelFlagInfo,
	}))

	lg.Info("server started on :%d", 8080)
}

// ExampleNewKafKaWriter shows the Kafka writer constructor with overflow config.
func ExampleNewKafKaWriter() {
	kw := log4go.NewKafKaWriter(log4go.KafKaWriterOptions{
		Enable:         true,
		Level:          log4go.LevelFlagInfo,
		Brokers:        []string{"kafka-1:9092"},
		ProducerTopic:  "app-logs",
		BufferSize:     1 << 14,
		OverflowPolicy: "spill",
		SpillType:      "ring",
		SpillSize:      1 << 16,
	})
	log4go.Register(kw)
	defer log4go.Close()

	log4go.Info("sent to kafka")
}

// ExampleNewFileWriter shows daily-rotating async file logging.
func ExampleNewFileWriter() {
	fw := log4go.NewFileWriterWithOptions(log4go.FileWriterOptions{
		Enable:          true,
		Level:           log4go.LevelFlagInfo,
		Filename:        "/var/log/app-%Y%M%D.log",
		Rotate:          true,
		Daily:           true,
		MaxDays:         14,
		Async:           true,
		AsyncBufferSize: 8192,
		OverflowPolicy:  "spill",
		SpillType:       "ring",
		SpillSize:       8192,
	})
	log4go.Register(fw)
	defer log4go.Close()

	log4go.Info("written to file")
}

// ExampleNewConsoleWriter shows the three console modes.
func ExampleNewConsoleWriter() {
	// Plain (production-safe)
	log4go.Register(log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{
		Enable: true,
		Level:  log4go.LevelFlagInfo,
	}))

	// Colored (development)
	_ = log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{
		Enable: true, Color: true, FullColor: true,
		Level: log4go.LevelFlagDebug,
	})

	// Buffered (container stdout)
	_ = log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{
		Enable: true, Buffered: true, BufferSize: 8192,
		Level: log4go.LevelFlagInfo,
	})
}

// ExampleNewNetWriter shows TCP log shipping.
func ExampleNewNetWriter() {
	nw := log4go.NewNetWriter(log4go.NetWriterOptions{
		Network:        "tcp",
		Address:        "collector.internal:514",
		Level:          log4go.LevelFlagWarning,
		BufferSize:     1024,
		OverflowPolicy: "drop",
		Timeout:        3 * time.Second,
	})
	log4go.Register(nw)
	defer log4go.Close()

	log4go.Warn("shipped to collector")
}

// ExampleNewWebhookAlertSink shows creating a Lark webhook sink.
func ExampleNewWebhookAlertSink() {
	sink := log4go.NewWebhookAlertSink(
		"https://oapi.example.com/lark/bot-xxx",
		256,
		log4go.LarkTextFormatter(""),
	)
	sink.SetRateLimit(10)
	sink.SetMaxRetries(2)
	// Wrap in a WebhookWriter for level-filtered alerting:
	_ = log4go.NewWebhookWriter(sink, log4go.WebhookWriterOptions{
		Level: log4go.LevelFlagError,
	})
}

// ExampleNewWebhookWriter shows the full alerting pipeline.
func ExampleNewWebhookWriter() {
	sink := log4go.NewWebhookAlertSink(
		"https://oapi.example.com/dingtalk/robot/send?token=xxx",
		256,
		log4go.DingtalkTextFormatter(""),
	)
	w := log4go.NewWebhookWriter(sink, log4go.WebhookWriterOptions{
		Level: log4go.LevelFlagError,
		Filter: log4go.AllOf(
			log4go.MatchField("domain", "payment"),
			log4go.MatchKeyword("fail"),
		),
		Gate:          log4go.NewRateAlerter(time.Minute, 5),
		RateFormatter: log4go.DefaultRateWebhookFormatter,
	})
	log4go.Register(w)
	defer log4go.Close()
}

// ExampleNewSlogHandler routes slog through log4go.
func ExampleNewSlogHandler() {
	lg := log4go.NewProduction()
	defer lg.Close()

	handler := log4go.NewSlogHandler(lg)
	_ = handler // pass to slog.New(handler)
}

// ExampleNewShardLogger shows multi-core sharding.
func ExampleNewShardLogger() {
	sl := log4go.NewShardLogger(0) // 0 = auto
	defer sl.Close()
	sl.SetLevel(log4go.INFO)
	sl.RegisterFunc(func() log4go.Writer {
		return log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{
			Enable: true, Level: log4go.LevelFlagInfo,
		})
	})
	sl.Info("sharded line")
}

// ExampleNewShardLoggerWithOptions shows structured shard config.
func ExampleNewShardLoggerWithOptions() {
	sl := log4go.NewShardLoggerWithOptions(log4go.ShardLoggerOptions{
		Shards:      0,
		Level:       log4go.LevelFlagInfo,
		ChannelSize: 8192,
	})
	defer sl.Close()
}

// ExampleNewRateAlerter shows rate-based alert gating.
func ExampleNewRateAlerter() {
	// Fire at most once per minute when >= 5 events accumulate:
	gate := log4go.NewRateAlerter(time.Minute, 5)
	_ = gate // pass to WebhookWriterOptions.Gate
}

// ExampleNewProduction shows the one-liner production preset.
func ExampleNewProduction() {
	lg := log4go.NewProduction()
	defer lg.Close()
	lg.Info("production started")
}

// ExampleNewDevelopment shows the one-liner development preset.
func ExampleNewDevelopment() {
	lg := log4go.NewDevelopment()
	defer lg.Close()
	lg.Debug("debug mode")
}

// ExampleLogger_With shows structured field chaining.
func ExampleLogger_With() {
	lg := log4go.NewProduction()
	defer lg.Close()

	lg.With("trace_id", "t-1").
		With("user_id", 42).
		Info("request handled")
}

// ExampleLogger_WithString shows typed field constructors.
func ExampleLogger_WithString() {
	lg := log4go.NewProduction()
	defer lg.Close()

	lg.WithString("trace_id", "abc").
		WithInt("status", 200).
		WithDuration("elapsed", 3*time.Millisecond).
		Info("served")
}

// ExampleLogger_WithAttrs shows batch typed field attachment.
func ExampleLogger_WithAttrs() {
	lg := log4go.NewProduction()
	defer lg.Close()

	lg.WithAttrs(
		log4go.String("route", "/api/v1"),
		log4go.Int("items", 3),
		log4go.Bool("cached", true),
		log4go.Duration("db_time", 12*time.Microsecond),
	).Info("order placed")
}

// ExampleLogger_WithBytes shows []byte field (base64 in JSON).
func ExampleLogger_WithBytes() {
	lg := log4go.NewProduction()
	defer lg.Close()

	raw := []byte{0x01, 0x02, 0x03}
	lg.WithBytes("payload", raw).Info("binary data logged")
}

// ExampleLogger_WithError shows error-typed fields.
func ExampleLogger_WithError() {
	lg := log4go.NewProduction()
	defer lg.Close()

	err := fmt.Errorf("connection refused")
	lg.WithError("cause", err).Error("db unreachable")
}

// ExampleLogger_WithSampling shows log storm protection.
func ExampleLogger_WithSampling() {
	lg := log4go.NewProduction()
	defer lg.Close()

	sampled := lg.WithSampling(10, 100)
	for i := 0; i < 1000; i++ {
		sampled.Info("high frequency event %d", i) // ~10 + 9 = 19 logged
	}
}

// ExampleLogger_WithContext shows request-scoped context logging.
func ExampleLogger_WithContext() {
	lg := log4go.NewProduction()
	defer lg.Close()

	reqLog := lg.With("request_id", "req-abc")
	ctx := reqLog.IntoContext(context.Background())

	// Later, in a handler:
	recovered := log4go.FromContext(ctx)
	recovered.Info("handled request") // carries request_id automatically
}

// ExampleLogger_Trace shows the finest log level.
func ExampleLogger_Trace() {
	lg := log4go.NewDevelopment() // DEBUG+ in dev; set TRACE for troubleshooting
	defer lg.Close()
	lg.SetLevel(log4go.TRACE)

	lg.Trace("loop i=%d val=%v", 42, "debug value")
}

// ExampleLogger_WithCaller shows caller info control.
func ExampleLogger_WithCaller() {
	lg := log4go.NewLogger()
	defer lg.Close()
	lg.Register(log4go.NewConsoleWriterWithOptions(log4go.ConsoleWriterOptions{
		Enable: true, Level: log4go.LevelFlagInfo,
	}))

	lg.WithCaller(true)  // includes file:line
	lg.Info("with caller")

	lg.WithCaller(false) // skip runtime.Caller (max throughput)
	lg.Info("without caller")
}

// ExampleSetBaseField shows global static fields.
func ExampleSetBaseField() {
	log4go.SetBaseField("hostname", "prod-01")
	log4go.SetBaseField("app", "payment-svc")
	log4go.SetBaseField("es_index", "payment-2026.06")
	defer log4go.Close()

	// Every record now carries hostname/app/es_index
	log4go.Info("started")
}

// ExampleSetBaseFields shows batch global field registration.
func ExampleSetBaseFields() {
	log4go.SetBaseFields(map[string]interface{}{
		"hostname":  "prod-01",
		"server_ip": "10.0.1.5",
		"env":       "prod",
	})
	defer log4go.Close()

	log4go.Info("all records carry these fields")
}

// ExampleSetFormat shows the three output formats.
func ExampleSetFormat() {
	defer log4go.Close()

	log4go.SetFormat(log4go.FormatJSON)
	log4go.Info("json line")

	log4go.SetFormat(log4go.FormatLogfmt)
	log4go.Info("logfmt line")

	log4go.SetFormat(log4go.FormatText)
	log4go.Info("text line")
}

// ExamplePanic shows crash logging (recovered).
func ExamplePanic() {
	lg := log4go.NewProduction()
	defer func() {
		_ = recover()
		lg.Close()
	}()

	lg.Panic("invariant violated: %v", "bad state")
}

// ExampleRecover shows panic capture.
func ExampleRecover() {
	lg := log4go.NewProduction()
	defer lg.Close()

	func() {
		defer log4go.Recover(func() *log4go.Logger { return lg })
		panic("unexpected")
	}()
}

// ExampleTrace shows the package-level Trace helper.
func ExampleTrace() {
	log4go.SetLevel(log4go.TRACE)
	defer log4go.Close()
	log4go.Trace("finest detail: val=%v", 42)
}

// ExampleMetrics shows per-level record counts.
func ExampleMetrics() {
	defer log4go.Close()
	log4go.Info("counted")
	log4go.Error("counted")
	time.Sleep(100 * time.Millisecond)

	m := log4go.Metrics()
	_ = m.Records[log4go.INFO]
	_ = m.Records[log4go.ERROR]
}

// ExampleRuntimeStats shows runtime memory diagnostics.
func ExampleRuntimeStats() {
	rs := log4go.RuntimeStats()
	fmt.Printf("GOMAXPROCS=%d goroutines=%d heap=%dMB gc%%=%.4f\n",
		rs.GOMAXPROCS, rs.NumGoroutine, rs.HeapAlloc/1e6, rs.GCCPUFraction)
}

// ExampleRequestIDMiddleware shows the built-in HTTP middleware.
func ExampleRequestIDMiddleware() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log4go.FromContext(r.Context()).Info("served")
	})
	handler := log4go.RequestIDMiddleware(mux, log4go.RequestIDMiddlewareOpts{})
	_ = handler
	// http.ListenAndServe(":8080", handler)
}

// ExampleMatchField shows field-based filtering.
func ExampleMatchField() {
	f := log4go.MatchField("domain", "payment")
	_ = f // pass to WebhookWriterOptions.Filter
}

// ExampleMatchKeyword shows message-based filtering.
func ExampleMatchKeyword() {
	f := log4go.MatchKeyword("fail")
	_ = f
}

// ExampleAllOf shows filter composition (AND).
func ExampleAllOf() {
	f := log4go.AllOf(
		log4go.MatchField("domain", "payment"),
		log4go.MatchKeyword("fail"),
	)
	_ = f
}

// ExampleAutoShardCount shows the auto shard sizing function.
func ExampleAutoShardCount() {
	n := log4go.AutoShardCount()
	fmt.Printf("recommended shards for this machine: %d\n", n)
}

// ExampleField_constructors shows the typed field constructors.
func ExampleField_constructors() {
	f := log4go.String("trace_id", "abc-123")
	fmt.Printf("key=%s kind=%d value=%v\n", f.Key(), f.Kind(), f.Value())
}
