package log4go

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LevelFlag log level flags
const (
	LevelFlagEmergency = "EMERGENCY"
	LevelFlagAlert     = "ALERT"
	LevelFlagCritical  = "CRITICAL"
	LevelFlagError     = "ERROR"
	LevelFlagWarning   = "WARNING" // compatible WARN
	LevelFlagWarn      = "WARN"
	LevelFlagNotice    = "NOTICE"
	LevelFlagInfo      = "INFO"
	LevelFlagDebug     = "DEBUG"
)

// RFC5424 log message levels.
// ref: https://tools.ietf.org/html/draft-ietf-syslog-protocol-23
const (
	EMERGENCY = iota // Emergency: system is unusable
	ALERT            // Alert: action must be taken immediately
	CRITICAL         // Critical: critical conditions
	ERROR            // Error: error conditions
	WARNING          // Warning: warning conditions
	NOTICE           // Notice: normal but significant condition
	INFO             // Informational: informational messages
	DEBUG            // Debug: debug-level messages
)

const (
	// default size or min size for record channel
	recordChannelSizeDefault = uint(4096)
	// default time layout (local time, no timezone — for human reading)
	defaultLayout = "2006/01/02 15:04:05"
	// timestampLayout for JSON/Kafka: RFC3339Nano with auto timezone.
	// Z07:00 renders +0800 in CN, -05:00 in US-East, Z for UTC.
	// .000000 gives microsecond precision for strict ordering.
	timestampLayout = "2006-01-02T15:04:05.000000Z07:00"
)

// jsonMarshal is the JSON marshal entry point used everywhere log4go serializes
// JSON (Record.JSON, FieldsJSON, KafKaWriter payload, deliverRecordToWriter).
// It defaults to jsonMarshalEncode (the codec-aware function — goccy by
// default, switchable via SetJSONCodec). Tests override it to inject a failing
// encoder for the marshal-error paths. NOTE: test overrides bypass the codec
// selection, which is fine — they only exercise the error branch.
var jsonMarshal = jsonMarshalEncode

// LevelFlags level Flags set
var (
	LevelFlags = []string{
		LevelFlagEmergency,
		LevelFlagAlert,
		LevelFlagCritical,
		LevelFlagError,
		LevelFlagWarning,
		LevelFlagNotice,
		LevelFlagInfo,
		LevelFlagDebug,
	}
	DefaultLayout = defaultLayout
)

// default logger
var (
	// loggerDefault is the package singleton. It is stored in an atomic.Pointer
	// so Close() can reset it to nil safely under concurrent access; the next
	// global call (NewLogger/Register/Debug/...) then rebuilds it. Without this,
	// Close() leaves loggerDefault pointing at a Logger whose bootstrap goroutine
	// has exited and whose records channel is closed — a second SetupLog or
	// Register would deliver records to a dead channel and orphan writers' daemon
	// goroutines.
	loggerDefault     atomic.Pointer[Logger]
	recordPool        *sync.Pool
	bufPool           *sync.Pool
	recordChannelSize = recordChannelSizeDefault // log chan size
)

// newDefaultLoggerInstance builds a fresh Logger configured the way the package
// singleton is configured in init() and after a Close() (flush 500ms, rotate
// 10s, default-size records channel). It does NOT publish the result; callers
// are responsible for storing it in loggerDefault if needed.
func newDefaultLoggerInstance() *Logger {
	records := make(chan *Record, recordChannelSize)
	l := newLoggerWithRecords(records)
	l.flushTimer = time.Millisecond * 500
	l.rotateTimer = time.Second * 10
	return l
}

// LogFormat selects the on-the-wire record serialization. The default is
// FormatText (the canonical "<time> [<LEVEL>] <<file>> <msg>\n" line plus any
// structured fields). FormatJSON emits a single JSON object per record:
//
//	{"time":"2026-06-25T15:04:05.000+0800","level":"INFO","msg":"...",
//	 "file":"svc.go:42","fields":{"trace_id":"abc","user":7}}
//
// The format is decided once per record in deliverRecordToWriter and cached on
// r.jsonBytes, so every registered writer (Console/File/Net/IO) outputs the
// pre-serialized bytes without re-serializing. KafKaWriter already emits its
// own JSON payload and is unaffected.
type LogFormat int32

const (
	// FormatText is the default human-readable line format (Record.String).
	FormatText LogFormat = iota
	// FormatJSON emits one JSON object per record (Record.JSON), suitable for
	// machine ingestion / log shippers like Fluentd/Filebeat.
	FormatJSON
)

// String returns the lowercase config name ("text"/"json") used by LogConfig.
func (f LogFormat) String() string {
	if f == FormatJSON {
		return "json"
	}
	return "text"
}

