package log4go

import (
	"context"
	"fmt"
	"io"
	"log"
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
	LevelFlagTrace     = "TRACE"
)

// RFC5424 log message levels + TRACE (finest, below DEBUG).
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
	TRACE            // Trace: variable-level detail for troubleshooting
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
		LevelFlagTrace,
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
// FormatLogfmt emits one space-separated key=value line per record (Loki /
// Promtail / docker friendly):
//
//	time=2026-06-25T15:04:05.000+0800 level=INFO msg="..." trace_id=abc user=42
//
// The format is decided once per record in deliverRecordToWriter and cached on
// r.formattedBytes, so every registered writer (Console/File/Net/IO) outputs the
// pre-serialized bytes without re-serializing. KafKaWriter already emits its
// own JSON payload and is unaffected.
type LogFormat int32

const (
	// FormatText is the default human-readable line format (Record.String).
	FormatText LogFormat = iota
	// FormatJSON emits one JSON object per record (Record.JSON), suitable for
	// machine ingestion / log shippers like Fluentd/Filebeat.
	FormatJSON
	// FormatLogfmt emits one space-separated key=value line per record
	// (Record.Logfmt), the format Loki/Promtail/docker consume natively.
	FormatLogfmt
)

// String returns the lowercase config name ("text"/"json"/"logfmt") used by LogConfig.
func (f LogFormat) String() string {
	switch f {
	case FormatJSON:
		return "json"
	case FormatLogfmt:
		return "logfmt"
	default:
		return "text"
	}
}

// ParseLogLogFormat parses a config string ("text"/"json"/"logfmt",
// case-insensitive) into a LogFormat. Unknown values fall back to FormatText
// with a log line so misconfiguration is loud rather than silently changing output.
func ParseLogLogFormat(s string) LogFormat {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "json":
		return FormatJSON
	case "logfmt":
		return FormatLogfmt
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

// field (the internal typed key/value pair) and its constructors/serialization
// live in field.go.

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
	// formattedBytes is the pre-serialized JSON form of the record, populated once by
	// deliverRecordToWriter when the Logger's format is FormatJSON. Writers
	// (Console/File/Net/IO) that support the FormattedWriter fast path emit these
	// bytes verbatim to avoid re-serializing per writer. nil under FormatText.
	// Reset to nil by the bootstrap goroutine before returning to the pool.
	formattedBytes []byte
}

// globalSeq is the process-global monotonic record sequence counter.
// Incremented atomically in deliverRecordToWriter so that even under high
// concurrency (multi-goroutine, multi-core), every record gets a unique seq
// that reflects its logical creation order.
var globalSeq uint64

// callerCache memoizes the "file:line[ func]" string per call site. A program
// counter always maps to one source location, so the runtime string work
// (FuncForPC, FileLine, name base) runs exactly once per log call site; every
// subsequent call at that site is a map lookup. This removes the per-record
// caller allocs (runtime.Caller's file string + the file:line concat). The key
// includes fullPath/withFunc so different caller-render configs cache separate
// strings. Bounded by the number of distinct log call sites (small).
var (
	callerCache   = make(map[callerKey]string)
	callerCacheMu sync.RWMutex
)

type callerKey struct {
	pc       uintptr
	fullPath bool
	withFunc bool
}

// callerFileLine resolves pc to "base.go:line" (or full path, or with func name)
// using callerCache. Misses compute once via runtime.FuncForPC.FileLine and
// store; hits return the memoized string with no allocation.
func callerFileLine(pc uintptr, fullPath, withFunc bool) string {
	key := callerKey{pc: pc, fullPath: fullPath, withFunc: withFunc}
	callerCacheMu.RLock()
	s, ok := callerCache[key]
	callerCacheMu.RUnlock()
	if ok {
		return s
	}
	s = computeCallerFileLine(pc, fullPath, withFunc)
	callerCacheMu.Lock()
	callerCache[key] = s
	callerCacheMu.Unlock()
	return s
}

// computeCallerFileLine builds the caller string for a PC (runs once per site).
func computeCallerFileLine(pc uintptr, fullPath, withFunc bool) string {
	fn := runtime.FuncForPC(pc)
	file, line := fn.FileLine(pc)
	if !fullPath {
		if i := strings.LastIndexByte(file, '/'); i >= 0 {
			file = file[i+1:]
		}
	}
	var lb [20]byte
	s := file + ":" + string(strconv.AppendInt(lb[:0], int64(line), 10))
	if withFunc {
		if name := fn.Name(); name != "" {
			if i := strings.LastIndexByte(name, '/'); i >= 0 {
				name = name[i+1:]
			}
			s += " " + name
		}
	}
	return s
}

// log4goPkgDir is the on-disk source directory of this package, captured once
// from a known internal frame. Used to tell log4go-internal frames (Info /
// deliverRecordToWriter / compiler wrappers) from the user's caller. Derived
// from the actual file location so it stays correct under vendoring, module
// relocation, or a renamed import path.
var log4goPkgDir = func() string {
	var pcs [1]uintptr
	runtime.Callers(1, pcs[:]) // depth 1 = this closure's frame, inside log4go
	file, _ := runtime.FuncForPC(pcs[0]).FileLine(pcs[0])
	if i := strings.LastIndexByte(file, '/'); i >= 0 {
		return file[:i+1] // ".../log4go/"
	}
	return ""
}()

var (
	callerInternalCache = make(map[uintptr]bool)
	callerInternalMu    sync.RWMutex
)

// callerIsInternal reports whether pc is a frame inside log4go's own production
// source (Info/Debug/deliverRecordToWriter, compiler-generated wrappers, etc.).
// Memoized per pc. A frame in log4go's source dir that lives in a *_test.go file
// is treated as a USER frame — internal tests exercise the caller path and their
// call sites are the callers under test. Production builds never carry *_test.go
// frames, so this distinction is exact for real callers.
func callerIsInternal(pc uintptr) bool {
	callerInternalMu.RLock()
	v, ok := callerInternalCache[pc]
	callerInternalMu.RUnlock()
	if ok {
		return v
	}
	internal := false
	if log4goPkgDir != "" {
		if file, _ := runtime.FuncForPC(pc).FileLine(pc); file != "" {
			internal = strings.HasPrefix(file, log4goPkgDir) && !strings.HasSuffix(file, "_test.go")
		}
	}
	callerInternalMu.Lock()
	callerInternalCache[pc] = internal
	callerInternalMu.Unlock()
	return internal
}

