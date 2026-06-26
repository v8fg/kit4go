package log4go

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// Test_Record_Logfmt covers the logfmt line shape, quoting rules and typed
// scalar rendering.
func Test_Record_Logfmt(t *testing.T) {
	r := &Record{
		level:    ERROR,
		msg:      `bid failed "x"`,
		file:     "svc.go:42",
		unixNano: 1782392990_000_000_000,
		fields: []field{
			strField("trace_id", "abc"),
			intField("count", 7),
			boolField("ok", false),
			floatField("rate", 1.5),
			errField("err", errors.New("boom")),
			strField("q", "needs quote"),
		},
	}
	out := string(r.Logfmt())
	if !strings.HasPrefix(out, "time=") {
		t.Errorf("missing time= prefix: %q", out)
	}
	if !strings.Contains(out, "level=ERROR") {
		t.Errorf("missing level=ERROR: %q", out)
	}
	// message with spaces+quotes must be quoted/escaped
	if !strings.Contains(out, `msg="bid failed \"x\""`) {
		t.Errorf("msg not quoted/escaped: %q", out)
	}
	// scalars render bare
	if !strings.Contains(out, " trace_id=abc") {
		t.Errorf("trace_id wrong: %q", out)
	}
	if !strings.Contains(out, " count=7") {
		t.Errorf("count wrong: %q", out)
	}
	if !strings.Contains(out, " ok=false") {
		t.Errorf("ok wrong: %q", out)
	}
	if !strings.Contains(out, " rate=1.5") {
		t.Errorf("rate wrong: %q", out)
	}
	if !strings.Contains(out, " err=boom") {
		t.Errorf("err wrong: %q", out)
	}
	if !strings.Contains(out, ` q="needs quote"`) {
		t.Errorf("quoted value wrong: %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("missing trailing newline: %q", out)
	}
}

// Test_Logfmt_NoFileNoFields: minimal record omits file and fields cleanly.
func Test_Logfmt_NoFileNoFields(t *testing.T) {
	r := &Record{level: INFO, msg: "hi", unixNano: 1}
	out := string(r.Logfmt())
	if strings.Contains(out, "file=") {
		t.Errorf("file should be omitted: %q", out)
	}
	// exactly one trailing newline, no stray spaces
	if strings.Count(out, "\n") != 1 {
		t.Errorf("want single newline: %q", out)
	}
}

// Test_SetFormat_Logfmt_Delivers verifies FormatLogfmt pre-serializes into
// formattedBytes end-to-end through a registered writer.
func Test_SetFormat_Logfmt_Delivers(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)
	root.SetFormat(FormatLogfmt)

	cw := &captureWriter{}
	root.Register(cw)
	root.WithString("trace_id", "t-1").Info("logfmt line")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
	}
	if cw.Len() == 0 {
		t.Fatal("record never reached writer")
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()
	if len(r.formattedBytes) == 0 {
		t.Fatal("formattedBytes empty under FormatLogfmt")
	}
	s := string(r.formattedBytes)
	for _, want := range []string{"time=", "level=INFO", `msg="logfmt line"`, "trace_id=t-1"} {
		if !strings.Contains(s, want) {
			t.Errorf("logfmt output missing %q: %s", want, s)
		}
	}
}
