package log4go

import (
	"bytes"
	"strings"
	"testing"
)

// Test_IOWriter_Text confirms IOWriter renders the text form into the wrapped
// io.Writer.
func Test_IOWriter_Text(t *testing.T) {
	var buf bytes.Buffer
	w := NewIOWriter(&buf, DEBUG)
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	r := &Record{level: INFO, time: "t", file: "svc.go:1", msg: "io hello"}
	if err := w.Write(r); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "[INFO]") || !strings.Contains(got, "io hello") {
		t.Errorf("text output wrong: %q", got)
	}
}

// Test_IOWriter_JSON confirms IOWriter honors r.jsonBytes (FormatJSON records
// ship as JSON).
func Test_IOWriter_JSON(t *testing.T) {
	var buf bytes.Buffer
	w := NewIOWriter(&buf, DEBUG)
	r := &Record{
		level: INFO, time: "t", file: "f", msg: "m",
		jsonBytes: []byte(`{"time":"t","level":"INFO","msg":"m"}` + "\n"),
	}
	if err := w.Write(r); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"msg":"m"`) {
		t.Errorf("JSON output wrong: %q", got)
	}
	if strings.Contains(got, "[INFO]") {
		t.Errorf("emitted text form under jsonBytes: %q", got)
	}
}

// Test_IOWriter_LevelFilter confirms records above the writer's level are
// dropped.
func Test_IOWriter_LevelFilter(t *testing.T) {
	var buf bytes.Buffer
	w := NewIOWriter(&buf, INFO) // INFO only; DEBUG(7) > INFO(6) so dropped
	r := &Record{level: DEBUG, time: "t", file: "f", msg: "debug-line"}
	if err := w.Write(r); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("DEBUG record passed INFO-level writer: %q", buf.String())
	}
}

// Test_IOWriter_WithFields confirms structured fields render via String().
func Test_IOWriter_WithFields(t *testing.T) {
	var buf bytes.Buffer
	w := NewIOWriter(&buf, DEBUG)
	r := &Record{
		level:  ERROR,
		time:   "t",
		file:   "f.go:9",
		msg:    "boom",
		fields: []field{{key: "trace_id", val: "abc"}},
	}
	if err := w.Write(r); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"trace_id":"abc"`) {
		t.Errorf("fields not rendered: %q", got)
	}
}

// Test_IOWriter_ImplementsWriter is the compile-time check.
func Test_IOWriter_ImplementsWriter(t *testing.T) {
	var _ Writer = (*IOWriter)(nil)
}
