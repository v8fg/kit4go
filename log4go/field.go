package log4go

import (
	"encoding/base64"
	"math"
	"strconv"
	"sync/atomic"
	"time"
)

// fieldKind enumerates the concrete value type stored in a field, so the
// serialization hot path can render it without reflection or any boxing
// — the same technique zap.Field / slog.Attr use to stay allocation-free.
type fieldKind uint8

const (
	kindString   fieldKind = iota
	kindInt                // int (stored in i; value() -> int)
	kindInt64              // int64 (stored in i; value() -> int64)
	kindUint               // uint64 (stored in i as int64)
	kindFloat64            // float64 bits stored in i
	kindBool               // 0/1 stored in i
	kindDuration           // int64 nanos stored in i
	kindTime               // int64 unix-nanos stored in i
	kindError              // error stored in any
	kindBytes              // []byte base64-encoded into str (JSON convention)
	kindAny                // arbitrary value stored in any (rendered via the active codec)
)

// field is the internal structured key/value pair. The value is stored unboxed
// across (kind, i, str, any) so attaching a scalar field never allocates a
// boxed any — the allocation that the old `val any` design
// paid on every With call. field is cheap to pass by value.
type field struct {
	key  string
	kind fieldKind
	i    int64
	str  string
	any  any
}

// --- internal constructors (zero boxing for scalars) ---

func strField(k, v string) field           { return field{key: k, kind: kindString, str: v} }
func intField(k string, v int) field       { return field{key: k, kind: kindInt, i: int64(v)} }
func int64Field(k string, v int64) field   { return field{key: k, kind: kindInt64, i: v} }
func uint64Field(k string, v uint64) field { return field{key: k, kind: kindUint, i: int64(v)} }
func boolField(k string, v bool) field {
	if v {
		return field{key: k, kind: kindBool, i: 1}
	}
	return field{key: k, kind: kindBool, i: 0}
}
func floatField(k string, v float64) field {
	return field{key: k, kind: kindFloat64, i: int64(math.Float64bits(v))}
}
func durField(k string, v time.Duration) field { return field{key: k, kind: kindDuration, i: int64(v)} }
func timeField(k string, v time.Time) field    { return field{key: k, kind: kindTime, i: v.UnixNano()} }
func errField(k string, v error) field         { return field{key: k, kind: kindError, any: v} }
func bytesField(k string, v []byte) field {
	return field{key: k, kind: kindBytes, str: base64.StdEncoding.EncodeToString(v)}
}
func anyField(k string, v any) field { return field{key: k, kind: kindAny, any: v} }

// fieldOf builds a typed field from an any value, mapping the common
// scalar types to their unboxed kind and falling back to kindAny for the rest.
// This keeps the existing With(key, val any) API allocation-free for
// scalars while remaining fully backward compatible.
func fieldOf(key string, v any) field {
	switch x := v.(type) {
	case nil:
		return field{key: key, kind: kindAny, any: nil}
	case string:
		return strField(key, x)
	case bool:
		return boolField(key, x)
	case int:
		return intField(key, x)
	case int64:
		return int64Field(key, x)
	case int32:
		return int64Field(key, int64(x))
	case int16:
		return int64Field(key, int64(x))
	case int8:
		return int64Field(key, int64(x))
	case uint:
		return uint64Field(key, uint64(x))
	case uint64:
		return uint64Field(key, x)
	case uint32:
		return uint64Field(key, uint64(x))
	case uint16:
		return uint64Field(key, uint64(x))
	case uint8:
		return uint64Field(key, uint64(x))
	case float64:
		return floatField(key, x)
	case float32:
		return floatField(key, float64(x))
	case time.Duration:
		return durField(key, x)
	case time.Time:
		return timeField(key, x)
	case []byte:
		return bytesField(key, x)
	case uintptr:
		return uint64Field(key, uint64(x))
	case complex128:
		return strField(key, complex128ToString(x))
	case complex64:
		return strField(key, complex128ToString(complex128(x)))
	case error:
		return errField(key, x)
	default:
		return anyField(key, v)
	}
}

