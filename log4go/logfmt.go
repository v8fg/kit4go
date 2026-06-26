package log4go

import (
	"math"
	"strconv"
)

// appendLogfmtValue appends a logfmt value, quoting+escaping it when it contains
// a space, '=', '"', backslash, control char, or non-ASCII byte (per the logfmt
// convention used by Loki/Promtail). Empty values are quoted ("").
func appendLogfmtValue(buf []byte, s string) []byte {
	needsQuote := s == ""
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= ' ' || c == '=' || c == '"' || c == '\\' || c > 0x7e {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return append(buf, s...)
	}
	buf = append(buf, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"', '\\':
			buf = append(buf, '\\', c)
		case '\n':
			buf = append(buf, '\\', 'n')
		case '\r':
			buf = append(buf, '\\', 'r')
		case '\t':
			buf = append(buf, '\\', 't')
		default:
			if c < 0x20 {
				buf = append(buf, '\\', 'x', hexDigit[c>>4], hexDigit[c&0xf])
			} else {
				buf = append(buf, c)
			}
		}
	}
	return append(buf, '"')
}

// appendFieldLogfmt appends " key=value" for a typed field. Scalars render
// directly (no reflection); kindAny falls back to the active codec.
func appendFieldLogfmt(buf []byte, f field) []byte {
	buf = append(buf, ' ')
	buf = appendLogfmtValue(buf, f.key)
	buf = append(buf, '=')
	switch f.kind {
	case kindString:
		buf = appendLogfmtValue(buf, f.str)
	case kindInt, kindInt64:
		buf = strconv.AppendInt(buf, f.i, 10)
	case kindUint:
		buf = strconv.AppendUint(buf, uint64(f.i), 10)
	case kindBool:
		buf = strconv.AppendBool(buf, f.i == 1)
	case kindFloat64:
		// NaN/±Inf (exponent all-ones) render as '-' (no valid logfmt number).
		if uint64(f.i)&floatExpMask == floatExpMask {
			buf = append(buf, '-')
		} else {
			buf = strconv.AppendFloat(buf, math.Float64frombits(uint64(f.i)), 'f', -1, 64)
		}
	case kindDuration:
		buf = strconv.AppendInt(buf, f.i, 10) // nanoseconds
	case kindTime:
		buf = appendISOTimeUTC(buf, f.i) // ISO time has no logfmt-special chars -> bare
	case kindBytes:
		buf = appendLogfmtValue(buf, f.str) // base64
	case kindError:
		if e, ok := f.any.(error); ok {
			if s, ok := safeErrorString(e); ok {
				buf = appendLogfmtValue(buf, s)
			} else {
				buf = append(buf, '-')
			}
		} else {
			buf = append(buf, '-')
		}
	case kindAny:
		if vb, ok := safeJSONMarshal(f.any); ok {
			buf = appendLogfmtValue(buf, string(vb))
		} else {
			buf = append(buf, '-')
		}
	}
	return buf
}

// Logfmt renders the record as one space-separated key=value line terminated by
// a newline:
//
//	time=<iso> level=<LEVEL> msg=<msg> [file=<file>] <key=value ...>
//
// the format Loki/Promtail/docker consume natively. Strings that need quoting
// (spaces, '=', '"', control chars) are quoted and escaped. Typed scalars never
// reach the JSON codec.
func (r *Record) Logfmt() []byte {
	buf := make([]byte, 0, 128+len(r.fields)*16)
	buf = append(buf, "time="...)
	buf = appendISOTimeUTC(buf, r.unixNano)
	buf = append(buf, " level="...)
	buf = appendLogfmtValue(buf, LevelFlags[r.level])
	buf = append(buf, " msg="...)
	buf = appendLogfmtValue(buf, r.msg)
	if r.file != "" {
		buf = append(buf, " file="...)
		buf = appendLogfmtValue(buf, r.file)
	}
	for _, f := range r.fields {
		buf = appendFieldLogfmt(buf, f)
	}
	return append(buf, '\n')
}
