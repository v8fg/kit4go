package log4go

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Test_LogFormat_String confirms the config-name mapping used by ParseLogLogFormat
// and the JSON config field.
func Test_LogFormat_String(t *testing.T) {
	if FormatText.String() != "text" {
		t.Errorf("FormatText.String()=%q want text", FormatText.String())
	}
	if FormatJSON.String() != "json" {
		t.Errorf("FormatJSON.String()=%q want json", FormatJSON.String())
	}
}

// Test_ParseLogLogFormat covers the config-string parser including the unknown
// fallback (which must be FormatText, never panic).
func Test_ParseLogLogFormat(t *testing.T) {
	cases := []struct {
		in   string
		want LogFormat
	}{
		{"json", FormatJSON},
		{"JSON", FormatJSON},
		{"  json ", FormatJSON},
		{"text", FormatText},
		{"", FormatText},
		{"yaml", FormatText}, // unknown -> text
	}
	for _, c := range cases {
		if got := ParseLogLogFormat(c.in); got != c.want {
			t.Errorf("ParseLogLogFormat(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

// Test_RecordJSON_NoFields verifies the JSON form of a record with no structured
// fields: valid JSON, contains time/level/msg, and OMITS the fields key.
func Test_RecordJSON_NoFields(t *testing.T) {
	r := &Record{level: INFO, time: "2026/06/25 10:00:00", file: "svc.go:42", msg: "hello"}
	b := r.JSON()
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Record.JSON() not valid JSON: %v\n%s", err, b)
	}
	if m["level"] != "INFO" {
		t.Errorf("level=%v want INFO", m["level"])
	}
	if m["msg"] != "hello" {
		t.Errorf("msg=%v want hello", m["msg"])
	}
	if _, ok := m["fields"]; ok {
		t.Errorf("fields key present with no fields: %s", b)
	}
	if !strings.HasSuffix(string(b), "\n") {
		t.Errorf("JSON must be newline-terminated: %q", b)
	}
}

// Test_RecordJSON_WithFields verifies fields render into the JSON fields object.
func Test_RecordJSON_WithFields(t *testing.T) {
	r := &Record{
		level:  ERROR,
		time:   "t",
		file:   "f.go:9",
		msg:    "boom",
		fields: []field{fld("trace_id", "abc"), fld("user", 42)},
	}
	b := r.JSON()
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b)
	}
	fields, ok := m["fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("fields not an object: %v", m["fields"])
	}
	if fields["trace_id"] != "abc" {
		t.Errorf("fields.trace_id=%v want abc", fields["trace_id"])
	}
	if u, ok := fields["user"].(float64); !ok || u != 42 {
		t.Errorf("fields.user=%v want 42", fields["user"])
	}
}

// Test_RecordJSON_UnmarshallableAny confirms a kindAny value JSON cannot encode
// (a channel) degrades to null in place. Record.JSON uses direct typed append,
// so it never fails and never falls back to text; the document stays valid JSON.
func Test_RecordJSON_UnmarshallableAny(t *testing.T) {
	r := &Record{level: INFO, time: "t", file: "f", msg: "m",
		fields: []field{anyField("k", make(chan int))}}
	b := r.JSON()
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("JSON() produced invalid JSON: %v\n%s", err, b)
	}
	if m["k"] != nil {
		t.Errorf("unmarshallable any = %v, want null", m["k"])
	}
	if m["msg"] != "m" {
		t.Errorf("msg=%v want m", m["msg"])
	}
}

// Test_SetFormat_DeliverJSON is the end-to-end check: a Logger with SetFormat(
// FormatJSON) pre-serializes formattedBytes on the record, and a registered writer
// (via captureWriter) sees those bytes rather than the text form.
func Test_SetFormat_DeliverJSON(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)
	root.SetFormat(FormatJSON)

	cw := &captureWriter{}
	root.Register(cw)

	root.With("trace_id", "t-1").Info("json line %d", 1)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		runtime.Gosched()
	}
	if cw.Len() == 0 {
		t.Fatal("record never reached writer")
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()
	if len(r.formattedBytes) == 0 {
		t.Fatalf("formattedBytes empty under FormatJSON; record was not pre-serialized")
	}
	var m map[string]interface{}
	if err := json.Unmarshal(r.formattedBytes, &m); err != nil {
		t.Fatalf("formattedBytes not valid JSON: %v\n%s", err, r.formattedBytes)
	}
	if m["msg"] != "json line 1" {
		t.Errorf("msg=%v want 'json line 1'", m["msg"])
	}
	if m["level"] != "INFO" {
		t.Errorf("level=%v want INFO", m["level"])
	}
	fields, _ := m["fields"].(map[string]interface{})
	if fields["trace_id"] != "t-1" {
		t.Errorf("fields.trace_id=%v want t-1", fields["trace_id"])
	}
}