// value materializes the field value as an any (for FieldValue and any
// legacy consumer that wants the boxed form). kindUint beyond int63 is lossy.
func (f field) value() any {
	switch f.kind {
	case kindString:
		return f.str
	case kindInt:
		return int(f.i)
	case kindInt64:
		return f.i
	case kindUint:
		return uint64(f.i)
	case kindBool:
		return f.i == 1
	case kindFloat64:
		return math.Float64frombits(uint64(f.i))
	case kindDuration:
		return time.Duration(f.i)
	case kindTime:
		return time.Unix(0, f.i)
	case kindBytes:
		if b, err := base64.StdEncoding.DecodeString(f.str); err == nil {
			return b
		}
		return nil
	default: // kindError / kindAny
		return f.any
	}
}

// --- public typed API ---

// FieldKind is the public mirror of the internal field kind.
type FieldKind = fieldKind

// Public FieldKind constants.
const (
	FieldKindString   = kindString
	FieldKindInt      = kindInt
	FieldKindInt64    = kindInt64
	FieldKindUint     = kindUint
	FieldKindFloat64  = kindFloat64
	FieldKindBool     = kindBool
	FieldKindDuration = kindDuration
	FieldKindTime     = kindTime
	FieldKindError    = kindError
	FieldKindBytes    = kindBytes
	FieldKindAny      = kindAny
)

// Field is the public typed key/value pair. Construct it with the String / Int /
// ... constructors and attach it via Logger.WithAttrs. It wraps the internal
// field so the hot path stays allocation-free.
type Field struct{ f field }

// String constructs a string-typed field for use with WithAttrs and slog
// interop. It never boxes scalars.
//
// The typed constructors below are the allocation-free variants of With on the
// package singleton; prefer them over With(key, any) on hot paths.
func String(k, v string) Field { return Field{strField(k, v)} }

// Int constructs an int-typed field (no any boxing).
func Int(k string, v int) Field { return Field{intField(k, v)} }

// Int64 constructs an int64-typed field (no any boxing).
func Int64(k string, v int64) Field { return Field{int64Field(k, v)} }

// Uint64 constructs a uint64-typed field (no any boxing).
func Uint64(k string, v uint64) Field { return Field{uint64Field(k, v)} }

// Bool constructs a bool-typed field (no any boxing).
func Bool(k string, v bool) Field { return Field{boolField(k, v)} }

// Float64 constructs a float64-typed field (no any boxing).
func Float64(k string, v float64) Field { return Field{floatField(k, v)} }

// Duration constructs a duration-typed field rendered as nanoseconds (slog convention).
func Duration(k string, v time.Duration) Field { return Field{durField(k, v)} }

// Time constructs a time-typed field rendered as an RFC3339 UTC timestamp.
func Time(k string, v time.Time) Field { return Field{timeField(k, v)} }

// Bytes constructs a bytes-typed field; the value is base64-encoded on the JSON
// path (the JSON convention for binary data).
func Bytes(k string, v []byte) Field { return Field{bytesField(k, v)} }

// Complex128 constructs a complex128-typed field rendered as the conventional
// "a+bi" string (JSON has no complex type). NaN/±Inf components render as "null".
func Complex128(k string, v complex128) Field { return Field{strField(k, complex128ToString(v))} }

// Complex64 constructs a complex64-typed field; see Complex128 for the format.
func Complex64(k string, v complex64) Field {
	return Field{strField(k, complex128ToString(complex128(v)))}
}

// ErrorField constructs an error-typed field. (Named ErrorField because Error
// is already the package-level ERROR log helper.)
func ErrorField(k string, v error) Field { return Field{errField(k, v)} }

// Any constructs an arbitrary-typed field; the value is rendered via the active
// JSON codec. Prefer the typed constructors for scalars (allocation-free).
func Any(k string, v any) Field { return Field{anyField(k, v)} }

// Key returns the field key.
func (f Field) Key() string { return f.f.key }

// Kind returns the field kind.
func (f Field) Kind() FieldKind { return f.f.kind }

// Value returns the field value as an any (materialized via field.value).
func (f Field) Value() any { return f.f.value() }

// --- typed serialization (zero reflection, zero boxing) ---

