package log4go

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockAlertSink records every Send for assertion. It is an AlertSink.
type mockAlertSink struct {
	mu   sync.Mutex
	msgs []alertMsg
}

func (m *mockAlertSink) Send(level AlertLevel, kind, text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, alertMsg{level: level, kind: kind, text: text})
}

func (m *mockAlertSink) Close() error { return nil }

func (m *mockAlertSink) snapshot() []alertMsg {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]alertMsg, len(m.msgs))
	copy(out, m.msgs)
	return out
}

// Test_WebhookWriter_LevelFilter: only records at/above the writer's level reach
// the sink; lower-severity records are counted as skipped.
func Test_WebhookWriter_LevelFilter(t *testing.T) {
	sink := &mockAlertSink{}
	w := NewWebhookWriter(sink, WebhookWriterOptions{Level: "error"})

	_ = w.Write(&Record{level: DEBUG, msg: "dbg"})
	_ = w.Write(&Record{level: INFO, msg: "info"})
	_ = w.Write(&Record{level: WARNING, msg: "warn"})
	_ = w.Write(&Record{level: ERROR, msg: "boom"})
	_ = w.Write(&Record{level: CRITICAL, msg: "critical"})

	got := sink.snapshot()
	if len(got) != 2 { // ERROR + CRITICAL
		t.Fatalf("sent %d, want 2: %+v", len(got), got)
	}
	if got[0].text[0] == 0 || !strings.Contains(got[0].text, "boom") {
		t.Errorf("first send text=%q want to contain 'boom'", got[0].text)
	}
	if got[0].level != AlertError || got[1].level != AlertError {
		t.Errorf("alert levels=%v,%v want AlertError,AlertError", got[0].level, got[1].level)
	}
	m := w.Metrics()
	if m.Sent != 2 || m.Skipped != 3 {
		t.Errorf("Metrics sent=%d skipped=%d want 2,3", m.Sent, m.Skipped)
	}
}

// Test_WebhookWriter_Filter: the predicate narrows which at-level records fire.
func Test_WebhookWriter_Filter(t *testing.T) {
	sink := &mockAlertSink{}
	w := NewWebhookWriter(sink, WebhookWriterOptions{
		Level:  "error",
		Filter: func(r *Record) bool { return strings.Contains(r.msg, "pay") },
	})

	_ = w.Write(&Record{level: ERROR, msg: "db timeout"})     // filtered out
	_ = w.Write(&Record{level: ERROR, msg: "payment failed"}) // passes
	_ = w.Write(&Record{level: ERROR, msg: "pay channel down"}) // passes

	got := sink.snapshot()
	if len(got) != 2 {
		t.Fatalf("sent %d, want 2 (only 'pay*' messages): %+v", len(got), got)
	}
}

// Test_WebhookWriter_Gate: the RateAlerter forwards only past the threshold.
func Test_WebhookWriter_Gate(t *testing.T) {
	sink := &mockAlertSink{}
	gate := NewRateAlerter(time.Second, 3) // fire once >=3/s, cooldown == window
	w := NewWebhookWriter(sink, WebhookWriterOptions{Level: "error", Gate: gate})

	var sent int64
	w.SetOnEvent(func(name string, delta int64) {
		if name == "sent" {
			atomic.AddInt64(&sent, delta)
		}
	})

	for i := 0; i < 10; i++ {
		_ = w.Write(&Record{level: ERROR, msg: "boom"})
	}
	// 10 errors in 1s, threshold 3, cooldown 1s => exactly 1 forwarded.
	if got := len(sink.snapshot()); got != 1 {
		t.Errorf("gate forwarded %d, want 1", got)
	}
	if atomic.LoadInt64(&sent) != 1 {
		t.Errorf("onEvent sent=%d want 1", sent)
	}
}

// Test_WebhookWriter_NilSinkFallback: a nil sink is usable (LogAlertSink) and
// does not panic — handy for tests/dev.
func Test_WebhookWriter_NilSinkFallback(t *testing.T) {
	w := NewWebhookWriter(nil, WebhookWriterOptions{Level: "warn"})
	if err := w.Write(&Record{level: ERROR, msg: "ok"}); err != nil {
		t.Fatalf("Write with nil sink errored: %v", err)
	}
	if w.Metrics().Sent != 1 {
		t.Errorf("Sent=%d want 1", w.Metrics().Sent)
	}
}

// Test_WebhookWriter_DefaultFormatter: the default formatter includes level,
// file, msg and structured fields.
func Test_WebhookWriter_DefaultFormatter(t *testing.T) {
	r := &Record{
		level:  ERROR,
		time:   "2026-06-26 12:00:00",
		file:   "svc.go:42",
		msg:    "payment failed",
		fields: []field{{key: "order_id", val: "o-1"}},
	}
	kind, text := DefaultWebhookFormatter(r)
	if kind != "ERROR" {
		t.Errorf("kind=%q want ERROR", kind)
	}
	for _, want := range []string{"[ERROR]", "<svc.go:42>", "payment failed", `"order_id":"o-1"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q missing %q", text, want)
		}
	}
}

// Test_WebhookWriter_CustomFormatter: a custom formatter controls the payload.
func Test_WebhookWriter_CustomFormatter(t *testing.T) {
	sink := &mockAlertSink{}
	w := NewWebhookWriter(sink, WebhookWriterOptions{
		Level: "error",
		Formatter: func(r *Record) (string, string) {
			return "payment", "order " + r.msg
		},
	})
	_ = w.Write(&Record{level: ERROR, msg: "X-99"})
	got := sink.snapshot()
	if len(got) != 1 || got[0].kind != "payment" || got[0].text != "order X-99" {
		t.Fatalf("unexpected: %+v", got)
	}
}

// closeTrackingSink is an AlertSink that records whether Close was called.
type closeTrackingSink struct {
	closed atomic.Bool
}

func (c *closeTrackingSink) Send(AlertLevel, string, string) {}
func (c *closeTrackingSink) Close() error                    { c.closed.Store(true); return nil }

// Test_WebhookWriter_ClosedByLoggerClose verifies the io.Closer path added to
// Logger.Close: registering a WebhookWriter and closing the logger must close
// the underlying sink, so a single defer log4go.Close() cleans up the webhook
// daemon.
func Test_WebhookWriter_ClosedByLoggerClose(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	root.SetLevel(DEBUG)

	sink := &closeTrackingSink{}
	wh := NewWebhookWriter(sink, WebhookWriterOptions{Level: "info"})
	root.Register(wh)

	root.Info("hi")
	root.Close()

	if !sink.closed.Load() {
		t.Fatal("Logger.Close did not close the WebhookWriter sink (io.Closer path)")
	}
}