// FieldsJSON returns the record's structured fields marshaled to a JSON object
// (e.g. `{"trace_id":"abc","user":42}`). Returns "" when there are no fields,
// so callers can cheaply skip the append. Used by Record.String and
// KafKaWriter.buildPayload.
func (r *Record) FieldsJSON() string {
	if len(r.fields) == 0 {
		return ""
	}
	// Direct typed append — no map allocation, no reflection. Scalars render
	// inline; kindAny values still go through the active codec (appendFieldJSON).
	buf := make([]byte, 0, 32+len(r.fields)*16)
	buf = appendFieldsJSONObject(buf, r.fields)
	return string(buf)
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
// Field order is fixed (unix_nano, seq, time, level, msg, file, fields); scalars
// are rendered by direct typed append (no map, no reflection), so this path is
// allocation-free for scalar fields. This is called once per record in
// deliverRecordToWriter (FormatJSON only) and cached on r.formattedBytes, so each
// record pays exactly one serialization regardless of how many writers run.
func (r *Record) JSON() []byte {
	buf := make([]byte, 0, 192+len(r.fields)*16)
	buf = append(buf, '{')
	buf = append(buf, `"unix_nano":`...)
	buf = strconv.AppendInt(buf, r.unixNano, 10)
	buf = append(buf, `,"seq":`...)
	buf = strconv.AppendUint(buf, r.seq, 10)
	buf = append(buf, `,"time":"`...)
	buf = appendISOTimeUTC(buf, r.unixNano)
	buf = append(buf, '"')
	buf = append(buf, `,"level":`...)
	buf = appendJSONQuoted(buf, LevelFlags[r.level])
	buf = append(buf, `,"msg":`...)
	buf = appendJSONQuoted(buf, r.msg)
	if r.file != "" {
		buf = append(buf, `,"file":`...)
		buf = appendJSONQuoted(buf, r.file)
	}
	if len(r.fields) > 0 {
		buf = append(buf, `,"fields":`...)
		buf = appendFieldsJSONObject(buf, r.fields)
	}
	buf = append(buf, '}', '\n')
	return buf
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

// Stopper is implemented by writers that own a background daemon and/or a
// connection that must be released on shutdown (File/Kafka/Net). Logger.Close
// stops every registered Stopper so a single log4go.Close() reclaims all writer
// goroutines, channels, file handles, and connections. Stop is idempotent, so a
// writer the caller already Stop()ed (or one without a daemon) is a no-op.
type Stopper interface {
	Stop()
}

// Pauser is implemented by writers that can be paused at runtime (Pause drops
// incoming records without removing the writer or touching its connection/
// daemon — useful to silence a noisy sink during an incident). Resume restores
// delivery. Non-destructive, atomic, and safe under any load.
type Pauser interface {
	Pause()
	Resume()
	Paused() bool
}

// Named is implemented by writers that carry a stable name (WriterNameConsole,
// etc.), enabling by-name control (Logger.PauseWriter(name), SetWriterLevel).
type Named interface {
	Name() string
}

// LevelSetter is implemented by writers whose level can be changed at runtime
// (atomic, lock-free). Setting a writer's level to a high value effectively
// silences it without removing it.
type LevelSetter interface {
	SetLevel(level int)
}

// RuntimeConfig is the hot, lock-free configuration surface of a Logger. Every
// method takes effect on the next record via atomic loads on the delivery path —
// no mutex, no stall — so any of them is safe to call at any time, including
// under extreme throughput (millions of records/sec, as in real-time ad
// serving). It is the runtime, non-destructive counterpart to SetupLog (which
// builds writers once at startup).
//
// Use it for live toggles that must NOT reconfigure writers: bump the level for
// debugging (SetLevel), switch text/JSON (SetFormat), enable caller/func names
// (WithCaller/WithFuncName), adjust or disable sampling (SetSampling), toggle
// trace/context capture (SetContextExtractor), or attach a static field
// (SetBaseField). None of these touch writer goroutines or connections, so they
// cost nothing on the hot path. *Logger satisfies this interface; obtain it via
// Logger.Runtime or the package-level Runtime.
//
// Note: SetBaseField uses copy-on-write atomic swap — safe to call
// concurrently with logging (no data race), but intended for infrequent updates
// (startup env fields, occasional additions), not high-frequency mutation.
type RuntimeConfig interface {
	SetLevel(level int)
	SetFormat(format LogFormat)
	SetLayout(layout string)
	WithCaller(enable bool)
	WithFuncName(enable bool)
	WithFullPath(enable bool)
	SetSampling(initial, thereafter int)
	SetContextExtractor(fn func(context.Context) map[string]interface{})
	SetBaseField(key string, val interface{})
	RemoveBaseField(key string)
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

	// quit is closed to retire the logger (Close/Reload). enqueue selects on it so
	// a retiring logger drops in-flight records instead of racing a channel close.
	// Shared by pointer-value across a clone tree (like c), so retiring the root
	// retires every child. records is NEVER closed.
	quit chan struct{}

	layout          atomic.Pointer[string]
	level           atomic.Int32
	format          atomic.Int32       // LogFormat (FormatText/FormatJSON); atomic for lock-free hot-path read
	recordsByLevel  *[TRACE + 1]uint64 // per-level Written counters (post-sample, incremented in bootstrap); shared pointer
	occurredByLevel *[TRACE + 1]uint64 // per-level Occurred counters (every log call, pre-filter); shared pointer
	fullPath        atomic.Bool        // show full path, default only show file:line_number
	withFuncName    atomic.Bool        // show caller func name
	hasCaller       atomic.Bool        // capture caller (file:line); disable for max throughput

	// baseFields are global static fields registered once via SetBaseField(s)
	// and carried by EVERY record — including those emitted by child Loggers
	// produced via With/WithField/WithFields/WithContext. To honor that contract
	// live (a SetBaseField on the root is visible to every child, even children
	// created earlier), the storage is shared by pointer: clone() copies the
	// *baseFieldsHolder, so parent and children read the same atomic. Stored in
	// an atomic.Pointer for lock-free hot-path read.
	baseFields *baseFieldsHolder

	// fields carries structured key/value pairs attached via With/WithField/
	// WithFields. A child Logger always gets its OWN copy (see clone), so a
	// parent's slice is never mutated and is safe to read concurrently from the
	// deliverRecordToWriter hot path without locking. Immutable after the With
	// call that produced it.
	fields []field

	// sampler drops high-frequency records to prevent log storms. nil disables
	// sampling (the default). Held in an atomic.Pointer so it can be swapped at
	// runtime (SetSampling) without racing the deliverRecordToWriter hot path,
	// which loads it once per record.
	sampler atomic.Pointer[Sampler]

	// priorityLevel is the severity threshold below which records ALWAYS bypass
	// sampling (even on a sampled-out request). Records with level <= priorityLevel
	// are kept regardless of sampleDrop. Default -1 (no bypass; sampling governs
	// all). Set to ERROR so EMERGENCY/ALERT/CRITICAL/ERROR are always logged —
	// the industry-standard "error protection" pattern (Stripe/Dapper/Netflix).
	priorityLevel atomic.Int32

	// samplingStrategy is the pluggable id-based sampling policy
	// (TraceIDRatioBased / TailDigit / Full). nil ⇒ Full (keep all). It is
	// evaluated ONCE per request at WithContext (decided from the correlation id
	// in ctx) and the verdict cached in sampleDrop, so the per-record hot path
	// pays only an atomic-bool load — the strategy itself never runs per record.
	samplingStrategy atomic.Pointer[SamplingStrategy]
	// sampleDrop is the cached per-request sampling verdict: true ⇒ drop records
	// from this logger (set by WithContext when the strategy says "not sampled").
	// Default false ⇒ keep. Read once per record on the hot path.
	sampleDrop atomic.Bool

	// ctxExtractor derives structured fields from a context.Context supplied via
	// WithContext. nil (the zero atomic pointer) disables extraction. Held in an
	// atomic.Pointer so it can be toggled live (SetContextExtractor) — e.g. turn
	// trace-context capture on only when investigating a distributed incident.
	ctxExtractor atomic.Pointer[contextExtractor]

	// hasSubSecond is set by SetLayout when the layout contains a fractional
	// seconds directive (".000", ".999", etc). When true, the time cache is
	// bypassed so every record gets a fresh time.Now().Format() call — otherwise
	// same-second records would share the same (stale) sub-second value.
	hasSubSecond atomic.Bool
}

// baseFieldsHolder is the shared backing store for a logger's base fields. It is
// heap-allocated once per logger tree and shared (by pointer) with every clone,
// so SetBaseField on any logger in the tree is visible to all of them — base
// fields affect the singleton and every child Logger, per the SetBaseField doc.
type baseFieldsHolder struct {
	v atomic.Pointer[[]field]
}

// contextExtractor wraps a context-field extractor so it can be held in an
// atomic.Pointer[contextExtractor] and toggled live via SetContextExtractor (a
// nil pointer disables extraction — the common case, so the hot path pays no map
// allocation). Used for runtime trace/context-capture control.
type contextExtractor struct {
	fn func(context.Context) map[string]interface{}
}

// NewLogger create the logger
func NewLogger() *Logger {
	return defaultLogger()
}

// newLoggerWithRecords is useful for go test
func newLoggerWithRecords(records chan *Record) *Logger {
	l := new(Logger)
	l.writers.Store(make([]Writer, 0, 1)) // normal least has console writer

	l.records = records
	l.c = make(chan bool, 1)
	l.quit = make(chan struct{})
	l.recordsByLevel = new([TRACE + 1]uint64)
	l.occurredByLevel = new([TRACE + 1]uint64)
	l.priorityLevel.Store(-1)          // default: no bypass; sampling governs all
	l.baseFields = &baseFieldsHolder{} // shared with every clone (see clone)
	l.level.Store(int32(DEBUG))
	lp := DefaultLayout
	l.layout.Store(&lp)
	l.hasCaller.Store(true)

	go bootstrapLogWriter(l)

	return l
}

// Register registers a writer (calling Init to start its daemon). It panics on
// Init failure; use registerOrFail for a non-panicking variant (Reload).
func (l *Logger) Register(w Writer) {
	if err := l.registerOrFail(w); err != nil {
		panic(err)
	}
}

// registerOrFail is Register without the panic: it returns the Init error so a
// caller that must not disturb the live logger (Reload) can fail gracefully. On
// success the writer is appended copy-on-write to the bootstrap's writer list.
func (l *Logger) registerOrFail(w Writer) error {
	if err := w.Init(); err != nil {
		return err
	}

	// copy-on-write so the bootstrap goroutine can read writers lock-free.
	cur := l.writers.Load().([]Writer)
	next := make([]Writer, len(cur)+1)
	copy(next, cur)
	next[len(cur)] = w
	l.writers.Store(next)
	return nil
}

// snapshotWriters returns the current writers slice for lock-free iteration
// by the bootstrap goroutine; Register replaces the slice copy-on-write so
// this view stays valid.
func (l *Logger) snapshotWriters() []Writer {
	return l.writers.Load().([]Writer)
}

// Writers returns a snapshot of the registered writers (copy-on-write, so the
// slice stays valid for iteration). Use it for direct, typed control, e.g.
// `for _, w := range log4go.Writers() { if p, ok := w.(log4go.Pauser); ok { p.Pause() } }`.
func (l *Logger) Writers() []Writer { return l.snapshotWriters() }

// SamplingStatus describes the active sampling strategy for ops display.
type SamplingStatus struct {
	Strategy string // "full" / "trace_id_ratio:0.10" / "tail_digit:10:3" / "<TypeName>"
}

// WriterStatus is a writer's runtime state for the Status snapshot.
type WriterStatus struct {
	Name   string
	Paused bool
	// Metrics is the writer's own Metrics() snapshot if it exposes one (per
	// writer type — FileWriterMetrics, WriterMetrics, …); nil otherwise. Fetch
	// a typed snapshot via the writer directly for full detail.
	Metrics interface{}
}

// RuntimeStatus is a point-in-time snapshot of the logger's runtime state —
// the active sampling strategy and every writer's name/paused/metrics. Designed
// for monitoring / admin UIs / business reconciliation. Reading it does NOT touch
// the delivery hot path (it reads copy-on-write snapshots + atomic loads).
type RuntimeStatus struct {
	Sampling SamplingStatus
	Writers  []WriterStatus
}

// Status returns a runtime snapshot of this logger (active sampling strategy +
// per-writer state). For per-writer metrics/level, type-assert from Writers().
func (l *Logger) Status() RuntimeStatus {
	ws := l.Writers()
	out := RuntimeStatus{
		Sampling: SamplingStatus{Strategy: describeStrategy(l.samplingStrategy.Load())},
		Writers:  make([]WriterStatus, 0, len(ws)),
	}
	for _, w := range ws {
		var s WriterStatus
		if n, ok := w.(Named); ok {
			s.Name = n.Name()
		}
		if p, ok := w.(Pauser); ok {
			s.Paused = p.Paused()
		}
		s.Metrics = metricSnapshot(w)
		out.Writers = append(out.Writers, s)
	}
	return out
}

// describeStrategy renders the active strategy for display. nil ⇒ "full"
// (FullSampling / keep-all default).
func describeStrategy(p *SamplingStrategy) string {
	if p == nil {
		return "full"
	}
	switch s := (*p).(type) {
	case FullSampling:
		return "full"
	case TraceIDRatioBased:
		return fmt.Sprintf("trace_id_ratio:%g", s.Ratio)
	case TailDigitSampling:
		return fmt.Sprintf("tail_digit:%d:%d", s.Modulus, s.Keep)
	default:
		return fmt.Sprintf("%T", s)
	}
}

// metricSnapshot returns a writer's Metrics() if it exposes one. Each writer's
// Metrics() returns its own concrete type, so we assert the known shapes rather
// than require a shared interface (no writer changes needed); nil if none.
func metricSnapshot(w Writer) interface{} {
	switch v := w.(type) {
	case interface{ Metrics() FileWriterMetrics }:
		return v.Metrics()
	case interface{ Metrics() WriterMetrics }: // KafKaWriter
		return v.Metrics()
	case interface{ Metrics() NetWriterMetrics }:
		return v.Metrics()
	case interface{ Metrics() WebhookWriterMetrics }:
		return v.Metrics()
	}
	return nil
}

// findWriter returns the first registered writer with the given Name(), or nil.
func (l *Logger) findWriter(name string) Writer {
	for _, w := range l.snapshotWriters() {
		if n, ok := w.(Named); ok && n.Name() == name {
			return w
		}
	}
	return nil
}

// PauseWriter pauses the named writer (drops its records without removing it or
// touching its connection/daemon). Returns true if a writer was paused; false if
// no such named writer exists or it is not pausable.
func (l *Logger) PauseWriter(name string) bool {
	if p, ok := l.findWriter(name).(Pauser); ok && p != nil {
		p.Pause()
		return true
	}
	return false
}

// ResumeWriter resumes the named writer. Returns true if a writer was resumed.
func (l *Logger) ResumeWriter(name string) bool {
	if p, ok := l.findWriter(name).(Pauser); ok && p != nil {
		p.Resume()
		return true
	}
	return false
}

// WriterPaused reports whether the named writer is paused.
func (l *Logger) WriterPaused(name string) bool {
	if p, ok := l.findWriter(name).(Pauser); ok && p != nil {
		return p.Paused()
	}
	return false
}

// LoggerMetrics is a snapshot of per-level record counters for monitoring.
type LoggerMetrics struct {
	Occurred [TRACE + 1]uint64 // every log call (pre-filter/pre-sample)
	Records  [TRACE + 1]uint64 // Written = delivered (post-sample, from bootstrap)
	Dropped  [TRACE + 1]uint64 // Occurred − Records (may briefly include in-flight)
}

// Metrics returns per-level counters of this logger for monitoring. Written
// (Records) is counted in the bootstrap goroutine (single-writer, no caller
// contention); Occurred is on the caller path. Dropped = Occurred − Records.
func (l *Logger) Metrics() LoggerMetrics {
	var m LoggerMetrics
	for i := 0; i <= TRACE; i++ {
		if l.occurredByLevel != nil {
			m.Occurred[i] = atomic.LoadUint64(&l.occurredByLevel[i])
		}
		if l.recordsByLevel != nil {
			m.Records[i] = atomic.LoadUint64(&l.recordsByLevel[i])
		}
		if m.Occurred[i] >= m.Records[i] {
			m.Dropped[i] = m.Occurred[i] - m.Records[i]
		}
	}
	return m
}

// Metrics returns the default logger's per-level record counters for monitoring.
func Metrics() LoggerMetrics { return defaultLogger().Metrics() }

// Close close logger
func (l *Logger) Close() {
	// Retire the logger by closing quit (NOT records): concurrent senders select
	// on quit and drop instead of racing a channel close, so Reload/Close under
	// traffic is race-free and panic-free. The bootstrap drains in-flight records
	// and exits; records is left open and GC'd with the logger.
	close(l.quit)
	<-l.c

	for _, w := range l.snapshotWriters() {
		if f, ok := w.(Flusher); ok {
			if err := f.Flush(); err != nil {
				log.Println(err)
			}
		}
		// Stop async-daemon writers (File/Kafka/Net) so a single log4go.Close()
		// reclaims their goroutines, channels, file handles, and connections.
		// Stop is idempotent, so a writer the caller already Stop()ed is a no-op.
		if s, ok := w.(Stopper); ok {
			s.Stop()
		}
		// Writers that own a resource exposed via Close() error (e.g.
		// WebhookWriter wrapping a WebhookAlertSink) are shut down here too.
		if c, ok := w.(io.Closer); ok {
			if err := c.Close(); err != nil {
				log.Println(err)
			}
		}
	}
}

// Runtime returns the hot, lock-free configuration surface of this Logger
// (SetLevel / WithCaller / SetBaseField / ...). See RuntimeConfig.
func (l *Logger) Runtime() RuntimeConfig { return l }

// SetLayout set the logger time layout
func (l *Logger) SetLayout(layout string) {
	v := layout
	l.hasSubSecond.Store(strings.Contains(layout, ".0") || strings.Contains(layout, ".9"))
	l.layout.Store(&v)
}

// Base field management — three orthogonal operations on a key-value store.
// Each operates on a SINGLE key (or all), never has implicit side effects on
// other keys, and is copy-on-write (safe under concurrent logging).
//
// | Operation              | Effect on the key       | Effect on OTHER keys |
// |------------------------|-------------------------|----------------------|
// | SetBaseField(key, val) | Added or updated        | Untouched            |
// | RemoveBaseField(key)   | Removed (no-op if gone) | Untouched            |
// | ClearBaseFields()      | —                       | All removed          |
//
// There is intentionally NO batch-replace API (a SetBaseFields that overwrites
// everything was removed: it violated consistency with SetBaseField's upsert
// semantics and was a footgun — callers would lose fields by surprise). To set
// multiple fields, call SetBaseField per key (cheap: base field sets are small).
//
// Base field 管理 —— 三个正交操作：
// SetBaseField（加/改一个 key）、RemoveBaseField（删一个 key）、ClearBaseFields（清空）。
// 无批量覆盖 API（故意去掉 —— 与 upsert 语义不一致，易误删字段）。批量设 = 循环 SetBaseField。

// SetBaseField adds or updates (upsert) a single base field by key. If the key
// already exists its value is replaced; if it is new it is appended. Other keys
// are untouched. Every subsequent log record carries all base fields.
//
// Typical usage at startup:
//
//	for k, v := range configMap {
//	    log4go.SetBaseField(k, v)
//	}
//
// Runtime update (e.g. canary deploy):
//
//	log4go.SetBaseField("service_name", "bidder-canary")
func (l *Logger) SetBaseField(key string, val interface{}) {
	f := fieldOf(key, val)
	cur := l.baseFields.v.Load()
	if cur != nil {
		next := make([]field, len(*cur))
		copy(next, *cur) // copy-on-write: never mutate the snapshot the bootstrap reads
		for i := range next {
			if next[i].key == key {
				next[i] = f // upsert: replace existing
				l.baseFields.v.Store(&next)
				return
			}
		}
		next = append(next, f)
		l.baseFields.v.Store(&next)
		return
	}
	next := []field{f}
	l.baseFields.v.Store(&next)
}

// RemoveBaseField removes a single base field by key. No-op if the key does not
// exist. Other keys are untouched.
func (l *Logger) RemoveBaseField(key string) {
	cur := l.baseFields.v.Load()
	if cur == nil || len(*cur) == 0 {
		return
	}
	s := *cur
	next := make([]field, 0, len(s))
	found := false
	for _, f := range s {
		if f.key == key {
			found = true
			continue
		}
		next = append(next, f)
	}
	if !found {
		return
	}
	if len(next) == 0 {
		l.baseFields.v.Store(nil)
	} else {
		l.baseFields.v.Store(&next)
	}
}

// ClearBaseFields removes ALL base fields. Subsequent records carry no base
// fields until SetBaseField is called again.
func (l *Logger) ClearBaseFields() {
	l.baseFields.v.Store(nil)
}

// SetBaseField upserts (add-or-update by key) a base field on the default logger.
func SetBaseField(key string, val interface{}) { defaultLogger().SetBaseField(key, val) }

// RemoveBaseField removes a single base field by key on the default logger.
func RemoveBaseField(key string) { defaultLogger().RemoveBaseField(key) }

// ClearBaseFields removes ALL base fields from the default logger.
func ClearBaseFields() { defaultLogger().ClearBaseFields() }

// SetLevel set the logger level
func (l *Logger) SetLevel(lvl int) {
	l.level.Store(int32(lvl))
}

// SetFormat selects the record serialization format (FormatText or FormatJSON).
// FormatText (the default) emits the human-readable line; FormatJSON emits one
// JSON object per record (see Record.JSON). The format is read once per record
// in deliverRecordToWriter and decides whether r.formattedBytes is pre-serialized.
// All registered writers honor the format via the FormattedWriter fast path
// (they emit r.formattedBytes when non-nil, else r.String()), so no per-writer change
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
		quit:            l.quit,            // shared: retiring the root retires every child
		fields:          l.fields,          // shared read-only; With copies before appending
		recordsByLevel:  l.recordsByLevel,  // shared pointer so children's emits count on the root
		occurredByLevel: l.occurredByLevel, // shared: Occurred counters propagate to children
		baseFields:      l.baseFields,      // shared pointer: SetBaseField on the root is live-visible to every child
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
	// sampler/ctxExtractor are atomic pointers — copy the current value (a child
	// later changing them via SetSampling/SetContextExtractor only affects itself).
	if s := l.sampler.Load(); s != nil {
		c.sampler.Store(s)
	}
	if x := l.ctxExtractor.Load(); x != nil {
		c.ctxExtractor.Store(x)
	}
	// samplingStrategy + the cached per-request sampleDrop verdict propagate to
	// children, so a WithContext-decided "sampled" child keeps its verdict across
	// further With calls.
	if ss := l.samplingStrategy.Load(); ss != nil {
		c.samplingStrategy.Store(ss)
	}
	c.sampleDrop.Store(l.sampleDrop.Load())
	c.priorityLevel.Store(l.priorityLevel.Load())
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
	return l.withField(fieldOf(key, val))
}

// withField returns a child Logger carrying one more (already typed) field.
// copy-on-write: the parent's slice stays immutable. All With* variants funnel
// through here so the clone happens once.
func (l *Logger) withField(f field) *Logger {
	c := l.clone()
	nf := make([]field, len(l.fields), len(l.fields)+1)
	copy(nf, l.fields)
	nf = append(nf, f)
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
		nf = append(nf, fieldOf(k, v))
	}
	c.fields = nf
	return c
}

