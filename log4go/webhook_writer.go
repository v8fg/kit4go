package log4go

import (
	"fmt"
	"sync/atomic"
)

// WebhookWriter routes records to an AlertSink — typically a WebhookAlertSink
// (lark/dingtalk/wechat) — when they pass a level threshold and an optional
// filter. It is a plain Writer: register it alongside Kafka/Net/Console and
// each filters by its own level, so ONE logger can fan out to many destinations
// at different severities:
//
//	kafka  (INFO+)  → full volume to Kafka→ES
//	net    (WARN+)  → hot lines to a TCP collector
//	webhook(ERROR+) → only severe lines to lark/dingtalk
//
// The sink owns async send / retry / rate-limit, so a slow or down webhook
// never blocks the application.
//
// Three composable trigger modes:
//   - Level: always on — only records at/above the writer's level are forwarded.
//   - Filter: an optional predicate on the record (keyword / field match), for
//     "only alert on specific errors".
//   - Gate: an optional *RateAlerter that forwards only past a threshold within
//     a sliding window (storm suppression / threshold-summary alerting).
//
// Close() closes the underlying sink (stops its async daemon). It is picked up
// automatically by log4go.Close() (Logger.Close invokes io.Closer on writers).
type WebhookWriter struct {
	sink          AlertSink
	level         int
	filter        Filter
	formatter     RecordWebhookFormatter
	rateFormatter RateWebhookFormatter
	gate          *RateAlerter

	sent    uint64
	skipped uint64
	onEvent atomic.Pointer[func(name string, delta int64)]
}

// WebhookContext carries writer-level state into a RateWebhookFormatter —
// currently the RateAlerter in-window count, so a threshold-summary alert can
// report "N errors in the last minute". RateCount is 0 when no gate is set.
type WebhookContext struct {
	RateCount int // gate.Count() snapshot at format time
}

// RateWebhookFormatter is the context-aware formatter variant: it receives the
// writer context (e.g. the rate count) in addition to the record. Set it via
// WebhookWriterOptions.RateFormatter when a Gate is configured and you want the
// in-window count in the payload. When RateFormatter is nil the plain
// Formatter is used instead.
type RateWebhookFormatter func(r *Record, ctx WebhookContext) (kind, text string)

// RecordWebhookFormatter renders a record into the (kind, text) pair passed to
// AlertSink.Send. kind is a short category (e.g. "ERROR", "payment"); text is
// the human-readable body the OA webhook will display.
type RecordWebhookFormatter func(r *Record) (kind, text string)

// WebhookWriterOptions configures a WebhookWriter.
type WebhookWriterOptions struct {
	// Level is a severity name ("emergency"…"debug", case-insensitive); records
	// at/above it are forwarded. Empty -> ERROR.
	Level string
	// Filter is an optional predicate; only records it returns true for are
	// forwarded (after the level check). nil disables filtering. Use the
	// MatchField / MatchKeyword / AllOf ... constructors for common cases.
	Filter Filter
	// Formatter renders the forwarded record. nil -> DefaultWebhookFormatter.
	// Ignored when RateFormatter is set.
	Formatter RecordWebhookFormatter
	// RateFormatter is the context-aware formatter used (in preference to
	// Formatter) when set — typically to embed the RateAlerter in-window count
	// for a threshold-summary payload. nil falls back to Formatter.
	RateFormatter RateWebhookFormatter
	// Gate is an optional RateAlerter threshold gate. nil disables gating.
	Gate *RateAlerter
}

// NewWebhookWriter wraps sink as a Writer with level/filter/gate routing. A nil
// sink falls back to LogAlertSink (standard logger), so the writer is always
// usable for testing without a real webhook.
func NewWebhookWriter(sink AlertSink, opts WebhookWriterOptions) *WebhookWriter {
	if sink == nil {
		sink = LogAlertSink{}
	}
	lvl := ERROR
	if opts.Level != "" {
		lvl = getLevelDefault(opts.Level, ERROR, "webhook")
	}
	f := opts.Formatter
	if f == nil {
		f = DefaultWebhookFormatter
	}
	return &WebhookWriter{
		sink:          sink,
		level:         lvl,
		filter:        opts.Filter,
		formatter:     f,
		rateFormatter: opts.RateFormatter,
		gate:          opts.Gate,
	}
}

// Init is a no-op: the sink owns its own async daemon (started at construction).
func (w *WebhookWriter) Init() error { return nil }

