package log4go

import (
	"fmt"
	"strings"
	"testing"
)

// recordStringSprintf reproduces the pre-optimization Record.String body for a
// byte-for-byte equivalence check against the current (strings.Builder) path.
func recordStringSprintf(r *Record) string {
	return fmt.Sprintf("#%d %s [%s] <%s> %s\n", r.seq, r.time, LevelFlags[r.level], r.file, r.msg)
}

// Test_RecordString_FormatEquivalence guards the format-preserving invariant of
// the strings.Builder optimization: the Builder path must match the original
// fmt.Sprintf("%s [%s] <%s> %s\n", ...) path exactly, including edge cases that
// would otherwise be misinterpreted as format verbs by Sprintf.
func Test_RecordString_FormatEquivalence(t *testing.T) {
	cases := []struct {
		name string
		r    Record
	}{
		{"info", Record{level: INFO, time: "2026/06/25 10:00:00", file: "svc.go:42", msg: "hello world"}},
		{"empty msg", Record{level: DEBUG, time: "2026/06/25 10:00:00", file: "svc.go:1", msg: ""}},
		{"percent verbs in msg", Record{level: ERROR, time: "2026/06/25 10:00:00", file: "svc.go:7", msg: "value=%d pct=100%% done"}},
		{"brackets in file", Record{level: WARNING, time: "t", file: "pkg/[x]/svc.go:9", msg: "m"}},
		{"every level", Record{level: EMERGENCY, time: "t", file: "f:0", msg: "m"}},
	}
	for _, c := range cases {
		got := c.r.String()               // production path (Builder)
		want := recordStringSprintf(&c.r) // original path (Sprintf)
		if got != want {
			t.Fatalf("%s: mismatch\n got=%q\nwant=%q", c.name, got, want)
		}
		if !strings.HasSuffix(got, "\n") {
			t.Fatalf("%s: missing trailing newline: %q", c.name, got)
		}
	}
}
