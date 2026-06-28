package log4go_test

// This test lives in the EXTERNAL test package (log4go_test) on purpose: it
// verifies caller (file:line) resolution from a call site OUTSIDE the log4go
// package — the real production scenario. That makes the assertion invariant to
// architecture and compiler inlining. The internal in-package caller test
// (caller_cache_test.go) cannot make this guarantee on every toolchain, because
// its call site lives inside package log4go and may be walked past on some
// compilers (observed on linux/amd64); this external test is the authoritative
// cross-arch correctness check.

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/v8fg/kit4go/log4go"
)

func TestCallerResolution_ExternalPackage(t *testing.T) {
	var buf bytes.Buffer
	lg := log4go.NewLogger()
	defer lg.Close()
	lg.SetLevel(log4go.DEBUG)
	lg.WithCaller(true)
	lg.Register(log4go.NewIOWriter(&buf, log4go.DEBUG))

	lg.Info("caller-resolution-probe")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "caller-resolution-probe") {
			break
		}
	}
	out := buf.String()
	if !strings.Contains(out, "caller-resolution-probe") {
		t.Fatalf("record never reached writer; buf=%q", out)
	}
	// The caller MUST resolve to this external test file, not a log4go-internal
	// source file (the original cross-arch bug reported log.go instead).
	if !strings.Contains(out, "caller_external_test.go") {
		t.Errorf("caller not resolved to the external test file; got: %q", out)
	}
}
