package log4go_test

// This file holds runnable Example* functions (rendered in godoc) that show
// idiomatic use of the log4go package API. Each example is self-contained and
// uses the package-level singleton, matching how most applications configure
// logging once at startup.

import (
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