// hexDigit maps a nibble to its lowercase hex char for \u escape encoding.
var hexDigit = []byte("0123456789abcdef")

// floatExpMask is the IEEE-754 exponent bits of a float64; a value whose
// exponent bits all equal 1 is NaN or ±Inf (not valid JSON), used by the
// one-instruction NaN/Inf check on the float hot path.
const floatExpMask uint64 = 0x7FF0000000000000

// appendJSONStringContent appends the escaped contents of s (no surrounding
// quotes), per RFC 8259. ASCII control chars become \uXXXX; the common escapes
// use their short forms. Clean runs (no special char) are appended in one slice
// append rather than byte-by-byte, so the common message/key with no quotes or
// control chars pays a single append instead of len(s) of them.
func appendJSONStringContent(buf []byte, s string) []byte {
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c != '"' && c != '\\' {
			continue // extend the clean run
		}
		if start < i { // flush the clean run [start:i] in one append
			buf = append(buf, s[start:i]...)
		}
		switch c {
		case '"', '\\':
			buf = append(buf, '\\', c)
		case '\n':
			buf = append(buf, '\\', 'n')
		case '\r':
			buf = append(buf, '\\', 'r')
		case '\t':
			buf = append(buf, '\\', 't')
		case '\b':
			buf = append(buf, '\\', 'b')
		case '\f':
			buf = append(buf, '\\', 'f')
		default: // c < 0x20
			buf = append(buf, '\\', 'u', '0', '0', hexDigit[c>>4], hexDigit[c&0xf])
		}
		start = i + 1
	}
	if start < len(s) {
		buf = append(buf, s[start:]...)
	}
	return buf
}

// appendJSONQuoted appends a JSON-quoted, escaped string.
func appendJSONQuoted(buf []byte, s string) []byte {
	buf = append(buf, '"')
	buf = appendJSONStringContent(buf, s)
	return append(buf, '"')
}

// appendFieldJSON appends "key":<value> for a typed field. Scalars render
// directly (no map, no reflection); kindAny falls back to the active JSON codec.
// Never allocates for scalars.
func appendFieldJSON(buf []byte, f field) []byte {
	buf = append(buf, '"')
	buf = appendJSONStringContent(buf, f.key)
	buf = append(buf, '"', ':')
	switch f.kind {
	case kindString:
		buf = appendJSONQuoted(buf, f.str)
	case kindInt, kindInt64:
		buf = strconv.AppendInt(buf, f.i, 10)
	case kindUint:
		buf = strconv.AppendUint(buf, uint64(f.i), 10)
	case kindBool:
		buf = strconv.AppendBool(buf, f.i == 1)
	case kindFloat64:
		// NaN/±Inf (IEEE-754 exponent all-ones) are not valid JSON -> null. A
		// single bitmask test is cheaper than Float64frombits + IsNaN + IsInf.
		if uint64(f.i)&floatExpMask == floatExpMask {
			buf = append(buf, 'n', 'u', 'l', 'l')
		} else {
			buf = strconv.AppendFloat(buf, math.Float64frombits(uint64(f.i)), 'f', -1, 64)
		}
	case kindDuration:
		buf = strconv.AppendInt(buf, f.i, 10) // nanoseconds (slog convention)
	case kindTime:
		buf = append(buf, '"')
		buf = appendISOTimeUTC(buf, f.i)
		buf = append(buf, '"')
	case kindBytes:
		buf = appendJSONQuoted(buf, f.str) // f.str holds the base64 text
	case kindError:
		if e, ok := f.any.(error); ok {
			if s, ok := safeErrorString(e); ok {
				buf = appendJSONQuoted(buf, s)
			} else {
				buf = append(buf, 'n', 'u', 'l', 'l')
			}
		} else {
			buf = append(buf, 'n', 'u', 'l', 'l')
		}
	case kindAny:
		if vb, ok := safeJSONMarshal(f.any); ok {
			buf = append(buf, vb...)
		} else {
			buf = append(buf, 'n', 'u', 'l', 'l')
		}
	}
	return buf
}

