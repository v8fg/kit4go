package log4go

// This file exposes read-only accessors for Record fields so that code in other
// packages (custom WebhookWriter filters/formatters, monitoring hooks) can
// inspect a record without reaching into its private fields. The record is
// immutable once delivered to a writer, so these are safe to call concurrently.

// Msg returns the formatted message text.
func (r *Record) Msg() string { return r.msg }

// TimeStr returns the formatted timestamp string (the logger's layout, or the
// ISO layout for FormatJSON records).
func (r *Record) TimeStr() string { return r.time }

// FileLine returns the source location "file.go:line" (with optional func name
// when WithFuncName is on), or "" when caller capture is disabled
// (WithCaller(false)).
func (r *Record) FileLine() string { return r.file }

// LevelName returns the level flag string (e.g. "ERROR", "INFO").
func (r *Record) LevelName() string { return LevelFlags[r.level] }

// LevelInt returns the numeric level (EMERGENCY=0 … DEBUG=7).
func (r *Record) LevelInt() int { return r.level }

// UnixNano returns the wall-clock nanosecond timestamp captured for the record
// (the ES/strict-ordering primary context key).
func (r *Record) UnixNano() int64 { return r.unixNano }

// Seq returns the process-global monotonic sequence number (strict-ordering
// tie-break across partitions/cores).
func (r *Record) Seq() uint64 { return r.seq }

// FieldValue returns the value of the first structured field named key, and
// whether such a field exists. Base Fields (SetBaseField), With/WithFields and
// context-extracted fields are all merged into the record, so this sees every
// attached field. Useful in custom WebhookWriter filters/formatters.
func (r *Record) FieldValue(key string) (interface{}, bool) {
	for _, f := range r.fields {
		if f.key == key {
			return f.value(), true
		}
	}
	return nil, false
}
