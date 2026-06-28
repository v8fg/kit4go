package log4go

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// Test_Field_TypedValueRoundTrip checks each kind round-trips through value()
// back to its original Go type (so FieldValue returns what With put in).
func Test_Field_TypedValueRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		f    field
		want interface{}
	}{
		{"string", strField("k", "v"), "v"},
		{"int", intField("k", 42), 42},
		{"int64", int64Field("k", 99), int64(99)},
		{"uint64", uint64Field("k", 7), uint64(7)},
		{"bool", boolField("k", true), true},
		{"float64", floatField("k", 1.5), 1.5},
		{"duration", durField("k", 5*time.Second), 5 * time.Second},
		{"error", errField("k", errors.New("boom")), errors.New("boom")},
		{"any", anyField("k", "x"), "x"},
	}
	for _, c := range cases {
		got := c.f.value()
		// error/[]int compare by string form (non-comparable without reflect)
		if g, ok := got.(error); ok && c.name == "error" {
			if g.Error() != "boom" {
				t.Errorf("%s: error=%q want boom", c.name, g.Error())
			}
			continue
		}
		if got != c.want {
			t.Errorf("%s: value=%v(%T) want %v(%T)", c.name, got, got, c.want, c.want)
		}
	}
}

// Test_Field_JSONEncoding verifies every kind renders valid JSON via the typed
// append path (no map, no reflection), and that scalars serialize to the
// expected literal form.
func Test_Field_JSONEncoding(t *testing.T) {
	fields := []field{
		strField("s", `hi "x"`),
		intField("i", 42),
		int64Field("i64", 99),
		uint64Field("u", 7),
		boolField("b", true),
		floatField("f", 1.5),
		durField("d", 2*time.Second),
		errField("e", errors.New("boom")),
		anyField("a", map[string]int{"x": 1}),
	}
	buf := appendFieldsJSONObject([]byte{}, fields)
	var m map[string]interface{}
	if err := json.Unmarshal(buf, &m); err != nil {
		t.Fatalf("typed fields produced invalid JSON: %v\n%s", err, buf)
	}
	if m["s"] != `hi "x"` {
		t.Errorf(`s=%v want 'hi "x"' (escaping)`, m["s"])
	}
	if m["i"] != float64(42) {
		t.Errorf("i=%v want 42", m["i"])
	}
	if m["i64"] != float64(99) {
		t.Errorf("i64=%v want 99", m["i64"])
	}
	if m["b"] != true {
		t.Errorf("b=%v want true", m["b"])
	}
	if m["f"] != 1.5 {
		t.Errorf("f=%v want 1.5", m["f"])
	}
	if m["d"] != float64(2_000_000_000) {
		t.Errorf("d=%v want 2e9 nanos", m["d"])
	}
	if m["e"] != "boom" {
		t.Errorf("e=%v want boom", m["e"])
	}
	am, _ := m["a"].(map[string]interface{})
	if am["x"] != float64(1) {
		t.Errorf("a.x=%v want 1", am["x"])
	}
}

// Test_Field_OfStringEscape verifies control chars and quotes are escaped per
// RFC 8259 (so a log message can never break the JSON document).
func Test_Field_StringEscape(t *testing.T) {
	buf := appendFieldJSON([]byte{}, strField("k", "a\"b\nc\x01d"))
	s := string(buf)
	if !strings.HasPrefix(s, `"k":"a\"b\nc`) {
		t.Errorf("quote/newline not escaped: %q", s)
	}
	if !strings.Contains(s, "\\u0001") {
		t.Errorf("control char not \\u-escaped: %q", s)
	}
}

// Test_FieldOf_Inference verifies the common Go types map to a typed (non-Any)
// kind, so With(key, interface{}) stays allocation-free for scalars.
func Test_FieldOf_Inference(t *testing.T) {
	cases := []struct {
		v    interface{}
		kind fieldKind
	}{
		{"s", kindString},
		{42, kindInt},
		{int64(1), kindInt64},
		{uint64(1), kindUint},
		{true, kindBool},
		{1.5, kindFloat64},
		{time.Second, kindDuration},
		{time.Now(), kindTime},
		{errors.New("e"), kindError},
		{[]int{1}, kindAny},
	}
	for _, c := range cases {
		f := fieldOf("k", c.v)
		if f.kind != c.kind {
			t.Errorf("fieldOf(%T) kind=%v want %v", c.v, f.kind, c.kind)
		}
	}
}
