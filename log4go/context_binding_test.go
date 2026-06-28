package log4go

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Test_IntoContext_FromContext_RoundTrip is the core zerolog-style binding:
// IntoContext stores a logger; FromContext recovers it (not a child, not the
// singleton) with its structured fields intact.
func Test_IntoContext_FromContext_RoundTrip(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()

	bound := root.With("request_id", "r-1").WithField("user", 7)
	ctx := bound.IntoContext(context.Background())

	got := FromContext(ctx)
	if got != bound {
		t.Fatal("FromContext did not return the exact bound logger")
	}
	// fields must be intact
	if len(got.fields) != 2 {
		t.Fatalf("bound logger fields lost: %v", got.fields)
	}
}

// Test_FromContext_DefaultOnMissing confirms FromContext falls back to the
// package singleton when no logger is bound (so callers never get nil).
func Test_FromContext_DefaultOnMissing(t *testing.T) {
	got := FromContext(context.Background())
	if got == nil {
		t.Fatal("FromContext returned nil")
	}
	// it should be the singleton (same records channel identity is enough)
	if got != defaultLogger() {
		t.Fatal("FromContext did not fall back to the singleton")
	}
}

// Test_FromContext_NilContext confirms FromContext(nil) is safe.
func Test_FromContext_NilContext(t *testing.T) {
	got := FromContext(nil)
	if got == nil {
		t.Fatal("FromContext(nil) returned nil")
	}
}

// Test_WithLogger confirms the functional helper stores a logger (nil falls
// back to the singleton).
func Test_WithLogger(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()

	ctx := WithLogger(context.Background(), root)
	if FromContext(ctx) != root {
		t.Fatal("WithLogger did not store the logger")
	}
	// nil -> singleton
	ctx2 := WithLogger(context.Background(), nil)
	if FromContext(ctx2) != defaultLogger() {
		t.Fatal("WithLogger(nil) did not fall back to singleton")
	}
}

// Test_Logger_FromContext_Method confirms the method form prefers the bound
// logger but falls back to the receiver when unbound.
func Test_Logger_FromContext_Method(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	fallback := newLoggerWithRecords(make(chan *Record, 4))
	defer fallback.Close()

	// unbound ctx -> returns the receiver (fallback)
	if got := fallback.FromContext(context.Background()); got != fallback {
		t.Fatal("method FromContext did not fall back to receiver")
	}
	// bound ctx -> returns the bound logger
	bound := root.With("k", "v")
	ctx := bound.IntoContext(context.Background())
	if got := fallback.FromContext(ctx); got != bound {
		t.Fatal("method FromContext did not prefer the bound logger")
	}
}

// Test_AddContextExtractor_Merge confirms multiple extractors stack and merge,
// with later extractors overriding earlier keys (last-writer-wins).
func Test_AddContextExtractor_Merge(t *testing.T) {
	resetExtractorStack()
	defer resetExtractorStack()

	AddContextExtractor(func(ctx context.Context) map[string]interface{} {
		return map[string]interface{}{"a": 1, "shared": "first"}
	})
	AddContextExtractor(func(ctx context.Context) map[string]interface{} {
		return map[string]interface{}{"b": 2, "shared": "second"} // overrides shared
	})

	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	child := root.WithContext(context.Background())

	got := map[string]interface{}{}
	for _, f := range child.fields {
		got[f.key] = f.value()
	}
	if got["a"] != 1 || got["b"] != 2 {
		t.Errorf("merge lost keys: %v", got)
	}
	if got["shared"] != "second" {
		t.Errorf("last-writer-wins failed: shared=%v want second", got["shared"])
	}
}

// Test_AddContextExtractor_DefaultStillRuns confirms AddContextExtractor ADDS to
// the default stack rather than replacing it (the default trace-id probe still
// runs alongside the custom one).
func Test_AddContextExtractor_DefaultStillRuns(t *testing.T) {
	resetExtractorStack()
	defer resetExtractorStack()

	AddContextExtractor(func(ctx context.Context) map[string]interface{} {
		return map[string]interface{}{"custom": "yes"}
	})

	ctx := context.WithValue(context.Background(), "trace_id", "t-9")
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	child := root.WithContext(ctx)

	got := map[string]interface{}{}
	for _, f := range child.fields {
		got[f.key] = f.value()
	}
	if got["trace_id"] != "t-9" {
		t.Errorf("default extractor did not run: %v", got)
	}
	if got["custom"] != "yes" {
		t.Errorf("custom extractor did not run: %v", got)
	}
}

// Test_AddContextExtractor_Concurrent confirms AddContextExtractor is safe under
// concurrent registration (race detector run). The atomic snapshot swap must
// not race WithContext readers.
func Test_AddContextExtractor_Concurrent(t *testing.T) {
	resetExtractorStack()
	defer resetExtractorStack()

	root := newLoggerWithRecords(make(chan *Record, 1024))
	defer root.Close()

	var wg sync.WaitGroup
	// writers: keep registering extractors
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			AddContextExtractor(func(ctx context.Context) map[string]interface{} {
				return map[string]interface{}{string(rune('A' + n)): n}
			})
		}(i)
	}
	// readers: keep building WithContext children (reads the snapshot)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = root.WithContext(context.Background())
			}
		}()
	}
	wg.Wait()
}