// ParseLogLogFormat parses a config string ("text"/"json", case-insensitive)
// into a LogFormat. Unknown values fall back to FormatText with a log line so
// misconfiguration is loud rather than silently changing output.
func ParseLogLogFormat(s string) LogFormat {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "json":
		return FormatJSON
	case "text", "":
		return FormatText
	}
	log.Printf("[log4go] unknown log format %q, defaulting to text", s)
	return FormatText
}

// defaultLogger returns the package singleton, rebuilding it (once) if it was
// reset by Close(). All package-level functions route through here so the
// singleton survives a Close+reuse cycle instead of leaving orphaned daemons.
func defaultLogger() *Logger {
	if l := loggerDefault.Load(); l != nil {
		return l
	}
	l := newDefaultLoggerInstance()
	if loggerDefault.CompareAndSwap(nil, l) {
		return l
	}
	// Lost the race; another goroutine published its instance. Close our extra
	// records channel so the bootstrap goroutine we spawned exits cleanly, then
	// return the winner.
	l.Close()
	return loggerDefault.Load()
}

// field is a structured key/value pair attached to a Logger (via With/WithField/
// WithFields) and copied onto every Record it emits. Writers that render
// structured output (Record.String for text, KafKaWriter.buildPayload for JSON)
// surface these fields. They are read-only after attachment; a Logger never
// mutates the slice of a shared parent, it always clones (see With).
type field struct {
	key string
	val interface{}
}

// Record log record
type Record struct {
	level int
	time  string
	file  string
	msg   string
	// unixNano is the wall-clock nanosecond timestamp (time.Now().UnixNano()),
	// populated in deliverRecordToWriter for strict ordering. Consumers (ELK/Grafana)
	// sort by seq then unixNano to reconstruct exact log order.
	unixNano int64
	// seq is a process-global monotonic counter (atomic). Together with unixNano
	// it forms a globally unique ordering key: two records never share the same seq.
	seq uint64
	// fields carries Logger-attached structured fields. nil for the common
	// no-With path (zero alloc overhead on the hot path). Record.String appends
	// them as a trailing JSON object; KafKaWriter.buildPayload hoists them into
	// the top-level JSON map.
	fields []field
	// jsonBytes is the pre-serialized JSON form of the record, populated once by
	// deliverRecordToWriter when the Logger's format is FormatJSON. Writers
	// (Console/File/Net/IO) that support the FormattedWriter fast path emit these
	// bytes verbatim to avoid re-serializing per writer. nil under FormatText.
	// Reset to nil by the bootstrap goroutine before returning to the pool.
	jsonBytes []byte
}

// globalSeq is the process-global monotonic record sequence counter.
// Incremented atomically in deliverRecordToWriter so that even under high
// concurrency (multi-goroutine, multi-core), every record gets a unique seq
// that reflects its logical creation order.
var globalSeq uint64