// WithAttrs returns a child Logger carrying the given typed Fields (constructed
// via log4go.String/Int/Bool/...). Scalars never box — the allocation-free
// counterpart to With(key, interface{}).
func (l *Logger) WithAttrs(attrs ...Field) *Logger {
	if len(attrs) == 0 {
		return l.clone()
	}
	c := l.clone()
	nf := make([]field, len(l.fields), len(l.fields)+len(attrs))
	copy(nf, l.fields)
	for _, a := range attrs {
		nf = append(nf, a.f)
	}
	c.fields = nf
	return c
}

// WithString/WithInt/... are the typed, allocation-free variants of With for
// the common scalar types. They avoid the interface{} boxing that With pays.
func (l *Logger) WithString(key, val string) *Logger { return l.withField(strField(key, val)) }

// WithInt returns a child Logger carrying an int-typed structured field.
func (l *Logger) WithInt(key string, val int) *Logger { return l.withField(intField(key, val)) }

// WithInt64 returns a child Logger carrying an int64-typed structured field.
func (l *Logger) WithInt64(key string, val int64) *Logger {
	return l.withField(int64Field(key, val))
}

// WithUint64 returns a child Logger carrying a uint64-typed structured field.
func (l *Logger) WithUint64(key string, val uint64) *Logger {
	return l.withField(uint64Field(key, val))
}

