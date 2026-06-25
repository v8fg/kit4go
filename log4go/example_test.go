package log4go_test

// This file holds runnable Example* functions (rendered in godoc) that show
// idiomatic use of the log4go package API. Each example is self-contained and
// uses the package-level singleton, matching how most applications configure
// logging once at startup.

import (
	"context"
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
// text format (and as top-level keys in FormatJSON / KafKaWriter).
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
	log4go.WithFields(map[string]interface{}{"k1": "v1", "k2": 2}).Info("batch fields")
}

// Example_jsonFormat shows structured JSON output (one JSON object per record),
// the convention Fluentd/Filebeat expect. Time uses the ISO layout; fields are
// hoisted into a \"fields\" object (omitted when empty).
func Example_jsonFormat() {
	_ = log4go.SetupLog(log4go.LogConfig{
		Level:  log4go.LevelFlagInfo,
		Format: "json", // FormatJSON
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