// Test_SetContextExtractor_OverridesStack confirms a per-logger extractor set
// via SetContextExtractor REPLACES the global stack for that logger (runs ONLY
// the per-logger extractor, not the stack).
func Test_SetContextExtractor_OverridesStack(t *testing.T) {
	resetExtractorStack()
	defer resetExtractorStack()

	AddContextExtractor(func(ctx context.Context) map[string]interface{} {
		return map[string]interface{}{"stack": "should-not-appear"}
	})

	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	root.SetContextExtractor(func(ctx context.Context) map[string]interface{} {
		return map[string]interface{}{"only": "me"}
	})

	child := root.WithContext(context.Background())
	got := map[string]interface{}{}
	for _, f := range child.fields {
		got[f.key] = f.value()
	}
	if _, ok := got["stack"]; ok {
		t.Errorf("per-logger extractor did not override global stack: %v", got)
	}
	if got["only"] != "me" {
		t.Errorf("per-logger extractor did not run: %v", got)
	}
}

// Test_RequestIDMiddleware_InboundHeader confirms the middleware reads the
// request id from the inbound header, binds a logger carrying it, and the
// handler sees it via FromContext + RequestIDFromContext.
func Test_RequestIDMiddleware_InboundHeader(t *testing.T) {
	var (
		gotRID    string
		gotFields map[string]interface{}
	)
	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRID = RequestIDFromContext(r.Context())
		lg := FromContext(r.Context())
		gotFields = map[string]interface{}{}
		for _, f := range lg.fields {
			gotFields[f.key] = f.value()
		}
	}), RequestIDMiddlewareOpts{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "inbound-rid-123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if gotRID != "inbound-rid-123" {
		t.Errorf("RequestIDFromContext=%q want inbound-rid-123", gotRID)
	}
	if gotFields["request_id"] != "inbound-rid-123" {
		t.Errorf("bound logger missing request_id field: %v", gotFields)
	}
}

// Test_RequestIDMiddleware_Generates confirms the middleware generates an id
// when the header is absent, using the configured Generator.
func Test_RequestIDMiddleware_Generates(t *testing.T) {
	var gotRID string
	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRID = RequestIDFromContext(r.Context())
	}), RequestIDMiddlewareOpts{Generator: func() string { return "gen-456" }})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if gotRID != "gen-456" {
		t.Errorf("generated rid=%q want gen-456", gotRID)
	}
}

// Test_RequestIDMiddleware_CustomHeaderAndField confirms Header/FieldName opts.
func Test_RequestIDMiddleware_CustomHeaderAndField(t *testing.T) {
	var gotFields map[string]interface{}
	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lg := FromContext(r.Context())
		gotFields = map[string]interface{}{}
		for _, f := range lg.fields {
			gotFields[f.key] = f.value()
		}
	}), RequestIDMiddlewareOpts{Header: "X-Correlation-Id", FieldName: "correlation_id"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Correlation-Id", "corr-789")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if gotFields["correlation_id"] != "corr-789" {
		t.Errorf("custom field/header wrong: %v", gotFields)
	}
}

// Test_RequestIDMiddleware_DefaultGenerator confirms the default generator
// produces a non-empty, unique-ish id.
func Test_RequestIDMiddleware_DefaultGenerator(t *testing.T) {
	var gotRID string
	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRID = RequestIDFromContext(r.Context())
	}), RequestIDMiddlewareOpts{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if gotRID == "" || !strings.HasPrefix(gotRID, "req-") {
		t.Errorf("default rid=%q invalid", gotRID)
	}
}

// Test_RequestIDMiddlewareHandler_Adapter confirms the chi/gin-style adapter
// returns a working middleware.
func Test_RequestIDMiddlewareHandler_Adapter(t *testing.T) {
	mw := RequestIDMiddlewareHandler(RequestIDMiddlewareOpts{Generator: func() string { return "x" }})
	var gotRID string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRID = RequestIDFromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotRID != "x" {
		t.Errorf("adapter rid=%q want x", gotRID)
	}
}

// Test_DefaultContextExtractor_ExtendedKeys confirms the default extractor now
// probes the extended key set (user_id, tenant_id, etc.), not just trace ids.
func Test_DefaultContextExtractor_ExtendedKeys(t *testing.T) {
	resetExtractorStack()
	defer resetExtractorStack()

	ctx := context.Background()
	ctx = context.WithValue(ctx, "user_id", 42)
	ctx = context.WithValue(ctx, "tenant_id", "acme")
	ctx = context.WithValue(ctx, "trace_id", "t-1")

	m := runContextExtractors(ctx)
	if m["user_id"] != 42 {
		t.Errorf("user_id not extracted: %v", m)
	}
	if m["tenant_id"] != "acme" {
		t.Errorf("tenant_id not extracted: %v", m)
	}
	if m["trace_id"] != "t-1" {
		t.Errorf("trace_id not extracted: %v", m)
	}
}

// Test_RequestIDMiddleware_EndToEnd_LogLine confirms an end-to-end request: the
// middleware binds a request_id-carrying logger, the handler logs through
// FromContext, and the resulting record's fields contain request_id.
func Test_RequestIDMiddleware_EndToEnd_LogLine(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	root.SetLevel(DEBUG)
	cw := &captureWriter{}
	root.Register(cw)

	// swap the singleton so RequestIDMiddleware (which uses defaultLogger()) and
	// FromContext share our instrumented root.
	old := loggerDefault.Swap(root)
	defer func() { loggerDefault.Store(old) }()

	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		FromContext(r.Context()).Info("handled request")
	}), RequestIDMiddlewareOpts{Generator: func() string { return "rid-e2e" }})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		// spin until the bootstrap goroutine has written the record
	}
	if cw.Len() == 0 {
		t.Fatal("record never reached writer")
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()
	var foundReqID bool
	for _, f := range r.fields {
		if f.key == "request_id" && f.value() == "rid-e2e" {
			foundReqID = true
		}
	}
	if !foundReqID {
		t.Fatalf("record missing request_id=rid-e2e; fields=%v", r.fields)
	}
}