// FieldsJSON returns the record's structured fields marshaled to a JSON object
// (e.g. `{"trace_id":"abc","user":42}`). Returns "" when there are no fields,
// so callers can cheaply skip the append. Used by Record.String and
// KafKaWriter.buildPayload.
func (r *Record) FieldsJSON() string {
	if len(r.fields) == 0 {
		return ""
	}
	m := make(map[string]interface{}, len(r.fields))
	for _, f := range r.fields {
		m[f.key] = f.val
	}
	b, err := jsonMarshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// RecordJSON is the on-the-wire shape of a record when the Logger format is
// FormatJSON. Fields is nil when no structured fields are attached so the JSON
// omits the key (the omitempty makes the line shorter for the common case).
type RecordJSON struct {
	UnixNano int64                  `json:"unix_nano"`
	Seq      uint64                 `json:"seq"`
	Time     string                 `json:"time"`
	Level    string                 `json:"level"`
	Msg      string                 `json:"msg"`
	File     string                 `json:"file,omitempty"`
	Fields   map[string]interface{} `json:"fields,omitempty"`
}

// JSON serializes the record to a single JSON object terminated by a newline
// (one log line == one JSON document, the convention Fluentd/Filebeat expect).
// The time field uses the ISO-ish timestampLayout; level uses LevelFlags; fields
// is omitted entirely when there are none. On marshal error it falls back to the
// text String() form so a record is never silently lost.
//
// This is called once per record in deliverRecordToWriter (FormatJSON only) and
// cached on r.jsonBytes, so it is NOT on the multi-writer path — each record
// pays exactly one json.Marshal regardless of how many writers are registered.
func (r *Record) JSON() []byte {
	rj := RecordJSON{
		Time:  r.time,
		Level: LevelFlags[r.level],
		Msg:   r.msg,
		File:  r.file,
	}
	if len(r.fields) > 0 {
		rj.Fields = make(map[string]interface{}, len(r.fields))
		for _, f := range r.fields {
			rj.Fields[f.key] = f.val
		}
	}
	b, err := jsonMarshal(rj)
	if err != nil {
		// never lose a record: fall back to text. The caller (deliverRecordToWriter)
		// ignores the returned bytes in the error path; this is the last resort.
		return []byte(r.String())
	}
	return append(b, '\n')
}

// String renders the record line in the canonical format
// "<time> [<LEVEL>] <<file>> <msg>\n" (mirrors fmt.Sprintf("%s [%s] <%s> %s\n", ...)),
// followed by a trailing JSON object of structured fields when any are attached
// (via Logger.With/WithField/WithFields).
//
// It uses a strings.Builder instead of fmt.Sprintf to avoid fmt's reflection/
// interface-boxing overhead — this is on the hot FileWriter daemon path where it
// is called once per record under high write rates (~6.7x faster, 1 alloc vs 5).
// The structured-fields append only runs when len(r.fields) > 0, so loggers
// without With pay no extra cost.
func (r *Record) String() string {
	var b strings.Builder
	// Pre-size; +20 covers "#<seq> " + " [" + "] <" + "> " + "\n".
	b.Grow(len(r.time) + len(LevelFlags[r.level]) + len(r.file) + len(r.msg) + 20)
	// seq prefix for strict ordering in text format
	b.WriteString("#")
	b.WriteString(strconv.FormatUint(r.seq, 10))
	b.WriteByte(' ')
	b.WriteString(r.time)
	b.WriteString(" [")
	b.WriteString(LevelFlags[r.level])
	b.WriteString("] <")
	b.WriteString(r.file)
	b.WriteString("> ")
	b.WriteString(r.msg)
	if fj := r.FieldsJSON(); fj != "" {
		b.WriteByte(' ')
		b.WriteString(fj)
	}
	b.WriteByte('\n')
	return b.String()
}

// Writer record writer
type Writer interface {
	Init() error
	Write(*Record) error
}

// Flusher record flusher
type Flusher interface {
	Flush() error
}

// Rotater record rotater
type Rotater interface {
	Rotate() error
	SetPathPattern(string) error
}

// Logger logger define
type Logger struct {
	writers         atomic.Value // []Writer, copy-on-write for lock-free reads
	records         chan *Record
	recordsChanSize uint
	lastTime        atomic.Int64
	lastTimeStr     atomic.Pointer[string]

	flushTimer  time.Duration // timer to flush logger record to chan
	rotateTimer time.Duration // timer to rotate logger record for writer

	c chan bool

	layout         atomic.Pointer[string]
	level          atomic.Int32
	format         atomic.Int32       // LogFormat (FormatText/FormatJSON); atomic for lock-free hot-path read
	recordsByLevel *[DEBUG + 1]uint64 // per-level record counters (monitoring); pointer so child Loggers share the root's counters
	fullPath       atomic.Bool        // show full path, default only show file:line_number
	withFuncName   atomic.Bool        // show caller func name
	hasCaller      atomic.Bool        // capture caller (file:line); disable for max throughput

	// baseFields are global static fields registered once via SetBaseField(s)
	// and carried by EVERY record. Unlike With() (per-sub-logger), these are
	// the "environment" fields: hostname, server_ip, app_name, env, version.
	// Set once at startup, read on every deliverRecordToWriter (immutable
	// after set, stored in atomic.Pointer for lock-free hot-path read).
	baseFields atomic.Pointer[[]field]

	// fields carries structured key/value pairs attached via With/WithField/
	// WithFields. A child Logger always gets its OWN copy (see clone), so a
	// parent's slice is never mutated and is safe to read concurrently from the
	// deliverRecordToWriter hot path without locking. Immutable after the With
	// call that produced it.
	fields []field

	// sampler drops high-frequency records to prevent log storms. nil disables
	// sampling (the default). See WithSampling. Read on the deliverRecordToWriter
	// hot path; set once at construction and never mutated, so no lock needed.
	sampler *Sampler

	// ctxExtractor derives structured fields from a context.Context supplied
	// via WithContext. nil disables context extraction. Set once at construction.
	// See WithContext / SetContextExtractor.
	ctxExtractor func(context.Context) map[string]interface{}

	lock sync.RWMutex

	// hasSubSecond is set by SetLayout when the layout contains a fractional
	// seconds directive (".000", ".999", etc). When true, the time cache is
	// bypassed so every record gets a fresh time.Now().Format() call — otherwise
	// same-second records would share the same (stale) sub-second value.
	hasSubSecond atomic.Bool
}

// NewLogger create the logger
func NewLogger() *Logger {
	return defaultLogger()
}

// newLoggerWithRecords is useful for go test
func newLoggerWithRecords(records chan *Record) *Logger {
	l := new(Logger)
	l.writers.Store(make([]Writer, 0, 1)) // normal least has console writer
	if l.recordsChanSize == 0 {
		recordChannelSize = recordChannelSizeDefault
	}

	l.records = records
	l.c = make(chan bool, 1)
	l.recordsByLevel = new([DEBUG + 1]uint64)
	l.level.Store(int32(DEBUG))
	lp := DefaultLayout
	l.layout.Store(&lp)
	l.hasCaller.Store(true)

	go bootstrapLogWriter(l)

	return l
}

// Register register writer
// the writer should be register once for writers by kind
func (l *Logger) Register(w Writer) {
	if err := w.Init(); err != nil {
		panic(err)
	}

	// copy-on-write so the bootstrap goroutine can read writers lock-free.
	cur := l.writers.Load().([]Writer)
	next := make([]Writer, len(cur)+1)
	copy(next, cur)
	next[len(cur)] = w
	l.writers.Store(next)
}

// snapshotWriters returns the current writers slice for lock-free iteration
// by the bootstrap goroutine; Register replaces the slice copy-on-write so
// this view stays valid.
func (l *Logger) snapshotWriters() []Writer {
	return l.writers.Load().([]Writer)
}

// LoggerMetrics is a snapshot of per-level record counters for monitoring.
type LoggerMetrics struct {
	Records [DEBUG + 1]uint64 // indexed by level (EMERGENCY..DEBUG)
}

// Metrics returns per-level record counters of this logger for monitoring.
// Because child Loggers (from With/WithField/WithFields/WithSampling/WithContext)
// share the root's counter array, Metrics on the root reflects records emitted
// through any child — the typical monitoring setup reads the package singleton.
func (l *Logger) Metrics() LoggerMetrics {
	var m LoggerMetrics
	if l.recordsByLevel == nil {
		return m
	}
	for i := range m.Records {
		m.Records[i] = atomic.LoadUint64(&l.recordsByLevel[i])
	}
	return m
}

// Metrics returns the default logger's per-level record counters for monitoring.
func Metrics() LoggerMetrics { return defaultLogger().Metrics() }

// Close close logger
func (l *Logger) Close() {
	close(l.records)
	<-l.c

	for _, w := range l.snapshotWriters() {
		if f, ok := w.(Flusher); ok {
			if err := f.Flush(); err != nil {
				log.Println(err)
			}
		}
	}
}

// SetLayout set the logger time layout
func (l *Logger) SetLayout(layout string) {
	v := layout
	l.hasSubSecond.Store(strings.Contains(layout, ".0") || strings.Contains(layout, ".9"))
	l.layout.Store(&v)
}

// SetBaseField registers a global static field that EVERY subsequent log
// record will carry (merged into Record.fields). Unlike With (which returns
// a child Logger), base fields affect the singleton and all child Loggers.
// Use at startup for environment fields: hostname, server_ip, app_name, env.
func (l *Logger) SetBaseField(key string, val interface{}) {
	cur := l.baseFields.Load()
	var next []field
	if cur != nil {
		next = make([]field, len(*cur)+1)
		copy(next, *cur)
	} else {
		next = make([]field, 1)
	}
	next[len(next)-1] = field{key: key, val: val}
	l.baseFields.Store(&next)
}

// SetBaseFields is a batch version of SetBaseField.
func (l *Logger) SetBaseFields(m map[string]interface{}) {
	cur := l.baseFields.Load()
	next := make([]field, 0, len(m)+func() int { if cur != nil { return len(*cur) }; return 0 }())
	if cur != nil {
		next = append(next, *cur...)
	}
	for k, v := range m {
		next = append(next, field{key: k, val: v})
	}
	l.baseFields.Store(&next)
}

// SetBaseField registers a global static field on the default logger.
func SetBaseField(key string, val interface{}) { defaultLogger().SetBaseField(key, val) }

// SetBaseFields registers global static fields on the default logger.
func SetBaseFields(m map[string]interface{}) { defaultLogger().SetBaseFields(m) }

// SetLevel set the logger level
func (l *Logger) SetLevel(lvl int) {
	l.level.Store(int32(lvl))
}

// SetFormat selects the record serialization format (FormatText or FormatJSON).
// FormatText (the default) emits the human-readable line; FormatJSON emits one
// JSON object per record (see Record.JSON). The format is read once per record
// in deliverRecordToWriter and decides whether r.jsonBytes is pre-serialized.
// All registered writers honor the format via the FormattedWriter fast path
// (they emit r.jsonBytes when non-nil, else r.String()), so no per-writer change
// is needed.
func (l *Logger) SetFormat(f LogFormat) {
	l.format.Store(int32(f))
}

// Format returns the current serialization format.
func (l *Logger) Format() LogFormat {
	return LogFormat(l.format.Load())
}

// WithFullPath set the logger with full path
func (l *Logger) WithFullPath(show bool) {
	l.fullPath.Store(show)
}

// WithFuncName set the logger with func name
func (l *Logger) WithFuncName(show bool) {
	l.withFuncName.Store(show)
}

// WithCaller toggles runtime.Caller capture (file:line). Disable for maximum
// throughput when line info is not needed.
func (l *Logger) WithCaller(enable bool) {
	l.hasCaller.Store(enable)
}

// clone returns a child Logger sharing this logger's writers/records channel
// (so records still flow through the same bootstrap goroutine) but carrying its
// own copy of the immutable per-instance config: structured fields, sampler and
// context extractor. The child's atomic knobs (level/layout/hasCaller/...) start
// at the parent's current values and can be re-tuned on the child independently.
//
// The child does NOT spawn a new bootstrap goroutine — it shares l.records, so
// Close on the parent (or the child) drains both. This is deliberate: structured
// fields are a per-call-site concern, not a separate sink.
func (l *Logger) clone() *Logger {
	c := &Logger{
		records:         l.records,
		recordsChanSize: l.recordsChanSize,
		flushTimer:      l.flushTimer,
		rotateTimer:     l.rotateTimer,
		c:               l.c,
		fields:          l.fields, // shared read-only; With copies before appending
		sampler:         l.sampler,
		ctxExtractor:    l.ctxExtractor,
		recordsByLevel:  l.recordsByLevel, // shared pointer so children's emits count on the root
	}
	// copy current atomic knob values into the child
	c.level.Store(l.level.Load())
	c.format.Store(l.format.Load()) // child inherits the parent's JSON/text format
	if lp := l.layout.Load(); lp != nil {
		v := *lp
		c.layout.Store(&v)
	}
	c.fullPath.Store(l.fullPath.Load())
	c.withFuncName.Store(l.withFuncName.Load())
	c.hasCaller.Store(l.hasCaller.Load())
	// share the writers snapshot (copy-on-write on Register applies to parent;
	// child reads its own snapshot which is fine since Register on a child is
	// unusual but supported).
	c.writers.Store(l.writers.Load())
	return c
}

// With returns a child Logger that attaches a structured key/value pair to every
// record it emits. It is chainable: With("a", 1).With("b", 2) yields a logger
// carrying both fields. The parent logger is unaffected (each call clones the
// fields slice before appending, so a parent never sees a child's fields and
// concurrent loggers don't race).
//
// Fields surface in Record.String() (as a trailing JSON object) and in
// KafKaWriter.buildPayload (hoisted into the top-level JSON map). Loggers
// without With pay no fields cost: Record.String short-circuits on empty fields.
func (l *Logger) With(key string, val interface{}) *Logger {
	c := l.clone()
	// copy-on-write: build a new slice so the parent's slice stays immutable.
	nf := make([]field, len(l.fields), len(l.fields)+1)
	copy(nf, l.fields)
	nf = append(nf, field{key: key, val: val})
	c.fields = nf
	return c
}

// WithField is an alias for With(key, val) for ergonomic parity with
// logrus/zap-style APIs.
func (l *Logger) WithField(key string, val interface{}) *Logger {
	return l.With(key, val)
}

// WithFields returns a child Logger attaching every key/value pair in fields.
// This is equivalent to chaining With for each entry, but does it in one clone.
// The map is copied; later mutation of the input map does not affect the logger.
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	c := l.clone()
	nf := make([]field, 0, len(l.fields)+len(fields))
	nf = append(nf, l.fields...)
	for k, v := range fields {
		nf = append(nf, field{key: k, val: v})
	}
	c.fields = nf
	return c
}