// Write applies level → filter → gate, then forwards surviving records to the
// sink. It never returns an error — a failing webhook is the sink's concern and
// is handled (retry/drop) inside it, so the log path is never disturbed.
func (w *WebhookWriter) Write(r *Record) error {
	if r.level > w.level { // below threshold
		w.bump(false)
		return nil
	}
	if w.filter != nil && !w.filter(r) { // predicate miss
		w.bump(false)
		return nil
	}
	if w.gate != nil && !w.gate.Allow() { // under threshold / cooling down
		w.bump(false)
		return nil
	}
	kind, text := w.format(r)
	w.sink.Send(recordAlertLevel(r.level), kind, text)
	w.bump(true)
	return nil
}

// format renders the record, using RateFormatter (with the gate's in-window
// count) when configured, else the plain Formatter.
func (w *WebhookWriter) format(r *Record) (string, string) {
	if w.rateFormatter == nil {
		return w.formatter(r)
	}
	ctx := WebhookContext{}
	if w.gate != nil {
		ctx.RateCount = w.gate.Count()
	}
	return w.rateFormatter(r, ctx)
}

// bump tallies sent/skipped and fires the onEvent hook (if any).
func (w *WebhookWriter) bump(sent bool) {
	if sent {
		atomic.AddUint64(&w.sent, 1)
		w.fire("sent", 1)
		return
	}
	atomic.AddUint64(&w.skipped, 1)
}

// Close closes the underlying sink (stops its async daemon). Idempotent via the
// sink's own Close.
func (w *WebhookWriter) Close() error {
	if w.sink != nil {
		return w.sink.Close()
	}
	return nil
}

// SetFilter installs or replaces the record filter.
func (w *WebhookWriter) SetFilter(f Filter) { w.filter = f }

// SetRateFormatter installs the context-aware formatter (used in preference to
// the plain Formatter when set), so the RateAlerter in-window count can be
// embedded in the payload. Pass nil to revert to the plain Formatter.
func (w *WebhookWriter) SetRateFormatter(f RateWebhookFormatter) { w.rateFormatter = f }

// SetGate installs or replaces the threshold gate.
func (w *WebhookWriter) SetGate(g *RateAlerter) { w.gate = g }

// SetOnEvent installs a real-time metric hook invoked on each sent/skipped
// decision with the metric name ("sent"/"skipped") and the delta (always 1).
func (w *WebhookWriter) SetOnEvent(fn func(name string, delta int64)) {
	if fn == nil {
		w.onEvent.Store(nil)
		return
	}
	w.onEvent.Store(&fn)
}

func (w *WebhookWriter) fire(name string, delta int64) {
	if p := w.onEvent.Load(); p != nil {
		(*p)(name, delta)
	}
}

// WebhookWriterMetrics is a point-in-time counter snapshot.
type WebhookWriterMetrics struct {
	Sent    uint64 // records forwarded to the sink
	Skipped uint64 // records dropped by level/filter/gate
}

// Metrics returns the current sent/skipped counters.
func (w *WebhookWriter) Metrics() WebhookWriterMetrics {
	return WebhookWriterMetrics{
		Sent:    atomic.LoadUint64(&w.sent),
		Skipped: atomic.LoadUint64(&w.skipped),
	}
}

// recordAlertLevel maps a record level to an alert severity: ERROR and above ->
// AlertError, WARNING -> AlertWarn, NOTICE/INFO/DEBUG -> AlertInfo.
func recordAlertLevel(level int) AlertLevel {
	if level <= ERROR {
		return AlertError
	}
	if level <= WARNING {
		return AlertWarn
	}
	return AlertInfo
}

// DefaultWebhookFormatter renders the canonical line "<time> [LEVEL] <<file>>
// <msg>" plus a trailing JSON object of structured fields (when any) as the
// alert text; kind is the level name.
func DefaultWebhookFormatter(r *Record) (string, string) {
	line := r.time + " [" + LevelFlags[r.level] + "]"
	if r.file != "" {
		line += " <" + r.file + ">"
	}
	line += " " + r.msg
	if fj := r.FieldsJSON(); fj != "" {
		line += " " + fj
	}
	return LevelFlags[r.level], line
}

// DefaultRateWebhookFormatter is the context-aware default: it prepends the
// in-window count from the gate (e.g. "[12 in window]") to the default line when
// the count is non-zero, so a threshold-summary alert reports how many events
// accumulated. When no gate is set (RateCount == 0) it renders the plain line.
func DefaultRateWebhookFormatter(r *Record, ctx WebhookContext) (string, string) {
	kind, text := DefaultWebhookFormatter(r)
	if ctx.RateCount > 0 {
		text = fmt.Sprintf("[%d in window] %s", ctx.RateCount, text)
	}
	return kind, text
}
