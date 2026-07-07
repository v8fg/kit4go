package log4go

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Test_WebhookAlertSink_Post verifies an alert is POSTed asynchronously.
func Test_WebhookAlertSink_Post(t *testing.T) {
	gotCh := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		select {
		case gotCh <- string(b):
		default:
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	sink := NewWebhookAlertSink(srv.URL, 16, LarkTextFormatter(srv.URL))
	defer sink.Close()
	sink.Send(AlertError, "DROP", "queue full; record lost")

	// The webhook POST runs on WebhookAlertSink's async daemon goroutine. Under
	// -race/CI load the daemon's select + http.Client.Do round-trip against the
	// httptest server can exceed the original 2s budget, falsely timing out.
	// Poll up to 5s (the same budget used by the NetWriter coverage waits) so
	// the test observes the POST deterministically rather than racing a fixed
	// deadline.
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case got := <-gotCh:
			if !strings.Contains(got, "DROP") || !strings.Contains(got, "queue full") {
				t.Errorf("unexpected payload: %s", got)
			}
			return
		default:
		}
		if !time.Now().Before(deadline) {
			t.Fatal("webhook POST not received in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Test_OverflowStats_AlertSink verifies the stats alert fans out to a sink.
func Test_OverflowStats_AlertSink(t *testing.T) {
	sink := &recordingSink{}
	var s OverflowStats
	s.SetAlertEvery(2, 2)
	s.SetAlertSink(sink)

	s.IncDropped() // first -> 1 alert
	s.IncDropped() // 2 == every -> 1 alert
	s.IncDropped() // 3 -> no alert (not every)
	s.IncSpilled() // first -> 1 alert

	if len(sink.msgs) != 3 {
		t.Fatalf("got %d alerts, want 3: %+v", len(sink.msgs), sink.msgs)
	}
	if sink.msgs[0].kind != "DROP" || sink.msgs[0].level != AlertError {
		t.Errorf("first alert %+v", sink.msgs[0])
	}
}

type recordingSink struct {
	msgs []alertRecord
}

type alertRecord struct {
	level AlertLevel
	kind  string
	text  string
}

func (r *recordingSink) Send(level AlertLevel, kind, text string) {
	r.msgs = append(r.msgs, alertRecord{level: level, kind: kind, text: text})
}
func (r *recordingSink) Close() error { return nil }
