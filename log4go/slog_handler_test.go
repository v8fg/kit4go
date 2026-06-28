package log4go

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// Test_SlogHandler_LevelMapping checks slog levels map to log4go levels.
func Test_SlogHandler_LevelMapping(t *testing.T) {
	cases := []struct {
		sl   slog.Level
		want int
	}{
		{slog.LevelDebug - 4, TRACE}, // below slog Debug -> log4go TRACE
		{slog.LevelDebug, DEBUG},
		{slog.LevelInfo, INFO},
		{slog.LevelWarn, WARNING},
		{slog.LevelError, ERROR},
	}
	for _, c := range cases {
		if got := slogToLog4goLevel(c.sl); got != c.want {
			t.Errorf("slogToLog4goLevel(%v)=%d want %d", c.sl, got, c.want)
		}
	}
}

// Test_SlogHandler_Enabled checks the level gate honors the logger threshold.
func Test_SlogHandler_Enabled(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(WARNING)
	h := NewSlogHandler(root)

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("DEBUG should be disabled at WARNING threshold")
	}
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("INFO should be disabled at WARNING threshold")
	}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("WARN should be enabled at WARNING threshold")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("ERROR should be enabled at WARNING threshold")
	}
}

// Test_SlogHandler_HandleAttrs drives a real slog.Logger through the handler and
// asserts the record reaches the writer with msg + typed attrs, in JSON format.
func Test_SlogHandler_HandleAttrs(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)
	root.SetFormat(FormatJSON)

	cw := &captureWriter{}
	root.Register(cw)

	sl := slog.New(NewSlogHandler(root))
	sl.Info("sourced via slog", "trace_id", "t-1", "count", 42, "ok", true)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
	}
	if cw.Len() == 0 {
		t.Fatal("record never reached writer")
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()

	if r.msg != "sourced via slog" {
		t.Errorf("msg=%q", r.msg)
	}
	// typed attrs preserved as fields
	got := map[string]interface{}{}
	for _, f := range r.fields {
		got[f.key] = f.value()
	}
	if got["trace_id"] != "t-1" {
		t.Errorf("trace_id=%v", got["trace_id"])
	}
	if got["count"] != int64(42) {
		t.Errorf("count=%v(%T) want int64 42", got["count"], got["count"])
	}
	if got["ok"] != true {
		t.Errorf("ok=%v", got["ok"])
	}
	// JSON pre-serialized and contains the attr
	if !strings.Contains(string(r.formattedBytes), `"count":42`) {
		t.Errorf("formattedBytes missing count: %s", r.formattedBytes)
	}
}

// Test_SlogHandler_WithAttrsAndGroup verifies WithAttrs/WithGroup propagate.
func Test_SlogHandler_WithAttrsAndGroup(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)

	cw := &captureWriter{}
	root.Register(cw)

	sl := slog.New(NewSlogHandler(root)).With("svc", "api").WithGroup("req")
	sl.Info("handled", "id", "r-9")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
	}
	if cw.Len() == 0 {
		t.Fatal("record never reached writer")
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()

	got := map[string]interface{}{}
	for _, f := range r.fields {
		got[f.key] = f.value()
	}
	if got["svc"] != "api" {
		t.Errorf("With attr svc=%v want api", got["svc"])
	}
	if got["req.id"] != "r-9" {
		t.Errorf("grouped attr req.id=%v want r-9", got["req.id"])
	}
}

// Test_SlogHandler_BaseFieldsIncluded confirms base fields ride slog records too.
func Test_SlogHandler_BaseFieldsIncluded(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)
	root.SetBaseField("hostname", "h-1")

	cw := &captureWriter{}
	root.Register(cw)

	slog.New(NewSlogHandler(root)).Info("hi")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()
	found := false
	for _, f := range r.fields {
		if f.key == "hostname" && f.value() == "h-1" {
			found = true
		}
	}
	if !found {
		t.Error("base field hostname missing from slog record")
	}
}
