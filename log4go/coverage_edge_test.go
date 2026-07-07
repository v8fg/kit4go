package log4go

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ===================== context.go edge cases =====================

func Test_IntoContext_NilLogger(t *testing.T) {
	resetDefaultForTest(t)
	// IntoContext stores nil; FromContext falls back to defaultLogger (non-nil)
	ctx := (*Logger)(nil).IntoContext(context.Background())
	// The stored value is nil pointer; FromContext should fall back to singleton
	l := FromContext(ctx)
	_ = l // l is the default logger (non-nil), not the nil we stored
}

func Test_FromContext_EmptyContext(t *testing.T) {
	resetDefaultForTest(t)
	// Empty context returns the default logger (not nil — it's a fallback)
	l := FromContext(context.Background())
	if l == nil {
		t.Error("FromContext should return default logger, not nil")
	}
}

func Test_RequestIDFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), RequestIDContextKey, "req-123")
	if got := RequestIDFromContext(ctx); got != "req-123" {
		t.Errorf("got %q", got)
	}
	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Errorf("empty got %q", got)
	}
}

func Test_RequestIDMiddleware_NoHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromContext(r.Context()) == "" {
			t.Error("should generate request ID")
		}
	}), RequestIDMiddlewareOpts{}).ServeHTTP(w, req)
}

func Test_RequestIDMiddleware_WithHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", "client-rid")
	w := httptest.NewRecorder()
	RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestIDFromContext(r.Context()); got != "client-rid" {
			t.Errorf("got %q", got)
		}
	}), RequestIDMiddlewareOpts{Header: "X-Request-Id"}).ServeHTTP(w, req)
}

func Test_AddContextExtractor_Multiple(t *testing.T) {
	// Save and restore the global extractor stack
	extractorMu.Lock()
	oldRef := extractorSnapshotRef.Load()
	extractorMu.Unlock()
	defer func() {
		extractorMu.Lock()
		extractorSnapshotRef.Store(oldRef)
		extractorMu.Unlock()
	}()

	AddContextExtractor(func(_ context.Context) map[string]any { return map[string]any{"a": 1} })
	AddContextExtractor(func(_ context.Context) map[string]any { return map[string]any{"b": 2} })

	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	root.SetLevel(DEBUG)
	cw := &captureWriter{}
	root.Register(cw)
	root.WithContext(context.Background()).Info("ctx test")
	waitForRecord(cw, t)
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()
	keys := map[string]bool{}
	for _, f := range r.fields {
		keys[f.key] = true
	}
	if !keys["a"] || !keys["b"] {
		t.Errorf("custom extractors missing: keys=%v", keys)
	}
}

// ===================== config.go edge cases =====================

func Test_SetLog_Bytes(t *testing.T) {
	resetDefaultForTest(t)
	if err := SetLog([]byte(`{"level":"ERROR","format":"text"}`)); err != nil {
		t.Fatalf("SetLog: %v", err)
	}
}

func Test_SetLog_InvalidJSON(t *testing.T) {
	resetDefaultForTest(t)
	if err := SetLog([]byte(`invalid`)); err == nil {
		t.Error("expected error")
	}
}

func Test_SetLogWithConf_NonexistentFile(t *testing.T) {
	if err := SetLogWithConf("/nonexistent/file.json"); err == nil {
		t.Error("expected error")
	}
}

func Test_SetLogWithConf_InvalidJSON(t *testing.T) {
	path := t.TempDir() + "/bad.json"
	_ = os.WriteFile(path, []byte(`not json`), 0644)
	if err := SetLogWithConf(path); err == nil {
		t.Error("expected error")
	}
}

func Test_SetupLog_AllWriters(t *testing.T) {
	resetDefaultForTest(t)
	dir := t.TempDir()
	err := SetupLog(LogConfig{
		Level:         LevelFlagDebug,
		Format:        "json",
		ConsoleWriter: ConsoleWriterOptions{Enable: true, Level: LevelFlagInfo},
		FileWriter:    FileWriterOptions{Enable: true, Level: LevelFlagInfo, Filename: dir + "/t-%Y%M%D.log", Rotate: true, Daily: true, Async: true, AsyncBufferSize: 128, OverflowPolicy: "drop"},
		KafkaWriter:   KafkaWriterOptions{Enable: false},
	})
	if err != nil {
		t.Fatalf("SetupLog: %v", err)
	}
}

// ===================== json_codec error path =====================

func Test_JsonMarshalEncode_Error(t *testing.T) {
	if _, err := jsonMarshalEncode(make(chan int)); err == nil {
		t.Error("expected error for chan")
	}
}