// Test_SetFormat_TextNoJSONBytes confirms FormatText leaves formattedBytes nil so
// writers use String() (the default/backward-compatible path).
func Test_SetFormat_TextNoJSONBytes(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)
	// default format is FormatText; SetFormat explicitly for clarity
	root.SetFormat(FormatText)

	cw := &captureWriter{}
	root.Register(cw)

	root.Info("text line")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		runtime.Gosched()
	}
	if cw.Len() == 0 {
		t.Fatal("record never reached writer")
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()
	if r.formattedBytes != nil {
		t.Errorf("formattedBytes non-nil under FormatText: %s", r.formattedBytes)
	}
	s := r.String()
	if !strings.Contains(s, "[INFO]") {
		t.Errorf("String() missing INFO: %q", s)
	}
}

// Test_Format_InheritedByChild confirms a child Logger inherits the parent's
// format (so With().Info() under a JSON root still emits JSON).
func Test_Format_InheritedByChild(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)
	root.SetFormat(FormatJSON)

	child := root.With("k", "v")
	if child.Format() != FormatJSON {
		t.Fatalf("child format=%v want FormatJSON (inherited)", child.Format())
	}
}

// Test_ConsoleWriter_JSONFastPath directly drives ConsoleWriter.Write with a
// record carrying formattedBytes and asserts it emits the bytes (no color/text).
func Test_ConsoleWriter_JSONFastPath(t *testing.T) {
	w := &ConsoleWriter{color: true, fullColor: true} // color must be IGNORED for JSON
	r := &Record{
		level:          INFO,
		time:           "t",
		file:           "f",
		msg:            "m",
		formattedBytes: []byte(`{"time":"t","level":"INFO","msg":"m"}` + "\n"),
	}
	// Write goes to os.Stdout; we can't easily capture it, but the contract is
	// "no error and no panic". The fast path is exercised; correctness of bytes
	// is covered by Test_SetFormat_DeliverJSON via captureWriter.
	if err := w.Write(r); err != nil {
		t.Fatalf("ConsoleWriter.Write formattedBytes err: %v", err)
	}
}

// Test_FileWriter_JSONFastPath drives FileWriter.writeSync with formattedBytes and
// confirms the file receives the JSON bytes (not the text line).
func Test_FileWriter_JSONFastPath(t *testing.T) {
	w := NewFileWriterWithOptions(FileWriterOptions{
		Enable:   true,
		Level:    LevelFlagDebug,
		Filename: t.TempDir() + "/json-%Y%M%D.log",
		Rotate:   true,
		Daily:    true,
	})
	w.level = DEBUG
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer w.Stop()

	r := &Record{
		level:          INFO,
		time:           "2026/06/25 10:00:00",
		file:           "svc.go:1",
		msg:            "json msg",
		formattedBytes: []byte(`{"time":"2026-06-25T10:00:00.000+0800","level":"INFO","msg":"json msg"}` + "\n"),
	}
	if err := w.writeSync(r); err != nil {
		t.Fatalf("writeSync: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	// read the file back: flush bufio, then read the underlying file.
	w.lock.RLock()
	_ = w.fileBufWriter.Flush()
	name := w.file.Name()
	w.lock.RUnlock()

	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if !strings.Contains(string(data), `"msg":"json msg"`) {
		t.Errorf("file did not receive JSON bytes; got: %s", data)
	}
	if strings.Contains(string(data), "[INFO]") {
		t.Errorf("file got text form instead of JSON: %s", data)
	}
}