// marshalPanics counts field-marshal / error-string panics recovered on the
// render hot path. Exposed via RuntimeStats().MarshalPanics so a recurring
// panic (a buggy MarshalJSON, a typed-nil receiver) is observable instead of
// silently turning the field into null on every record.
var marshalPanics uint64

// safeJSONMarshal marshals v via the active codec, recovering from any panic
// (a custom MarshalJSON that panics, a typed-nil receiver, or a codec-internal
// panic). ok=false on error OR panic so callers emit null — a field value must
// never crash the log pipeline. A recovered panic increments marshalPanics so
// the silent-to-null degradation is visible to monitoring (L5: observable
// degradation).
func safeJSONMarshal(v any) (b []byte, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			atomic.AddUint64(&marshalPanics, 1)
		}
	}()
	b, err := jsonMarshalEncode(v)
	return b, err == nil
}

// safeErrorString returns e.Error() without panicking (a nil-receiver method on
// a typed-nil error panics; this guards it). A recovered panic increments
// marshalPanics.
func safeErrorString(e error) (s string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			atomic.AddUint64(&marshalPanics, 1)
		}
	}()
	return e.Error(), true
}

// appendISOTimeUTC appends the UTC RFC3339-with-micros timestamp
// "2006-01-02T15:04:05.000000Z" for unixNano DIRECTLY into buf, with no string
// allocation (unlike time.Format, which builds an intermediate string then has
// it copied in). The date math uses time.Unix(sec,nsec).UTC().Date()/Clock(),
// which are allocation-free. This is the JSON/logfmt time hot path.
func appendISOTimeUTC(buf []byte, unixNano int64) []byte {
	t := time.Unix(unixNano/1e9, unixNano%1e9).UTC()
	year, month, day := t.Date()
	hour, minute, sec := t.Clock()
	micros := t.Nanosecond() / 1000
	buf = append4d(buf, year)
	buf = append(buf, '-')
	buf = append2d(buf, int(month))
	buf = append(buf, '-')
	buf = append2d(buf, day)
	buf = append(buf, 'T')
	buf = append2d(buf, hour)
	buf = append(buf, ':')
	buf = append2d(buf, minute)
	buf = append(buf, ':')
	buf = append2d(buf, sec)
	buf = append(buf, '.')
	buf = append6d(buf, micros)
	buf = append(buf, 'Z')
	return buf
}

// append4d/append2d/append6d write zero-padded decimal digits with no branching
// (year is 1000-9999; month/day/hour/min/sec < 100; micros < 1e6).
func append4d(buf []byte, v int) []byte {
	return append(buf,
		byte('0'+v/1000),
		byte('0'+(v/100)%10),
		byte('0'+(v/10)%10),
		byte('0'+v%10))
}

func append2d(buf []byte, v int) []byte {
	return append(buf, byte('0'+v/10), byte('0'+v%10))
}

func append6d(buf []byte, v int) []byte {
	return append(buf,
		byte('0'+(v/100000)%10),
		byte('0'+(v/10000)%10),
		byte('0'+(v/1000)%10),
		byte('0'+(v/100)%10),
		byte('0'+(v/10)%10),
		byte('0'+v%10))
}

// complex128ToString renders a complex128 as the conventional "a+bi" string (the
// form zap/zerolog use, since JSON has no complex type). NaN/±Inf in either
// component render as "null" so the field is never lost or invalid.
func complex128ToString(c complex128) string {
	r, im := real(c), imag(c)
	if math.IsNaN(r) || math.IsInf(r, 0) || math.IsNaN(im) || math.IsInf(im, 0) {
		return "null"
	}
	rs := strconv.FormatFloat(r, 'g', -1, 64)
	is := strconv.FormatFloat(im, 'g', -1, 64) // includes sign for negatives
	if im >= 0 {
		return rs + "+" + is + "i"
	}
	return rs + is + "i" // is already starts with '-'
}

// appendFieldsJSONObject appends {...} of the fields joined by commas.
func appendFieldsJSONObject(buf []byte, fields []field) []byte {
	buf = append(buf, '{')
	for i := range fields {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = appendFieldJSON(buf, fields[i])
	}
	return append(buf, '}')
}
