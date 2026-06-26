package log4go

import (
	"strings"
	"testing"
	"time"
)

// Test_Record_Accessors covers the read-only getters exposed for custom
// filters/formatters in other packages.
func Test_Record_Accessors(t *testing.T) {
	r := &Record{
		level:    ERROR,
		time:     "2026-06-26 12:00:00",
		file:     "svc.go:42",
		msg:      "boom",
		unixNano: 1782392990_123456789,
		seq:      7,
		fields:   []field{fld("domain", "pay"), fld("code", 42)},
	}
	if r.Msg() != "boom" {
		t.Errorf("Msg=%q want boom", r.Msg())
	}
	if r.TimeStr() != "2026-06-26 12:00:00" {
		t.Errorf("TimeStr=%q", r.TimeStr())
	}
	if r.FileLine() != "svc.go:42" {
		t.Errorf("FileLine=%q", r.FileLine())
	}
	if r.LevelName() != "ERROR" || r.LevelInt() != ERROR {
		t.Errorf("level name/int=%q/%d want ERROR/%d", r.LevelName(), r.LevelInt(), ERROR)
	}
	if r.UnixNano() != 1782392990_123456789 || r.Seq() != 7 {
		t.Errorf("unixNano=%d seq=%d", r.UnixNano(), r.Seq())
	}
	if v, ok := r.FieldValue("domain"); !ok || v != "pay" {
		t.Errorf("FieldValue(domain)=%v,%v want pay,true", v, ok)
	}
	if v, ok := r.FieldValue("code"); !ok || v != 42 {
		t.Errorf("FieldValue(code)=%v,%v want 42,true", v, ok)
	}
	if _, ok := r.FieldValue("missing"); ok {
		t.Error("FieldValue(missing) should be false")
	}
}

func rec(domain string, code int, msg string) *Record {
	return &Record{level: ERROR, msg: msg, fields: []field{
		fld("domain", domain), fld("code", code),
	}}
}

// Test_Filter_MatchField: exact value and stringified value both match.
func Test_Filter_MatchField(t *testing.T) {
	if !MatchField("code", 42)(rec("pay", 42, "x")) {
		t.Error("exact int match failed")
	}
	if !MatchField("code", "42")(rec("pay", 42, "x")) {
		t.Error("string-form match against int field failed")
	}
	if MatchField("code", 99)(rec("pay", 42, "x")) {
		t.Error("non-match should be false")
	}
	if MatchField("missing", 1)(rec("pay", 42, "x")) {
		t.Error("absent field should be false")
	}
}

// Test_Filter_MatchFieldIn: OR over values.
func Test_Filter_MatchFieldIn(t *testing.T) {
	f := MatchFieldIn("domain", "pay", "risk", "auth")
	if !f(rec("pay", 1, "x")) || !f(rec("risk", 1, "x")) {
		t.Error("expected match for pay/risk")
	}
	if f(rec("ops", 1, "x")) {
		t.Error("ops should not match")
	}
}

// Test_Filter_MatchKeyword: case-insensitive substring on the message.
func Test_Filter_MatchKeyword(t *testing.T) {
	f := MatchKeyword("PAYMENT")
	if !f(rec("pay", 1, "user payment failed")) {
		t.Error("case-insensitive keyword match failed")
	}
	if f(rec("pay", 1, "user login")) {
		t.Error("non-containing message should not match")
	}
}

// Test_Filter_MatchKeywordIn: any of several substrings.
func Test_Filter_MatchKeywordIn(t *testing.T) {
	f := MatchKeywordIn("timeout", "refused")
	if !f(rec("", 1, "db timeout")) || !f(rec("", 1, "connection refused")) {
		t.Error("expected match")
	}
	if f(rec("", 1, "all good")) {
		t.Error("no keyword present")
	}
}

// Test_Filter_Combinators: AllOf / AnyOf / NotMatch logic.
func Test_Filter_Combinators(t *testing.T) {
	pay := MatchField("domain", "pay")
	err := MatchKeyword("fail")

	if !AllOf(pay, err)(rec("pay", 1, "payment fail")) {
		t.Error("AllOf should match when both match")
	}
	if AllOf(pay, err)(rec("pay", 1, "payment ok")) {
		t.Error("AllOf should not match when one misses")
	}
	if !AnyOf(pay, err)(rec("ops", 1, "payment fail")) {
		t.Error("AnyOf should match when one matches")
	}
	if AnyOf(pay, err)(rec("ops", 1, "all ok")) {
		t.Error("AnyOf should not match when none match")
	}
	if !NotMatch(pay)(rec("ops", 1, "x")) {
		t.Error("NotMatch should be true when inner is false")
	}
	if NotMatch(pay)(rec("pay", 1, "x")) {
		t.Error("NotMatch should be false when inner is true")
	}
}

// Test_WebhookWriter_BuiltinFilter: a constructor-built Filter plugs into the
// writer end-to-end.
func Test_WebhookWriter_BuiltinFilter(t *testing.T) {
	sink := &mockAlertSink{}
	w := NewWebhookWriter(sink, WebhookWriterOptions{
		Level:  "error",
		Filter: AllOf(MatchField("domain", "pay"), MatchKeyword("fail")),
	})
	_ = w.Write(rec("pay", 1, "payment failed")) // matches
	_ = w.Write(rec("pay", 1, "payment ok"))     // keyword miss
	_ = w.Write(rec("ops", 1, "payment failed")) // field miss

	got := sink.snapshot()
	if len(got) != 1 {
		t.Fatalf("sent %d, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].text, "payment failed") {
		t.Errorf("text=%q", got[0].text)
	}
}

// Test_WebhookWriter_RateFormatter: when a gate is set and RateFormatter is
// configured, the forwarded payload carries the in-window count prefix.
func Test_WebhookWriter_RateFormatter(t *testing.T) {
	sink := &mockAlertSink{}
	gate := NewRateAlerter(time.Second, 2) // fire once at >=2/s, cooldown 1s
	w := NewWebhookWriter(sink, WebhookWriterOptions{
		Level:         "error",
		Gate:          gate,
		RateFormatter: DefaultRateWebhookFormatter,
	})

	_ = w.Write(rec("pay", 1, "e1")) // sum=1, under threshold -> skipped
	_ = w.Write(rec("pay", 1, "e2")) // sum=2, fires -> forwarded with count
	_ = w.Write(rec("pay", 1, "e3")) // cooldown -> skipped

	got := sink.snapshot()
	if len(got) != 1 {
		t.Fatalf("forwarded %d, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].text, "[2 in window]") {
		t.Errorf("text missing rate prefix: %q", got[0].text)
	}
	if !strings.Contains(got[0].text, "e2") {
		t.Errorf("text missing message: %q", got[0].text)
	}
}
