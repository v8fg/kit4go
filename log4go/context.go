package log4go

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ctxKey is an unexported context key type so log4go's stored values never
// collide with caller keys (the standard-library idiom for private context
// values). Different packages using the same string key would clash; a private
// type never does. We reuse one type and distinct zero-value vs "request_id"
// instances via named vars.
type ctxKey struct{ name string }

// loggerCtxKey is the single key under which a *Logger is stashed in a context
// (by Logger.IntoContext / WithLogger).
var loggerCtxKey = ctxKey{name: "log4go.logger"}

// RequestIDContextKey is the context key under which RequestIDMiddleware stores
// the resolved request id, so handlers can read it directly via
// ctx.Value(log4go.RequestIDContextKey) (in addition to it being attached to
// the bound Logger's fields as "request_id").
var RequestIDContextKey = ctxKey{name: "log4go.request_id"}

// IntoContext returns a new context.Context carrying this logger, so a later
// FromContext(r.Context()) in a deeper call (e.g. an HTTP handler) recovers the
// SAME logger — including its With/WithField/WithFields structured fields. This
// is the zerolog-style "bind logger to request context" pattern: a middleware
// builds a logger with request_id/trace_id, stores it on the request context,
// and handlers emit through log4go.FromContext(ctx) carrying those fields
// automatically.
//
// The stored logger is used verbatim by FromContext (not a child), so fields
// added to a separate logger AFTER IntoContext are not visible to FromContext
// callers — build the full request-scoped logger first, then store it.
func (l *Logger) IntoContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, loggerCtxKey, l)
}

// FromContext recovers a Logger previously stashed in ctx by IntoContext (or
// WithLogger). If none is present it returns the package singleton — so callers
// can always write log4go.FromContext(ctx).Info(...) without a nil check, and a
// request that skipped the middleware still logs through the default logger.
//
// This is the read side of the zerolog-style context binding; pair it with a
// middleware that does:
//
//	logger := log4go.With("request_id", rid)
//	next.ServeHTTP(w, r.WithContext(logger.IntoContext(r.Context())))
//
// and in handlers:
//
//	log4go.FromContext(r.Context()).Info("handled")
func FromContext(ctx context.Context) *Logger {
	if ctx == nil {
		return defaultLogger()
	}
	if l, ok := ctx.Value(loggerCtxKey).(*Logger); ok && l != nil {
		return l
	}
	return defaultLogger()
}

// FromContext recovers the context-bound Logger if present, else returns the
// receiver. Useful when a caller already holds a fallback logger and prefers it
// over the package singleton for unbound contexts. The package-level FromContext
// is the more common entry point.
func (l *Logger) FromContext(ctx context.Context) *Logger {
	if ctx == nil {
		return l
	}
	if bound, ok := ctx.Value(loggerCtxKey).(*Logger); ok && bound != nil {
		return bound
	}
	return l
}

// WithLogger returns ctx with lg stored on it (the functional equivalent of
// lg.IntoContext(ctx)), named to read naturally at the start of a middleware:
//
//	ctx = log4go.WithLogger(r.Context(), logger)
//
// nil lg falls back to the package singleton.
func WithLogger(ctx context.Context, lg *Logger) context.Context {
	if lg == nil {
		lg = defaultLogger()
	}
	return lg.IntoContext(ctx)
}

// --- multi-extractor registry ---------------------------------------------

// ContextExtractor derives structured fields from a context.Context. Return an
// empty/nil map to attach nothing. Extractors are stacked via
// AddContextExtractor: every registered extractor runs (in registration order)
// and its results are merged into the child Logger's fields when WithContext is
// called. The default stack already covers common trace-id / request-id /
// user-id keys; callers add domain-specific extractors (route, tenant, otel
// span/baggage) without touching the library.
type ContextExtractor = func(context.Context) map[string]interface{}

// extractorSnapshot is an immutable snapshot of the extractor stack, stored in
// an atomic.Pointer so the WithContext hot path reads it lock-free. Replaced
// wholesale by AddContextExtractor (copy-on-write).
type extractorSnapshot struct {
	fns []ContextExtractor
}

var (
	extractorMu          sync.RWMutex
	extractorSnapshotRef atomic.Pointer[extractorSnapshot]
)

// init seeds the default extractor stack with the built-in trace/request-id
// probe. Runs at package init (before main) so AddContextExtractor calls in
// early setup append after it.
func init() {
	resetExtractorStackLocked()
}

// resetExtractorStack is test-only: restores the built-in default extractor and
// drops any AddContextExtractor additions.
func resetExtractorStack() {
	extractorMu.Lock()
	defer extractorMu.Unlock()
	resetExtractorStackLocked()
}

func resetExtractorStackLocked() {
	stack := []ContextExtractor{defaultContextExtractor}
	extractorSnapshotRef.Store(&extractorSnapshot{fns: stack})
}

