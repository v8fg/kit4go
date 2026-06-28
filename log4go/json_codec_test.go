package log4go

import (
	"encoding/json"
	"strings"
	"testing"
)

// Test_JSONCodec_DefaultIsGoccy confirms the package default is goccy (the
// fastest portable option) and SetJSONCodec switches it.
func Test_JSONCodec_DefaultIsGoccy(t *testing.T) {
	if got := GetJSONCodec(); got != JSONCodecGoccy {
		t.Errorf("default codec=%v want Goccy", got)
	}
	defer SetJSONCodec(JSONCodecGoccy) // restore

	SetJSONCodec(JSONCodecStd)
	if got := GetJSONCodec(); got != JSONCodecStd {
		t.Errorf("after SetJSONCodec(Std)=%v want Std", got)
	}
	SetJSONCodec(JSONCodecSonic)
	if got := GetJSONCodec(); got != JSONCodecSonic {
		t.Errorf("after SetJSONCodec(Sonic)=%v want Sonic", got)
	}
}

// Test_JSONCodec_AllProduceValidJSON confirms each codec produces parseable JSON
// for the record shape (so switching codecs never breaks a downstream parser).
func Test_JSONCodec_AllProduceValidJSON(t *testing.T) {
	defer SetJSONCodec(JSONCodecGoccy)
	r := &Record{
		level:  INFO,
		time:   "t",
		file:   "f.go:1",
		msg:    "msg with \"quotes\"",
		fields: []field{fld("trace_id", "abc"), fld("n", 42)},
	}

	for _, codec := range []JSONCodec{JSONCodecGoccy, JSONCodecStd, JSONCodecSonic} {
		SetJSONCodec(codec)
		b := r.JSON()
		var m map[string]interface{}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Errorf("codec %d: JSON() not parseable: %v\n%s", codec, err, b)
		}
		if m["msg"] != `msg with "quotes"` {
			t.Errorf("codec %d: msg quote escaping wrong: %v", codec, m["msg"])
		}
		fields, _ := m["fields"].(map[string]interface{})
		if fields["trace_id"] != "abc" {
			t.Errorf("codec %d: fields.trace_id=%v", codec, fields["trace_id"])
		}
	}
}

// Test_JSONCodec_TextFieldsHonored confirms the text format's trailing JSON
// object (Record.String with fields) also honors the codec.
func Test_JSONCodec_TextFieldsHonored(t *testing.T) {
	defer SetJSONCodec(JSONCodecGoccy)
	r := &Record{
		level: INFO, time: "t", file: "f", msg: "m",
		fields: []field{fld("k", "v")},
	}
	for _, codec := range []JSONCodec{JSONCodecGoccy, JSONCodecStd, JSONCodecSonic} {
		SetJSONCodec(codec)
		s := r.String()
		if !strings.Contains(s, `"k":"v"`) {
			t.Errorf("codec %d: String() fields not rendered: %q", codec, s)
		}
	}
}

// Benchmark_Record_JSON_Goccy measures the per-record FormatJSON cost under the
// default goccy codec (the production path). Compare against _Std and _Sonic to
// pick the codec for your platform.
func Benchmark_Record_JSON_Goccy(b *testing.B) {
	SetJSONCodec(JSONCodecGoccy)
	defer SetJSONCodec(JSONCodecGoccy)
	r := &Record{
		level: INFO, time: "2026-06-25T15:04:05.000+0800", file: "svc.go:42", msg: "benchmark json payload",
		fields: []field{fld("trace_id", "abc"), fld("user", 42), fld("route", "/api/v1")},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.JSON()
	}
}

// Benchmark_Record_JSON_Std measures encoding/json for comparison.
func Benchmark_Record_JSON_Std(b *testing.B) {
	SetJSONCodec(JSONCodecStd)
	defer SetJSONCodec(JSONCodecGoccy)
	r := &Record{
		level: INFO, time: "2026-06-25T15:04:05.000+0800", file: "svc.go:42", msg: "benchmark json payload",
		fields: []field{fld("trace_id", "abc"), fld("user", 42), fld("route", "/api/v1")},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.JSON()
	}
}

// Benchmark_Record_JSON_Sonic measures bytedance/sonic for comparison.
func Benchmark_Record_JSON_Sonic(b *testing.B) {
	SetJSONCodec(JSONCodecSonic)
	defer SetJSONCodec(JSONCodecGoccy)
	r := &Record{
		level: INFO, time: "2026-06-25T15:04:05.000+0800", file: "svc.go:42", msg: "benchmark json payload",
		fields: []field{fld("trace_id", "abc"), fld("user", 42), fld("route", "/api/v1")},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.JSON()
	}
}

// Benchmark_Record_JSON_NoFields measures the common no-With path (no fields
// object) — the production baseline for FormatJSON.
func Benchmark_Record_JSON_NoFields(b *testing.B) {
	SetJSONCodec(JSONCodecGoccy)
	r := &Record{level: INFO, time: "2026-06-25T15:04:05.000+0800", file: "svc.go:42", msg: "benchmark json payload"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.JSON()
	}
}
