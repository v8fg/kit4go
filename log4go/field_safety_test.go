package log4go

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"testing"
	"time"
)

// panicMarshaler implements json.Marshaler and panics. A field holding it must
// NEVER crash the log pipeline — it degrades to null.
type panicMarshaler struct{}

func (panicMarshaler) MarshalJSON() ([]byte, error) { panic("boom from MarshalJSON") }

// nilRecvError is a typed-nil error: a nil pointer with an Error method. Calling
// .Error() on it panics (nil receiver); the field must degrade, not crash.
type nilRecvError struct{}

func (*nilRecvError) Error() string { panic("nil receiver") }

// Test_Field_AnyMarshalPanic verifies a panicking MarshalJSON degrades to null
// in BOTH JSON and logfmt output, never propagating the panic.
func Test_Field_AnyMarshalPanic(t *testing.T) {
	f := anyField("k", panicMarshaler{})

	jb := appendFieldJSON([]byte{}, f)
	if got := string(jb); got != `"k":null` {
		t.Errorf("JSON on panic = %q, want \"k\":null", got)
	}
	lb := appendFieldLogfmt([]byte{}, f)
	if got := string(lb); got != ` k=-` {
		t.Errorf("logfmt on panic = %q, want ' k=-'", got)
	}
}

// Test_Field_NaNInfIsValidJSON verifies NaN/±Inf render as null (valid JSON),
// not the invalid literals "NaN"/"+Inf".
func Test_Field_NaNInfIsValidJSON(t *testing.T) {
	for _, bits := range []uint64{
		math.Float64bits(math.NaN()),
		math.Float64bits(math.Inf(1)),
		math.Float64bits(math.Inf(-1)),
	} {
		f := field{key: "k", kind: kindFloat64, i: int64(bits)}
		jb := appendFieldJSON([]byte{}, f)
		var m map[string]json.RawMessage
		wrapped := []byte("{" + string(jb) + "}")
		if err := json.Unmarshal(wrapped, &m); err != nil {
			t.Errorf("NaN/Inf produced invalid JSON %s: %v", jb, err)
		}
		if string(m["k"]) != "null" {
			t.Errorf("NaN/Inf = %s, want null", m["k"])
		}
	}
}

// Test_Field_TypedNilErrorDegrades verifies a typed-nil error does not crash.
func Test_Field_TypedNilErrorDegrades(t *testing.T) {
	var e *nilRecvError // typed nil; e.Error() panics
	f := errField("k", e)

	jb := appendFieldJSON([]byte{}, f)
	if got := string(jb); got != `"k":null` {
		t.Errorf("JSON on typed-nil error = %q, want \"k\":null", got)
	}
	lb := appendFieldLogfmt([]byte{}, f)
	if got := string(lb); got != ` k=-` {
		t.Errorf("logfmt on typed-nil error = %q, want ' k=-'", got)
	}
}

// Test_Field_UnmarshallableKinds verifies chan/func (JSON cannot encode them)
// degrade safely to null in JSON (and '-' in logfmt) without panicking.
func Test_Field_UnmarshallableKinds(t *testing.T) {
	for _, v := range []interface{}{make(chan int), func() {}} {
		f := anyField("k", v)
		jb := appendFieldJSON([]byte{}, f)
		if got := string(jb); got != `"k":null` {
			t.Errorf("JSON on %T = %q, want \"k\":null", v, got)
		}
	}
}

// Test_Field_BytesBase64 verifies []byte round-trips as base64 (JSON standard)
// in both directions: serialize as base64, value() decodes back to the bytes.
func Test_Field_BytesBase64(t *testing.T) {
	orig := []byte("hello\x00bytes")
	f := bytesField("k", orig)

	want := base64.StdEncoding.EncodeToString(orig)
	if f.str != want {
		t.Errorf("bytesField str=%q want %q", f.str, want)
	}
	jb := appendFieldJSON([]byte{}, f)
	var m map[string]string
	if err := json.Unmarshal([]byte("{"+string(jb)+"}"), &m); err != nil {
		t.Fatalf("invalid JSON: %v %s", err, jb)
	}
	if m["k"] != want {
		t.Errorf("JSON bytes = %q want %q", m["k"], want)
	}
	// value() decodes back
	got, ok := f.value().([]byte)
	if !ok {
		t.Fatalf("value() not []byte: %T", f.value())
	}
	if string(got) != string(orig) {
		t.Errorf("decoded bytes = %q want %q", got, orig)
	}
}

// Test_FieldOf_UintptrAndBytes verifies fieldOf maps uintptr and []byte to typed
// kinds (not kindAny), so they stay allocation-free / deterministic.
func Test_FieldOf_UintptrAndBytes(t *testing.T) {
	if f := fieldOf("k", uintptr(42)); f.kind != kindUint {
		t.Errorf("uintptr kind=%v want kindUint", f.kind)
	}
	if f := fieldOf("k", []byte("x")); f.kind != kindBytes {
		t.Errorf("[]byte kind=%v want kindBytes", f.kind)
	}
}

// Test_Logger_WithBytes verifies the WithBytes typed API attaches a []byte field
// that renders as base64 end-to-end.
func Test_Logger_WithBytes(t *testing.T) {
	lg := newLoggerWithRecords(make(chan *Record, 4))
	cw := &captureWriter{}
	lg.Register(cw)
	lg.SetLevel(DEBUG)

	lg.WithBytes("raw", []byte("abc")).Info("got bytes")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
	}
	if cw.Len() == 0 {
		t.Fatal("record never reached writer")
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()
	var found bool
	for _, f := range r.fields {
		if f.key == "raw" && f.kind == kindBytes {
			found = true
		}
	}
	if !found {
		t.Error("WithBytes field not attached as kindBytes")
	}
}

// Test_Field_Complex verifies complex128/64 serialize as "a+bi" strings (JSON
// has no complex type) and never become null (the data-loss bug that motivated
// this kind), including NaN-component safety.
func Test_Field_Complex(t *testing.T) {
	cases := []struct {
		c    complex128
		want string
	}{
		{1 + 2i, "1+2i"},
		{3 + 0i, "3+0i"},
		{0 + -4i, "0-4i"},
		{complex(math.NaN(), 1), "null"}, // NaN component -> null, not invalid JSON
	}
	for _, c := range cases {
		f := fieldOf("k", c.c)
		if f.kind != kindString {
			t.Errorf("complex %v kind=%v want kindString (reuse)", c.c, f.kind)
		}
		if f.str != c.want {
			t.Errorf("complex %v -> %q want %q", c.c, f.str, c.want)
		}
		// JSON output is a valid quoted string
		jb := appendFieldJSON([]byte{}, f)
		var m map[string]string
		if err := json.Unmarshal([]byte("{"+string(jb)+"}"), &m); err != nil {
			t.Errorf("complex JSON invalid: %v %s", err, jb)
		}
	}
}
