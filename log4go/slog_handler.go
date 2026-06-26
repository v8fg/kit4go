package log4go

import (
	"context"
	"log/slog"
	"path"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"
)

// SlogHandler adapts a log4go Logger to the standard library log/slog.Handler
// interface, so code using slog (net/http, third-party libraries, the Go runtime)
// routes through the log4go pipeline — its writers, overflow protection,
// alerting and metrics. Install it as the default slog handler:
//
//	slog.SetDefault(slog.New(log4go.NewSlogHandler(log4go.NewLogger())))
//
// Records flow into the logger's records channel like any other log4go call,
// carrying the same base fields and pre-serialized format bytes.
type SlogHandler struct {
	logger *Logger
	attrs  []field // attrs accumulated via WithAttrs
	group  string  // group prefix, flattened as "group.key"
}

// NewSlogHandler returns a slog.Handler forwarding to logger (nil -> the package
// singleton).
func NewSlogHandler(logger *Logger) *SlogHandler {
	if logger == nil {
		logger = defaultLogger()
	}
	return &SlogHandler{logger: logger}
}

// Enabled reports whether the logger would emit at the given slog level.
func (h *SlogHandler) Enabled(_ context.Context, sl slog.Level) bool {
	return int32(slogToLog4goLevel(sl)) <= h.logger.level.Load()
}

// Handle converts a slog.Record into a log4go Record and delivers it.
func (h *SlogHandler) Handle(_ context.Context, sr slog.Record) error {
	lvl := slogToLog4goLevel(sr.Level)
	if int32(lvl) > h.logger.level.Load() {
		return nil
	}
	extra := make([]field, 0, len(h.attrs)+sr.NumAttrs())
	extra = append(extra, h.attrs...)
	sr.Attrs(func(a slog.Attr) bool {
		extra = append(extra, slogAttrToField(h.group, a))
		return true
	})

	now := sr.Time
	if now.IsZero() {
		now = time.Now()
	}
	layout := defaultLayout
	if lp := h.logger.layout.Load(); lp != nil {
		layout = *lp
	}

	r := recordPool.Get().(*Record)
	r.msg = sr.Message
	r.file = slogSource(sr)
	r.time = now.Format(layout)
	r.level = lvl
	r.unixNano = now.UnixNano()
	r.seq = atomic.AddUint64(&globalSeq, 1)
	r.fields = mergeLoggerFields(h.logger, extra)

	switch LogFormat(h.logger.format.Load()) {
	case FormatJSON:
		r.formattedBytes = r.JSON()
	case FormatLogfmt:
		r.formattedBytes = r.Logfmt()
	}

	h.logger.records <- r
	return nil
}

// WithAttrs returns a child handler carrying the additional attrs.
func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := *h
	fs := make([]field, len(attrs))
	for i, a := range attrs {
		fs[i] = slogAttrToField(h.group, a)
	}
	nh.attrs = append(append([]field(nil), h.attrs...), fs...)
	return &nh
}

// WithGroup returns a child handler with a group prefix (flattened as group.key).
func (h *SlogHandler) WithGroup(name string) slog.Handler {
	nh := *h
	if h.group == "" {
		nh.group = name
	} else {
		nh.group = h.group + "." + name
	}
	return &nh
}

// slogToLog4goLevel maps slog.Level (DEBUG=-4, INFO=0, WARN=4, ERROR=8) to the
// log4go level (lower int = more severe). slog levels below LevelDebug (< -4,
// e.g. LevelTrace=-8 if a library defines one) map to TRACE.
func slogToLog4goLevel(sl slog.Level) int {
	switch {
	case sl >= slog.LevelError:
		return ERROR
	case sl >= slog.LevelWarn:
		return WARNING
	case sl >= slog.LevelInfo:
		return INFO
	case sl >= slog.LevelDebug:
		return DEBUG
	default:
		return TRACE
	}
}

// slogAttrToField converts a resolved slog.Attr into a typed field, applying the
// group prefix. Group values are flattened recursively with a dotted prefix.
func slogAttrToField(group string, a slog.Attr) field {
	v := a.Value.Resolve()
	key := a.Key
	if group != "" {
		key = group + "." + key
	}
	switch v.Kind() {
	case slog.KindString:
		return strField(key, v.String())
	case slog.KindInt64:
		return int64Field(key, v.Int64())
	case slog.KindFloat64:
		return floatField(key, v.Float64())
	case slog.KindBool:
		return boolField(key, v.Bool())
	case slog.KindDuration:
		return durField(key, v.Duration())
	case slog.KindTime:
		return timeField(key, v.Time())
	case slog.KindGroup:
		// Group attrs are flattened; represented as the JSON of the group object
		// via kindAny so nested structure is preserved on output.
		return anyField(key, v.Any())
	default:
		return anyField(key, v.Any())
	}
}

// slogSource renders file:line from a slog record's program counter, or "".
func slogSource(sr slog.Record) string {
	if sr.PC == 0 {
		return ""
	}
	frames := runtime.CallersFrames([]uintptr{sr.PC})
	f, _ := frames.Next()
	if f.File == "" {
		return ""
	}
	return path.Base(f.File) + ":" + strconv.Itoa(f.Line)
}

// mergeLoggerFields combines base fields + logger fields + extra (call-site)
// fields. Priority: extra > logger > base (append order; later overrides on
// duplicate-key JSON/logfmt emit). Returns nil when there is nothing.
func mergeLoggerFields(l *Logger, extra []field) []field {
	bf := l.baseFields.v.Load()
	need := len(l.fields) + len(extra)
	if bf != nil {
		need += len(*bf)
	}
	if need == 0 {
		return nil
	}
	out := make([]field, 0, need)
	if bf != nil {
		out = append(out, *bf...)
	}
	out = append(out, l.fields...)
	out = append(out, extra...)
	return out
}