// WithSampling returns a child Logger that applies sampling to prevent
// high-frequency log storms. The first `initial` records at each level are all
// emitted; after that, one record is emitted every `thereafter` records. Sampling
// runs on the deliverRecordToWriter hot path using an atomic counter per level,
// and sampled-out records are dropped BEFORE Metrics increment, so dropped
// records don't inflate the counters.
//
// initial/thereafter <= 0 disables sampling on the returned logger (a nil
// sampler is a no-op in deliverRecordToWriter).
func (l *Logger) WithSampling(initial, thereafter int) *Logger {
	c := l.clone()
	if initial <= 0 && thereafter <= 0 {
		c.sampler = nil
		return c
	}
	if initial < 0 {
		initial = 0
	}
	if thereafter <= 0 {
		thereafter = 1
	}
	c.sampler = newSampler(initial, thereafter)
	return c
}

// SetContextExtractor installs a function that derives structured fields from a
// context.Context attached via WithContext. The returned map is merged onto the
// record's fields at delivery time (after the logger's own With fields, so
// explicit With fields take precedence on key collision in JSON only if the
// context extractor doesn't also set the same key — last-writer-wins in the map).
// Set nil to disable context extraction.
//
// SetContextExtractor is intended to be called once at setup on a root logger
// before any WithContext child is produced; the extractor is cloned into every
// child.
func (l *Logger) SetContextExtractor(fn func(context.Context) map[string]interface{}) {
	l.ctxExtractor = fn
}