// ===================== log.go edge cases =====================

func Test_Record_String_WithFields(t *testing.T) {
	r := &Record{level: INFO, time: "t", file: "f:1", msg: "m", fields: []field{fld("k", "v")}}
	if !strings.Contains(r.String(), "k") {
		t.Error("String missing field key")
	}
}

func Test_GetLevelDefault_Unknown(t *testing.T) {
	if getLevelDefault("BOGUS", DEBUG, "test") != DEBUG {
		t.Error("unknown should fallback")
	}
}

func Test_GetLevelDefault_Warn(t *testing.T) {
	if getLevelDefault("WARN", DEBUG, "test") != WARNING {
		t.Error("WARN should map")
	}
}

func Test_Logger_Clone(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	root.SetLevel(INFO)
	root.SetFormat(FormatJSON)
	child := root.clone()
	if int32(INFO) != child.level.Load() || LogFormat(child.format.Load()) != FormatJSON {
		t.Error("clone mismatch")
	}
}

// ===================== field.go escape edge =====================

func Test_AppendJSONStringContent_AllEscapes(t *testing.T) {
	for _, s := range []string{`"`, `\`, "\n", "\r", "\t", "\b", "\f", "\x01", "plain"} {
		quoted := appendJSONQuoted([]byte{}, s)
		var got string
		if err := json.Unmarshal(quoted, &got); err != nil {
			t.Errorf("round-trip fail %q: %v", s, err)
		}
		if got != s {
			t.Errorf("round-trip mismatch %q→%q", s, got)
		}
	}
}

// ===================== logfmt edge =====================

func Test_AppendLogfmtValue_NeedsQuote(t *testing.T) {
	for _, s := range []string{"hello world", `a"b`, "a=b", "a\\b", ""} {
		if len(appendLogfmtValue([]byte{}, s)) == 0 {
			t.Errorf("empty result for %q", s)
		}
	}
}

// ===================== alert.go edge =====================

func Test_NewWebhookAlertSink_NilFormatter(t *testing.T) {
	if NewWebhookAlertSink("http://example.com", 64, nil) == nil {
		t.Error("nil formatter should default")
	}
}

// ===================== filter.go edge =====================

func Test_MatchFieldIn_Empty(t *testing.T) {
	if MatchFieldIn("k")(&Record{fields: []field{fld("k", "v")}}) {
		t.Error("empty MatchFieldIn should not match")
	}
}

// ===================== sampling edge =====================

func Test_Sampler_Disabled(t *testing.T) {
	// newSampler always returns a *Sampler (initial/thereafter control behavior);
	// WithSampling disables by setting nil
	s := newSampler(0, 0)
	if s == nil {
		t.Error("newSampler(0,0) should return non-nil Sampler")
	}
}

// ===================== console edge =====================

func Test_ConsoleWriter_JSONPath_NoBuf(t *testing.T) {
	w := &ConsoleWriter{level: DEBUG}
	r := &Record{level: INFO, msg: "j", formattedBytes: []byte(`{"msg":"j"}` + "\n")}
	_ = w.Write(r)
}

// ===================== webhook edge =====================

func Test_WebhookWriter_CloseNilSink(t *testing.T) {
	if err := (&WebhookWriter{}).Close(); err != nil {
		t.Errorf("Close nil: %v", err)
	}
}

// ===================== slog handler edge =====================

func Test_SlogHandler_Handle_ZeroPC(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	root.SetLevel(DEBUG)
	h := NewSlogHandler(root)
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "no pc", 0)
	_ = h.Handle(context.Background(), r)
}

func Test_SlogHandler_Handle_WithAttrs(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	root.SetLevel(DEBUG)
	cw := &captureWriter{}
	root.Register(cw)
	h := NewSlogHandler(root)
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "attrs", 0)
	r.AddAttrs(slog.Int64("n", 42), slog.String("s", "v"))
	_ = h.Handle(context.Background(), r)
	waitForRecord(cw, t)
}

func Test_SlogHandler_WithAttrs_Nil(t *testing.T) {
	h := NewSlogHandler(newLoggerWithRecords(make(chan *Record, 4)))
	_ = h.WithAttrs(nil) // should not panic
}

// ===================== helpers =====================

func waitForRecord(cw *captureWriter, t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		runtime.Gosched()
	}
	if cw.Len() == 0 {
		t.Fatal("record never reached writer")
	}
}

func resetDefaultForTest(t *testing.T) {
	t.Helper()
	loggerDefault.Store((*Logger)(nil))
}
