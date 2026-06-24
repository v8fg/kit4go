package log4go

import (
	"runtime"
	"sync/atomic"
	"testing"
)

// discardWriter is a no-op Writer used to benchmark the logging pipeline
// (record formatting + delivery) without I/O noise.
type discardWriter struct{}

func (discardWriter) Init() error         { return nil }
func (discardWriter) Write(*Record) error { return nil }

// countingWriter counts records actually written by the bootstrap goroutine.
type countingWriter struct {
	n int64
}

func (c *countingWriter) Init() error         { return nil }
func (c *countingWriter) Write(*Record) error { atomic.AddInt64(&c.n, 1); return nil }
func (c *countingWriter) Count() int64        { return atomic.LoadInt64(&c.n) }

func newBenchLogger() *Logger {
	records := make(chan *Record, recordChannelSizeDefault)
	return newLoggerWithRecords(records)
}

// Test_MemSustained_1M logs 1M records and reports process memory (Sys /
// HeapAlloc / HeapInuse) + GC count + goroutines — answers "how much memory
// does high-QPS logging actually occupy".
func Test_MemSustained_1M(t *testing.T) {
	lg := newBenchLogger()
	lg.SetLevel(DEBUG)
	lg.WithCaller(false)
	lg.Register(discardWriter{})

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	const n = 1_000_000
	for i := 0; i < n; i++ {
		lg.Info("x")
	}
	lg.Close()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	t.Logf("1M logs: Sys=%.1fMB HeapAlloc=%.1fMB HeapInuse=%.1fMB NumGC=%d Goroutines=%d",
		float64(after.Sys)/1e6,
		float64(after.HeapAlloc)/1e6,
		float64(after.HeapInuse)/1e6,
		after.NumGC-before.NumGC,
		runtime.NumGoroutine())
}

// Test_LevelFiltering verifies that records below the logger level are dropped
// before reaching writers, while records at/above the level are delivered.
func Test_LevelFiltering(t *testing.T) {
	lg := newBenchLogger()
	lg.SetLevel(ERROR) // only ERROR/CRITICAL/ALERT/EMERGENCY pass
	cw := &countingWriter{}
	lg.Register(cw)

	lg.Debug("filtered")
	lg.Info("filtered")
	lg.Warn("filtered")
	lg.Notice("filtered")
	lg.Error("pass")
	lg.Critical("pass")

	lg.Close() // drains the record channel before returning

	if got := cw.Count(); got != 2 {
		t.Errorf("level filtering: %d records written, want 2 (ERROR+CRITICAL)", got)
	}
}

// Test_LoggerDeliversRecords verifies records flow through to a writer.
func Test_LoggerDeliversRecords(t *testing.T) {
	lg := newBenchLogger()
	lg.SetLevel(DEBUG)
	cw := &countingWriter{}
	lg.Register(cw)

	const n = 100
	for i := 0; i < n; i++ {
		lg.Info("record %d", i)
	}
	lg.Close()

	if got := cw.Count(); got != n {
		t.Errorf("delivered %d records, want %d", got, n)
	}
}

// Benchmark_LoggerInfo measures the cost of a passing log call:
// formatting + runtime.Caller + async record delivery.
func Benchmark_LoggerInfo(b *testing.B) {
	lg := newBenchLogger()
	lg.SetLevel(DEBUG)
	lg.Register(discardWriter{})
	defer lg.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lg.Info("bench info iter=%d", i)
	}
}

// Benchmark_LoggerInfoNoCaller measures cost with runtime.Caller disabled
// (no file:line) — the max-throughput mode.
func Benchmark_LoggerInfoNoCaller(b *testing.B) {
	lg := newBenchLogger()
	lg.SetLevel(DEBUG)
	lg.WithCaller(false)
	lg.Register(discardWriter{})
	defer lg.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lg.Info("x")
	}
}

// Benchmark_LoggerFiltered measures the cost of a log call that is filtered
// out by level — should be near-free (early return before formatting).
func Benchmark_LoggerFiltered(b *testing.B) {
	lg := newBenchLogger()
	lg.SetLevel(EMERGENCY) // DEBUG filtered out
	lg.Register(discardWriter{})
	defer lg.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lg.Debug("filtered iter=%d", i)
	}
}

// Benchmark_LoggerParallel measures throughput under concurrent producers.
func Benchmark_LoggerParallel(b *testing.B) {
	lg := newBenchLogger()
	lg.SetLevel(DEBUG)
	lg.Register(discardWriter{})
	defer lg.Close()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lg.Info("parallel info")
		}
	})
}