// WithContext returns a child Logger that, on each emit, extracts structured
// fields from ctx using the configured context extractor (default: looks up the
// common trace-id keys "trace_id"/"traceID"/"x-request-id" in ctx.Value). The
// returned logger captures ctx by reference; passing a new ctx requires a new
// WithContext call. The extractor runs once per record on the hot path.
//
// If no extractor is configured and none of the default keys are present in ctx,
// the child behaves like its parent (no extra fields).
func (l *Logger) WithContext(ctx context.Context) *Logger {
	c := l.clone()
	c.attachContextFields(ctx)
	return c
}

// attachContextFields extracts structured fields from ctx and appends them to
// the logger's fields slice (copy-on-write so the parent stays immutable). The
// extraction source is:
//  1. if the logger has a per-instance extractor (SetContextExtractor), ONLY it
//     runs (explicit override — useful to disable extraction by setting nil, or
//     to pin a custom scheme on one logger without affecting the global stack);
//  2. otherwise the global extractor stack runs (default trace/request-id/user
//     probe + anything added via AddContextExtractor, e.g. otel span/baggage).
func (l *Logger) attachContextFields(ctx context.Context) {
	if ctx == nil {
		return
	}
	var m map[string]interface{}
	if l.ctxExtractor != nil {
		// per-logger override: run ONLY this extractor (not the global stack),
		// so SetContextExtractor is a full replacement, not an addition.
		m = l.ctxExtractor(ctx)
	} else {
		m = runContextExtractors(ctx)
	}
	if len(m) == 0 {
		return
	}
	nf := make([]field, 0, len(l.fields)+len(m))
	nf = append(nf, l.fields...)
	for k, v := range m {
		nf = append(nf, field{key: k, val: v})
	}
	l.fields = nf
}