// WithBool returns a child Logger carrying a bool-typed structured field.
func (l *Logger) WithBool(key string, val bool) *Logger { return l.withField(boolField(key, val)) }

// WithFloat64 returns a child Logger carrying a float64-typed structured field.
func (l *Logger) WithFloat64(key string, val float64) *Logger {
	return l.withField(floatField(key, val))
}

// WithDuration returns a child Logger carrying a duration-typed structured field
// (rendered as nanoseconds, the slog convention).
func (l *Logger) WithDuration(key string, val time.Duration) *Logger {
	return l.withField(durField(key, val))
}

// WithTime returns a child Logger carrying a time-typed structured field
// (rendered as an RFC3339 UTC timestamp).
func (l *Logger) WithTime(key string, val time.Time) *Logger {
	return l.withField(timeField(key, val))
}

// WithBytes returns a child Logger carrying a bytes-typed structured field
// (base64-encoded on the JSON path).
func (l *Logger) WithBytes(key string, val []byte) *Logger {
	return l.withField(bytesField(key, val))
}

// WithError returns a child Logger carrying an error-typed structured field
// whose value is rendered via the error's Error() (panic-safe).
func (l *Logger) WithError(key string, val error) *Logger {
	return l.withField(errField(key, val))
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
		c.sampler.Store(nil)
		return c
	}
	if initial < 0 {
		initial = 0
	}
	if thereafter <= 0 {
		thereafter = 1
	}
	c.sampler.Store(newSampler(initial, thereafter))
	return c
}

