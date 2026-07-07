package log4go

// fld is a test-only helper that builds a typed field from an any value,
// mirroring fieldOf's type inference, so tests read naturally:
//
//	fld("trace_id", "abc")  // -> kindString
//	fld("user", 42)         // -> kindInt
//
// and serialize the same way production With(key, val) does.
func fld(key string, val any) field { return fieldOf(key, val) }
