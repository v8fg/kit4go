package log4go

import (
	"log/slog"
	"testing"
	"time"
)

// Benchmark_Record_Logfmt measures the logfmt pre-serialization path (typed
// append, no map), comparable to Benchmark_Record_JSON_Goccy.
func Benchmark_Record_Logfmt(b *testing.B) {
	r := &Record{
		level:    INFO,
		time:     "2026-06-25T15:04:05.000+0800",
		file:     "svc.go:42",
		msg:      "benchmark writer message payload",
		unixNano: 1782392990_123456789,
		fields:   []field{fld("trace_id", "abc"), fld("user", 42), fld("route", "/api/v1")},
	}
	b.ReportAllocs()

	for b.Loop() {
		_ = r.Logfmt()
	}
}

// Benchmark_SlogHandler_Handle measures the slog->log4go bridge overhead vs a
// native log4go call (Benchmark_DeliverPipeline_Discard). The gap is the slog
// Record + attr conversion cost.
func Benchmark_SlogHandler_Handle(b *testing.B) {
	lg := newBenchLogger()
	lg.SetLevel(DEBUG)
	lg.Register(discardWriter{})
	defer lg.Close()
	sl := slog.New(NewSlogHandler(lg))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sl.Info("slog line", "trace_id", "t", "n", i)
	}
}

// Benchmark_Logger_WithAttrs_Typed vs WithInterface: typed Fields attached in
// one clone, allocation-free for scalars.
func Benchmark_Logger_WithAttrs_Typed(b *testing.B) {
	root := newBenchLogger()
	defer root.Close()
	attrs := []Field{String("trace_id", "t"), Int("n", 42), Bool("ok", true)}
	b.ReportAllocs()

	for b.Loop() {
		_ = root.WithAttrs(attrs...)
	}
}

// Benchmark_Logger_DeliverTypedFields: a full deliver with 3 typed fields +
// JSON format, the realistic structured-log hot path end to end.
func Benchmark_Logger_DeliverTypedFields(b *testing.B) {
	lg := newBenchLogger()
	lg.SetLevel(DEBUG)
	lg.SetFormat(FormatJSON)
	lg.Register(discardWriter{})
	defer lg.Close()
	b.ReportAllocs()

	for b.Loop() {
		lg.WithAttrs(String("trace_id", "t"), Int("user", 42), Duration("elapsed", time.Millisecond)).Info("served")
	}
}