// defaultContextTraceKeys are the context.Value keys probed by the built-in
// extractor. They cover the common trace/request/user/tenant conventions across
// the ecosystem; callers needing more register an extractor via
// AddContextExtractor (e.g. for OpenTelemetry spans or custom baggage).
var defaultContextTraceKeys = []string{
	// distributed tracing
	"trace_id", "traceID", "trace-id",
	"span_id", "spanID", "span-id",
	// request / correlation
	"x-request-id", "requestId", "request_id", "x-correlation-id", "correlation_id",
	// business identity
	"uid", "user_id", "userId", "tenant_id", "tenantId", "org_id", "orgId",
}

// defaultContextExtractor looks up the common trace/request/user keys in
// ctx.Value and returns them as a fields map (only non-nil values are included).
// It is the zero-config base of the global extractor stack; callers add more
// via AddContextExtractor, and a per-logger SetContextExtractor overrides the
// whole stack.
func defaultContextExtractor(ctx context.Context) map[string]interface{} {
	if ctx == nil {
		return nil
	}
	var m map[string]interface{}
	for _, k := range defaultContextTraceKeys {
		if v := ctx.Value(k); v != nil {
			if m == nil {
				m = make(map[string]interface{}, len(defaultContextTraceKeys))
			}
			m[k] = v
		}
	}
	return m
}

// Debug level debug
func (l *Logger) Debug(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(DEBUG, fmt, args...)
}

// Info level info
func (l *Logger) Info(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(INFO, fmt, args...)
}

// Notice level notice
func (l *Logger) Notice(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(NOTICE, fmt, args...)
}

// Warn level warn
func (l *Logger) Warn(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(WARNING, fmt, args...)
}

// Error level error
func (l *Logger) Error(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(ERROR, fmt, args...)
}

// Critical level critical
func (l *Logger) Critical(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(CRITICAL, fmt, args...)
}

// Alert level alert
func (l *Logger) Alert(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(ALERT, fmt, args...)
}

// Emergency level emergency
func (l *Logger) Emergency(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(EMERGENCY, fmt, args...)
}

