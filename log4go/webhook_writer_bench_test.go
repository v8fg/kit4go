package log4go

import (
	"strings"
	"testing"
	"time"
)

func Benchmark_WebhookWriter_PassThrough(b *testing.B) {
	sink := &mockAlertSink{}
	w := NewWebhookWriter(sink, WebhookWriterOptions{
		Level:  "error",
		Filter: func(r *Record) bool { return strings.Contains(r.msg, "pay") },
		Gate:   NewRateAlerter(time.Second, 1<<30), // gate never suppresses (huge threshold)
	})
	r := &Record{level: ERROR, msg: "payment failed", file: "svc.go:1"}
	b.ReportAllocs()

	for b.Loop() {
		_ = w.Write(r)
	}
}

func Benchmark_WebhookWriter_LevelSkip(b *testing.B) {
	sink := &mockAlertSink{}
	w := NewWebhookWriter(sink, WebhookWriterOptions{Level: "error"})
	r := &Record{level: INFO, msg: "info line"} // below ERROR -> skipped fast
	b.ReportAllocs()

	for b.Loop() {
		_ = w.Write(r)
	}
}