// SetSampling atomically applies sampling to this logger in place (unlike
// WithSampling, which returns a child). The first `initial` records at each
// level pass, then one every `thereafter`; initial/thereafter <= 0 disables
// sampling. Safe to call concurrently with logging — the next record observes
// the new policy via an atomic pointer load.
func (l *Logger) SetSampling(initial, thereafter int) {
	if initial <= 0 && thereafter <= 0 {
		l.sampler.Store(nil)
		return
	}
	if initial < 0 {
		initial = 0
	}
	if thereafter <= 0 {
		thereafter = 1
	}
	l.sampler.Store(newSampler(initial, thereafter))
}

// SetContextExtractor installs a function that derives structured fields from a
// context.Context attached via WithContext. The returned map is merged onto the
// record's fields at delivery time. It is stored atomically, so it is safe to
// swap at runtime under any load — e.g. install a trace-context extractor only
// when an incident requires distributed-trace capture, and pass nil to clear the
// per-logger override (the global extractor stack, if any, then runs).
//
// The extractor is cloned into every child Logger produced after the call; a
// child may override it independently.
func (l *Logger) SetContextExtractor(fn func(context.Context) map[string]interface{}) {
	if fn == nil {
		l.ctxExtractor.Store(nil)
		return
	}
	l.ctxExtractor.Store(&contextExtractor{fn: fn})
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
	// Evaluate the sampling strategy ONCE per request (cheap; not per record):
	// decide from the correlation id in ctx and cache the verdict on the child as
	// sampleDrop. The deliver hot path then reads only the atomic bool. A nil
	// strategy (FullSampling) skips this — sampleDrop stays false (keep).
	if ss := c.samplingStrategy.Load(); ss != nil {
		id := correlationIDFromContext(ctx)
		c.sampleDrop.Store(!(*ss).ShouldLog(id))
	}
	return c
}