// AddContextExtractor appends a context extractor to the global stack. Every
// extractor on the stack runs when WithContext builds a child logger; results
// are merged in order, so later extractors can override earlier keys. Use this
// to attach domain-specific context fields (tenant_id, route, otel trace) on
// every WithContext call without wrapping the logger manually.
//
// Example (OpenTelemetry trace, zero hard otel dependency — the extractor is
// only registered if the caller imports otel and passes the function in):
//
//	log4go.AddContextExtractor(func(ctx context.Context) map[string]interface{} {
//	    span := trace.SpanFromContext(ctx)
//	    sc := span.SpanContext()
//	    if !sc.IsValid() {
//	        return nil
//	    }
//	    return map[string]interface{}{
//	        "trace_id": sc.TraceID().String(),
//	        "span_id":  sc.SpanID().String(),
//	    }
//	})
func AddContextExtractor(fn ContextExtractor) {
	if fn == nil {
		return
	}
	extractorMu.Lock()
	cur := extractorSnapshotRef.Load()
	next := make([]ContextExtractor, 0, len(cur.fns)+1)
	next = append(next, cur.fns...)
	next = append(next, fn)
	extractorSnapshotRef.Store(&extractorSnapshot{fns: next})
	extractorMu.Unlock()
}

// runContextExtractors runs every registered extractor against ctx and merges
// their maps. The result is nil if no extractor produced any field (so
// attachContextFields adds nothing and the logger is unchanged). Later
// extractors override earlier ones on key collision (last-writer-wins).
func runContextExtractors(ctx context.Context) map[string]interface{} {
	if ctx == nil {
		return nil
	}
	snap := extractorSnapshotRef.Load()
	if snap == nil {
		return nil
	}
	var merged map[string]interface{}
	for _, fn := range snap.fns {
		if fn == nil {
			continue
		}
		m := fn(ctx)
		if len(m) == 0 {
			continue
		}
		if merged == nil {
			merged = make(map[string]interface{}, len(m))
		}
		for k, v := range m {
			merged[k] = v
		}
	}
	return merged
}

// --- request-id middleware -------------------------------------------------

// RequestIDHeader is the default HTTP header consulted by RequestIDMiddleware
// for an inbound request id. Override per-middleware via RequestIDMiddlewareOpts.
const RequestIDHeader = "X-Request-Id"

// RequestIDMiddlewareOpts configures RequestIDMiddleware.
type RequestIDMiddlewareOpts struct {
	// Header is the inbound header to read the request id from (default
	// "X-Request-Id"). If absent, the middleware generates a new id.
	Header string
	// Generator returns a fresh request id when none is inbound. If nil, a
	// default process-unique-ish id is used (timestamp + counter; good enough
	// for correlation within a process — supply a UUID generator for
	// cross-process uniqueness).
	Generator func() string
	// FieldName is the structured-field name the bound Logger attaches for the
	// request id (default "request_id").
	FieldName string
}

// RequestIDMiddleware returns an http.Handler that resolves a request id (from
// the inbound header, or generated), binds a child Logger carrying it as a
// structured field onto the request context, and calls next. Handlers then log
// through log4go.FromContext(r.Context()) and every line carries the request id
// automatically — the zerolog/logrus pattern for per-request log correlation.
//
// The request id is ALSO stored under RequestIDContextKey for direct access.
//
//	r.Use(log4go.RequestIDMiddlewareHandler(log4go.RequestIDMiddlewareOpts{
//	    Generator: uuid.NewString,
//	}))
func RequestIDMiddleware(next http.Handler, opts RequestIDMiddlewareOpts) http.Handler {
	header := opts.Header
	if header == "" {
		header = RequestIDHeader
	}
	field := opts.FieldName
	if field == "" {
		field = "request_id"
	}
	gen := opts.Generator
	if gen == nil {
		gen = defaultRequestID
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get(header)
		if rid == "" {
			rid = gen()
		}
		lg := defaultLogger().With(field, rid)
		ctx := r.Context()
		ctx = context.WithValue(ctx, RequestIDContextKey, rid)
		ctx = lg.IntoContext(ctx)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDMiddlewareHandler is a chi/gin-style adapter that returns the
// middleware as a function, for routers expecting func(http.Handler) http.Handler.
func RequestIDMiddlewareHandler(opts RequestIDMiddlewareOpts) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return RequestIDMiddleware(next, opts)
	}
}

// RequestIDFromContext returns the request id stored by RequestIDMiddleware, or
// "" if the context did not pass through the middleware. Pair with
// log4go.FromContext to get both the id and the bound logger.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(RequestIDContextKey).(string); ok {
		return v
	}
	return ""
}

// requestIDCounter is the process-local sequence for defaultRequestID, making
// ids unique within a process even when generated in the same nanosecond.
var requestIDCounter atomic.Uint64

// defaultRequestID is the fallback id generator when none is configured. It is
// NOT cryptographically unique — it combines a Unix-nano timestamp with a
// process-local counter so ids are unique within a process and roughly sortable.
// For cross-process uniqueness supply a UUID generator via RequestIDMiddlewareOpts.
func defaultRequestID() string {
	n := requestIDCounter.Add(1)
	return fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), n)
}