func (l *Logger) deliverRecordToWriter(level int, f string, args ...interface{}) {
	var msg string
	var fileStr string

	if level > int(l.level.Load()) {
		return
	}
	// Sampling runs before Metrics increment: a record dropped by the sampler
	// is never written and must not inflate the per-level counters (otherwise
	// monitoring would report a write rate the writers never see). nil sampler
	// is a no-op on the common path.
	if l.sampler != nil && !l.sampler.allow(level) {
		return
	}
	if l.recordsByLevel != nil {
		atomic.AddUint64(&l.recordsByLevel[level], 1)
	}

	msg = f
	sz := len(args)
	if sz != 0 {
		if strings.Contains(msg, "%") && !strings.Contains(msg, "%%") {
		} else {
			msg += strings.Repeat("%v", len(args))
		}
	}
	msg = fmt.Sprintf(msg, args...)

	// source code, file and line num
	if l.hasCaller.Load() {
		fi := bufPool.Get().(*bytes.Buffer)
		fi.Reset()
		pc, file, line, ok := runtime.Caller(2)
		if ok {
			fileName := path.Base(file)
			if l.fullPath.Load() {
				fileName = file
			}
			fi.WriteString(fileName)
			fi.WriteByte(':')
			fi.WriteString(strconv.Itoa(line))

			if l.withFuncName.Load() {
				funcName := runtime.FuncForPC(pc).Name()
				funcName = path.Base(funcName)
				fi.WriteByte(' ')
				fi.WriteString(funcName)
			}
		}
		fileStr = fi.String()
		bufPool.Put(fi)
	}

	// format time
	now := time.Now()
	sec := now.Unix()
	lpStr := l.lastTimeStr.Load()
	var lastTimeStr string
	// Bypass cache when layout has sub-second precision (e.g. ".000"):
	// cached values would give same-second records stale sub-second digits.
	if !l.hasSubSecond.Load() && lpStr != nil && l.lastTime.Load() == sec {
		lastTimeStr = *lpStr
	} else {
		lpLayout := l.layout.Load()
		layout := defaultLayout
		if lpLayout != nil {
			layout = *lpLayout
		}
		lastTimeStr = now.Format(layout)
		l.lastTime.Store(sec)
		l.lastTimeStr.Store(&lastTimeStr)
	}

	r := recordPool.Get().(*Record)
	r.msg = msg
	r.file = fileStr
	r.time = lastTimeStr
	r.level = level
	r.unixNano = now.UnixNano()
	r.seq = atomic.AddUint64(&globalSeq, 1)
	// Merge base fields (global static, e.g. hostname/server_ip) + logger fields
	// (from With/WithField/WithFields) + context fields. Priority: context >
	// logger > base (more specific wins). We append base fields first if present.
	if bf := l.baseFields.Load(); bf != nil && len(*bf) > 0 {
		if len(l.fields) > 0 {
			merged := make([]field, 0, len(*bf)+len(l.fields))
			merged = append(merged, *bf...)    // base first (lowest priority)
			merged = append(merged, l.fields...) // logger fields override base
			r.fields = merged
		} else {
			r.fields = *bf
		}
	} else {
		r.fields = l.fields
	}

	// Pre-serialize once for FormatJSON so every registered writer emits the
	// same bytes without re-marshaling. For FormatText r.jsonBytes stays nil and
	// writers fall back to r.String(). The JSON time field uses the ISO-ish
	// timestampLayout (machine-readable, timezone-aware) regardless of the
	// logger's text layout, per the structured-logging convention.
	if LogFormat(l.format.Load()) == FormatJSON {
		rJSON := RecordJSON{
			UnixNano: r.unixNano,
			Seq:      r.seq,
			Level:    LevelFlags[level],
			Msg:      msg,
			File:     fileStr,
		}
		// JSON time prefers the canonical timestampLayout; fall back to the
		// logger's text-layout time if formatting fails (never happens for the
		// const layout, but keeps rJSON.Time non-empty defensively).
		rJSON.Time = now.Format(timestampLayout)
		if rJSON.Time == "" {
			rJSON.Time = lastTimeStr
		}
		if len(l.fields) > 0 {
			rJSON.Fields = make(map[string]interface{}, len(l.fields))
			for _, f := range l.fields {
				rJSON.Fields[f.key] = f.val
			}
		}
		if b, err := jsonMarshal(rJSON); err == nil {
			r.jsonBytes = append(b, '\n')
		} else {
			// marshal error: leave jsonBytes nil so writers use String() (text).
			// Better to emit text than to drop the record.
		}
	}

	l.records <- r
}

func bootstrapLogWriter(logger *Logger) {
	var (
		r  *Record
		ok bool
	)

	if r, ok = <-logger.records; !ok {
		logger.c <- true
		return
	}

	for _, w := range logger.snapshotWriters() {
		if err := w.Write(r); err != nil {
			log.Printf("%v\n", err)
		}
	}

	flushTimer := time.NewTimer(logger.flushTimer)
	rotateTimer := time.NewTimer(logger.rotateTimer)

	for {
		select {
		case r, ok = <-logger.records:
			if !ok {
				logger.c <- true
				return
			}

			for _, w := range logger.snapshotWriters() {
				if err := w.Write(r); err != nil {
					log.Printf("%v\n", err)
				}
			}

			// Drop the fields + jsonBytes references before returning to the pool
			// so a logger's long-lived fields slice and the JSON buffer are not
			// pinned by a pooled record. r.msg/time/file are scalars and
			// overwritten on reuse, but r.fields/r.jsonBytes are slice headers
			// that would otherwise leak stale data into the next record.
			r.fields = nil
			r.jsonBytes = nil
			recordPool.Put(r)

		case <-flushTimer.C:
			for _, w := range logger.snapshotWriters() {
				if f, ok := w.(Flusher); ok {
					if err := f.Flush(); err != nil {
						log.Printf("%v\n", err)
					}
				}
			}
			flushTimer.Reset(logger.flushTimer)

		case <-rotateTimer.C:
			for _, w := range logger.snapshotWriters() {
				if r, ok := w.(Rotater); ok {
					if err := r.Rotate(); err != nil {
						log.Printf("%v\n", err)
					}
				}
			}
			rotateTimer.Reset(logger.rotateTimer)
		}
	}
}