// correlationIDFromContext returns the first correlation id found in ctx.Value
// (in defaultContextTraceKeys order: trace_id first, then request/correlation,
// then device). Empty when none is present. Used to make the per-request
// sampling decision deterministic across services (all see the same id).
func correlationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	for _, k := range defaultContextTraceKeys {
		if v := ctx.Value(k); v != nil {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

// SetSamplingStrategy installs an id-based sampling policy evaluated once per
// request at WithContext (nil ⇒ FullSampling / keep all, the default). Because
// the decision is a pure function of the correlation id, every service with the
// same id keeps-or-drops the whole chain together — no fragmentation. Atomic +
// lock-free; safe under any load.
func (l *Logger) SetSamplingStrategy(s SamplingStrategy) {
	if s == nil {
		l.samplingStrategy.Store(nil)
		return
	}
	l.samplingStrategy.Store(&s)
}

// SetPriorityLevel sets the severity threshold below which records ALWAYS bypass
// sampling (even on a sampled-out request). Records with level <= this value
// (numerically lower = more severe: EMERGENCY=0 ... ERROR=3 ... DEBUG=7) are
// always kept. Default -1 (no bypass). Set to ERROR so errors are never sampled
// away — the industry-standard "error protection" pattern (Stripe/Dapper/Netflix
// all keep error traces/logs regardless of sampling). Atomic + lock-free.
func (l *Logger) SetPriorityLevel(level int) {
	l.priorityLevel.Store(int32(level))
}

// SetSamplingStrategyFor temporarily installs s for duration, then auto-reverts
// to the previous strategy (or nil/Full). Returns a stop func for early cancel
// (idempotent). Use as a bounded debug/troubleshoot window, e.g. sample 10% for
// 30 minutes then return to full logging. A new call overrides any active
// session. Safe under concurrency.
func (l *Logger) SetSamplingStrategyFor(s SamplingStrategy, duration time.Duration) (stop func()) {
	prev := l.samplingStrategy.Load()
	l.SetSamplingStrategy(s)
	done := make(chan struct{})
	var once sync.Once
	revert := func() { l.samplingStrategy.Store(prev) }
	go func() {
		t := time.NewTimer(duration)
		defer t.Stop()
		select {
		case <-t.C:
			revert() // timer expired -> revert
		case <-done:
			// stop() already reverted synchronously
		}
	}()
	return func() {
		once.Do(func() { close(done); revert() }) // synchronous revert + signal goroutine
	}
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
	if x := l.ctxExtractor.Load(); x != nil {
		// per-logger override: run ONLY this extractor (not the global stack),
		// so SetContextExtractor is a full replacement, not an addition.
		m = x.fn(ctx)
	} else {
		m = runContextExtractors(ctx)
	}
	if len(m) == 0 {
		return
	}
	nf := make([]field, 0, len(l.fields)+len(m))
	nf = append(nf, l.fields...)
	for k, v := range m {
		nf = append(nf, fieldOf(k, v))
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
	// business identity (ad-tech: device id is a first-class correlation key)
	"device_id", "deviceId", "did", "dpid",
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

// Trace level trace — finest granularity, below DEBUG. Use for variable-level
// detail during troubleshooting (individual values, loop iterations).
func (l *Logger) Trace(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(TRACE, fmt, args...)
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
	// Occurred: every log call, pre-filter/pre-sample (on the caller path). Written
	// is counted later in the bootstrap goroutine (single-writer, no contention).
	if l.occurredByLevel != nil {
		atomic.AddUint64(&l.occurredByLevel[level], 1)
	}
	var msg string
	var fileStr string

	if level > int(l.level.Load()) {
		return
	}
	// Per-request sampling verdict (cached at WithContext). A dropped request's
	// non-priority records never reach the writers. Records at or above
	// priorityLevel (e.g. ERROR) ALWAYS bypass sampling — the industry-standard
	// "error protection" pattern (errors kept for alerting even on sampled-out
	// requests). Default priorityLevel=-1 (no bypass).
	if l.sampleDrop.Load() && level > int(l.priorityLevel.Load()) {
		return
	}
	// Sampling runs before Metrics increment: a record dropped by the sampler
	// is never written and must not inflate the per-level counters (otherwise
	// monitoring would report a write rate the writers never see). nil sampler
	// is a no-op on the common path.
	if s := l.sampler.Load(); s != nil && !s.allow(level) {
		return
	}
	// Written (recordsByLevel) is now incremented in the bootstrap goroutine
	// (single-writer, no caller-path contention), not here.

	msg = f
	if sz := len(args); sz != 0 {
		if !(strings.Contains(msg, "%") && !strings.Contains(msg, "%%")) {
			msg += strings.Repeat("%v", sz)
		}
		msg = fmt.Sprintf(msg, args...) // skipped for the common no-args case (saves an alloc + fixes lone-% mangling)
	}

	// source code, file and line num. runtime.Callers returns ONLY the PC (no
	// file-string alloc, unlike runtime.Caller); the PC -> "file:line[ func]"
	// mapping is then resolved through callerFileLine, which memoizes it per call
	// site (a given PC is always the same source location). Steady state: 0 alloc
	// on the caller path (the file string + FuncForPC.Name run once per site).
	//
	// We walk the stack past log4go-internal frames (Info / deliverRecordToWriter
	// and any compiler-generated wrappers) to the first USER frame, instead of a
	// fixed skip count. A fixed skip is fragile: the number of intermediate
	// frames depends on the compiler's per-architecture inlining decisions, so
	// skip=3 lands on a log4go frame on some toolchains (observed on linux/amd64,
	// where it reported log.go instead of the caller). Walking past internal
	// frames is invariant to all such variation. callerIsInternal + callerFileLine
	// are both memoized per call site, so steady-state cost is a few map lookups.
	if l.hasCaller.Load() {
		var pcs [8]uintptr
		n := runtime.Callers(2, pcs[:]) // depth 1 = this func; scan from depth 2
		for i := 0; i < n; i++ {
			if callerIsInternal(pcs[i]) {
				continue
			}
			fileStr = callerFileLine(pcs[i], l.fullPath.Load(), l.withFuncName.Load())
			break
		}
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
	if bf := l.baseFields.v.Load(); bf != nil && len(*bf) > 0 {
		if len(l.fields) > 0 {
			merged := make([]field, 0, len(*bf)+len(l.fields))
			merged = append(merged, *bf...)      // base first (lowest priority)
			merged = append(merged, l.fields...) // logger fields override base
			r.fields = merged
		} else {
			r.fields = *bf
		}
	} else {
		r.fields = l.fields
	}

	// Pre-serialize once for FormatJSON / FormatLogfmt so every registered writer
	// emits the same bytes without re-serializing. For FormatText r.formattedBytes
	// stays nil and writers fall back to r.String(). Typed fields render directly
	// (no map), so base fields (merged into r.fields above) are included.
	switch LogFormat(l.format.Load()) {
	case FormatJSON:
		r.formattedBytes = r.JSON()
	case FormatLogfmt:
		r.formattedBytes = r.Logfmt()
	}

	l.enqueue(r)
}

// enqueue delivers r to the bootstrap goroutine. The send is blocking under
// normal load (plain back-pressure, identical to before), but it also selects on
// quit: when the logger is retired (Close/Reload closes quit), the quit case is
// ready and the record is dropped instead of racing a channel close. So a Reload
// racing traffic can neither panic (records is never closed) nor deadlock (quit
// always provides an exit), and is data-race-free. Lost records on retirement are
// expected — the logger is going away and the new one handles new traffic.
func (l *Logger) enqueue(r *Record) {
	select {
	case l.records <- r:
	case <-l.quit:
	}
}

func bootstrapLogWriter(logger *Logger) {
	// deliver writes one record to every registered writer, then returns it to
	// the pool with slice refs cleared so a long-lived fields slice or JSON buffer
	// is not pinned by a pooled record.
	deliver := func(r *Record) {
		for _, w := range logger.snapshotWriters() {
			if err := w.Write(r); err != nil {
				log.Printf("%v\n", err)
			}
		}
		// Written counter: incremented here (single-writer bootstrap goroutine)
		// to avoid atomic contention on the caller hot path. Records that were
		// filtered/sampled never reach here, so Written == actually delivered.
		if logger.recordsByLevel != nil {
			atomic.AddUint64(&logger.recordsByLevel[r.level], 1)
		}
		r.fields = nil
		r.formattedBytes = nil
		recordPool.Put(r)
	}
	// drainAndExit reaps any records buffered at retirement so a Reload/shutdown
	// does not lose in-flight records, then signals completion. New sends select
	// on quit and drop, so this only reaps what is already buffered.
	drainAndExit := func() {
		for {
			select {
			case r, ok := <-logger.records:
				if !ok {
					logger.c <- true
					return
				}
				deliver(r)
			default:
				logger.c <- true
				return
			}
		}
	}

	// Wait for the first record OR retirement (quit), so a logger retired before
	// any record arrives exits promptly. Either way, buffered records are drained
	// (never lost to a quit that races the first receive). (records may also be
	// closed by a legacy caller; that path is preserved.)
	select {
	case r, ok := <-logger.records:
		if !ok {
			logger.c <- true
			return
		}
		deliver(r)
	case <-logger.quit:
		drainAndExit()
		return
	}

	flushTimer := time.NewTimer(logger.flushTimer)
	rotateTimer := time.NewTimer(logger.rotateTimer)
	defer flushTimer.Stop()
	defer rotateTimer.Stop()

	for {
		select {
		case r, ok := <-logger.records:
			if !ok {
				logger.c <- true
				return
			}
			deliver(r)

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

		case <-logger.quit:
			drainAndExit()
			return
		}
	}
}

func init() {
	recordPool = &sync.Pool{New: func() interface{} {
		return &Record{}
	}}
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

// Runtime returns the hot, lock-free configuration surface (RuntimeConfig) of the
// package singleton, for live non-destructive toggles under any load.
func Runtime() RuntimeConfig { return defaultLogger().Runtime() }

// SetSamplingStrategy installs an id-based sampling policy on the package
// singleton (nil ⇒ FullSampling / keep all). See Logger.SetSamplingStrategy.
func SetSamplingStrategy(s SamplingStrategy) { defaultLogger().SetSamplingStrategy(s) }

// SetPriorityLevel sets the error-protection threshold on the singleton.
// See Logger.SetPriorityLevel.
func SetPriorityLevel(level int) { defaultLogger().SetPriorityLevel(level) }

// SetSamplingStrategyFor temporarily installs s on the singleton for duration,
// then reverts. See Logger.SetSamplingStrategyFor.
func SetSamplingStrategyFor(s SamplingStrategy, duration time.Duration) (stop func()) {
	return defaultLogger().SetSamplingStrategyFor(s, duration)
}

// Status returns a runtime snapshot of the package singleton (active sampling
// strategy + per-writer state). See Logger.Status.
func Status() RuntimeStatus { return defaultLogger().Status() }

// Writers returns a snapshot of the writers registered on the package singleton.
func Writers() []Writer { return defaultLogger().Writers() }

// PauseWriter pauses the named writer on the package singleton (e.g.
// log4go.PauseWriter(log4go.WriterNameKafka) to silence kafka during an incident).
// Returns true if a writer was paused.
func PauseWriter(name string) bool { return defaultLogger().PauseWriter(name) }

// ResumeWriter resumes the named writer on the package singleton.
func ResumeWriter(name string) bool { return defaultLogger().ResumeWriter(name) }

// WriterPaused reports whether the named writer on the package singleton is paused.
func WriterPaused(name string) bool { return defaultLogger().WriterPaused(name) }

// SetKafkaCodec applies c to every registered KafKaWriter on the package
// singleton (nil ⇒ JSON, the default). Use at startup to choose the on-the-wire
// Kafka payload format (KafkaCodecProto for ~46% smaller payloads at 1M+ QPS),
// or for a rare runtime format switch. Non-Kafka writers are ignored. Each
// writer's codec swap is RWMutex-protected, so this is safe under load.
func SetKafkaCodec(c KafkaCodec) {
	for _, w := range Writers() {
		if kw, ok := w.(*KafKaWriter); ok {
			kw.SetKafkaCodec(c)
		}
	}
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

// WithAttrs returns a child Logger of the package singleton carrying the given
// typed Fields (allocation-free for scalars). See Logger.WithAttrs.
func WithAttrs(attrs ...Field) *Logger { return defaultLogger().WithAttrs(attrs...) }

// WithString/WithInt/... are the typed, allocation-free variants of With on the
// package singleton (see the Logger methods of the same name).
func WithString(key, val string) *Logger { return defaultLogger().WithString(key, val) }

// WithInt returns a child Logger of the package singleton carrying an int-typed
// structured field (see Logger.WithInt).
func WithInt(key string, val int) *Logger { return defaultLogger().WithInt(key, val) }

// WithInt64 returns a child Logger of the package singleton carrying an int64-typed
// structured field (see Logger.WithInt64).
func WithInt64(key string, val int64) *Logger { return defaultLogger().WithInt64(key, val) }

// WithBool returns a child Logger of the package singleton carrying a bool-typed
// structured field (see Logger.WithBool).
func WithBool(key string, val bool) *Logger { return defaultLogger().WithBool(key, val) }

// WithFloat64 returns a child Logger of the package singleton carrying a
// float64-typed structured field (see Logger.WithFloat64).
func WithFloat64(key string, val float64) *Logger { return defaultLogger().WithFloat64(key, val) }

// WithDuration returns a child Logger of the package singleton carrying a
// duration-typed structured field (see Logger.WithDuration).
func WithDuration(key string, val time.Duration) *Logger {
	return defaultLogger().WithDuration(key, val)
}

// WithTime returns a child Logger of the package singleton carrying a time-typed
// structured field (see Logger.WithTime).
func WithTime(key string, val time.Time) *Logger { return defaultLogger().WithTime(key, val) }

// WithBytes returns a child Logger of the package singleton carrying a bytes-typed
// structured field (see Logger.WithBytes).
func WithBytes(key string, val []byte) *Logger { return defaultLogger().WithBytes(key, val) }

// WithError returns a child Logger of the package singleton carrying an
// error-typed structured field (see Logger.WithError).
func WithError(key string, val error) *Logger { return defaultLogger().WithError(key, val) }

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

// Trace level trace — finest granularity, below DEBUG.
func Trace(fmt string, args ...interface{}) {
	defaultLogger().deliverRecordToWriter(TRACE, fmt, args...)
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