func init() {
	recordPool = &sync.Pool{New: func() interface{} {
		return &Record{}
	}}
	bufPool = &sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
	loggerDefault.Store(newDefaultLoggerInstance())
}

// Register register writer
func Register(w Writer) {
	defaultLogger().Register(w)
}

// Close closes the package singleton and resets it so a subsequent SetupLog /
// Register / log call rebuilds a fresh logger (rather than orphaning writer
// daemons on a closed bootstrap). Safe to call at most once per setup cycle.
func Close() {
	if l := loggerDefault.Swap(nil); l != nil {
		l.Close()
	}
}

// SetLayout set the logger time layout, should call before logger real use
func SetLayout(layout string) {
	defaultLogger().SetLayout(layout)
}

// SetLevel set the logger level, should call before logger real use
func SetLevel(lvl int) {
	defaultLogger().SetLevel(lvl)
}

// SetFormat selects the record serialization format on the package singleton
// (FormatText default, FormatJSON for structured/machine-readable logs). Should
// be called before the logger is used for real (alongside SetLevel/SetLayout).
func SetFormat(f LogFormat) {
	defaultLogger().SetFormat(f)
}

// Format returns the package singleton's current serialization format.
func Format() LogFormat {
	return defaultLogger().Format()
}

// WithFullPath set the logger with full path, should call before logger real use
func WithFullPath(show bool) {
	defaultLogger().WithFullPath(show)
}

// WithFuncName set the logger with func name, should call before logger real use
func WithFuncName(show bool) {
	defaultLogger().WithFuncName(show)
}

// With returns a child Logger of the package singleton carrying a structured
// key/value field on every record it emits (see Logger.With). It does NOT
// mutate the singleton; the returned logger is bound to the singleton's records
// channel at call time, so callers should not reuse it across a Close() cycle
// (Close rebuilds the singleton with a new channel).
func With(key string, val interface{}) *Logger {
	return defaultLogger().With(key, val)
}

// WithField is an alias for With(key, val) on the package singleton.
func WithField(key string, val interface{}) *Logger {
	return defaultLogger().WithField(key, val)
}

// WithFields returns a child Logger of the package singleton carrying every
// key/value pair in fields (see Logger.WithFields).
func WithFields(fields map[string]interface{}) *Logger {
	return defaultLogger().WithFields(fields)
}

// WithSampling returns a child Logger of the package singleton with sampling
// applied (see Logger.WithSampling).
func WithSampling(initial, thereafter int) *Logger {
	return defaultLogger().WithSampling(initial, thereafter)
}

// SetContextExtractor installs a context-field extractor on the package
// singleton (see Logger.SetContextExtractor). Subsequent WithContext children
// inherit it.
func SetContextExtractor(fn func(context.Context) map[string]interface{}) {
	defaultLogger().SetContextExtractor(fn)
}

// WithContext returns a child Logger of the package singleton carrying fields
// extracted from ctx (see Logger.WithContext).
func WithContext(ctx context.Context) *Logger {
	return defaultLogger().WithContext(ctx)
}

// Debug level debug
func Debug(fmt string, args ...interface{}) {
	defaultLogger().deliverRecordToWriter(DEBUG, fmt, args...)
}

// Info level info
func Info(fmt string, args ...interface{}) {
	defaultLogger().deliverRecordToWriter(INFO, fmt, args...)
}

// Notice level notice
func Notice(fmt string, args ...interface{}) {
	defaultLogger().deliverRecordToWriter(NOTICE, fmt, args...)
}

// Warn level warn
func Warn(fmt string, args ...interface{}) {
	defaultLogger().deliverRecordToWriter(WARNING, fmt, args...)
}

// Error level error
func Error(fmt string, args ...interface{}) {
	defaultLogger().deliverRecordToWriter(ERROR, fmt, args...)
}

// Critical level critical
func Critical(fmt string, args ...interface{}) {
	defaultLogger().deliverRecordToWriter(CRITICAL, fmt, args...)
}

// Alert level alert
func Alert(fmt string, args ...interface{}) {
	defaultLogger().deliverRecordToWriter(ALERT, fmt, args...)
}

// Emergency level emergency
func Emergency(fmt string, args ...interface{}) {
	defaultLogger().deliverRecordToWriter(EMERGENCY, fmt, args...)
}

// The method is put here, so it's easy to test
func getLevelDefault(flag string, defaultFlag int, writer string) int {
	// level WARN == WARNING
	if strings.EqualFold(flag, LevelFlagWarn) {
		flag = LevelFlagWarning
	}

	for i, f := range LevelFlags {
		if strings.TrimSpace(strings.ToUpper(flag)) == f {
			return i
		}
	}
	log.Printf("[log4go] no matching level for writer(%v, flag:%v), use default level(%d, flag:%v)", writer, flag, defaultFlag, LevelFlags[defaultFlag])
	return defaultFlag
}
