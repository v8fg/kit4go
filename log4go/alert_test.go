package log4go

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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

// ---- R20 regression tests (P1 shutdown bound + P2 config races) ----
//
// Each test is written so it FAILS on the pre-fix code:
//   - the race tests data-race under `go test -race` when the fields are plain
//     ints (concurrent unlocked read/write);
//   - the shutdown-bound test would block for maxRetries*~3.2s on the old code
//     (retry Sleep ignored quit), exceeding the tight deadline below.

// Test_R20_WebhookAlertSink_ConfigRaceConcurrentSend hammers Send concurrently
// with SetRateLimit/SetMaxRetries. Under -race the pre-fix code (plain int
// fields, unlocked writes) reports a data race and fails.
func Test_R20_WebhookAlertSink_ConfigRaceConcurrentSend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	sink := NewWebhookAlertSink(srv.URL, 64, LarkTextFormatter(srv.URL))
	defer sink.Close()

	// Bound the test in wall-clock time (not just iteration count) so the
	// daemon can drain under -race without being starved by a busy-loop.
	deadline := time.Now().Add(500 * time.Millisecond)
	stop := make(chan struct{})

	var wg sync.WaitGroup

	// Writer: flip the rate limit and retry budget concurrently with Send. A
	// tiny yield lets the daemon make progress and still produces enough
	// overlap for the race detector to fire on the old (plain-int) code.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				sink.SetRateLimit(0)
				sink.SetRateLimit(100)
				sink.SetMaxRetries(0)
				sink.SetMaxRetries(3)
				time.Sleep(time.Microsecond)
			}
			if time.Now().After(deadline) {
				return
			}
		}
	}()

	// Producer: Send reads maxPerSec (allow) on the hot path.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				sink.Send(AlertError, "DROP", "race probe")
			}
			if time.Now().After(deadline) {
				return
			}
		}
	}()

	wg.Wait()
	close(stop)
}

// Test_R20_WebhookAlertSink_CloseInterruptsInFlightRetry verifies that Close()
// (a) returns promptly and (b) actually stops the daemon's in-flight retry
// against a 500 server with a large maxRetries.
//
// On the pre-fix code the retry Sleep ignored quit, so the daemon kept firing
// retries long after Close returned (goroutine leak + wasted requests). This
// test counts server hits in a window AFTER Close: on the old code the daemon
// is still alive and the retry loop keeps hitting the server (200ms cadence),
// so hitsAfter > 0 fails the test; on the fixed code the retry wait is
// interrupted and the daemon exits, so hitsAfter == 0.
func Test_R20_WebhookAlertSink_CloseInterruptsInFlightRetry(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	sink := NewWebhookAlertSink(srv.URL, 8, LarkTextFormatter(srv.URL))
	// Large retry budget so the daemon would otherwise retry for many seconds.
	sink.SetMaxRetries(50)

	// Prime the daemon so a POST (and thus a retry wait) is in flight when we
	// close. A few sends guarantee overlap with the retry backoff.
	for i := 0; i < 8; i++ {
		sink.Send(AlertError, "DROP", "prime retry")
	}

	// Let the daemon enter its retry backoff, then close mid-retry.
	time.Sleep(60 * time.Millisecond)

	start := time.Now()
	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	elapsed := time.Since(start)

	// (a) Close must return promptly: on old code Close returned instantly
	// anyway (no wg.Wait), so this is a sanity bound; the real discriminator
	// is (b). Keep a generous ceiling for -race scheduler latency.
	if elapsed > 2*time.Second {
		t.Fatalf("Close blocked for %v; retry was not interrupted by quit", elapsed)
	}

	// (b) The daemon must have STOPPED. Sample server hits for a window long
	// enough that the old code's retry loop (200ms first backoff, growing)
	// would fire at least once more. On the fixed code the retry wait was
	// interrupted by quit and the daemon exited, so no further hits land.
	hitsBeforeWindow := hits.Load()
	time.Sleep(500 * time.Millisecond)
	hitsAfter := hits.Load() - hitsBeforeWindow
	if hitsAfter > 0 {
		t.Fatalf("daemon still retrying after Close: %d server hits in 500ms post-Close window (total=%d)", hitsAfter, hits.Load())
	}
}

// Test_R20_WebhookAlertSink_CloseIsIdempotent ensures the wg.Wait in Close is
// safe to call twice (once.Do guard) and the second call returns immediately.
func Test_R20_WebhookAlertSink_CloseIsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	sink := NewWebhookAlertSink(srv.URL, 4, LarkTextFormatter(srv.URL))
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
}

// Test_R20_OverflowStats_ConfigRaceConcurrentInc hammers IncDropped/IncSpilled
// (hot path: read dropEvery/spillEvery/alertSink) concurrently with
// SetAlertEvery/SetAlertSink. Under -race the pre-fix code (plain fields,
// unlocked writes) reports a data race and fails.
//
// The sink used here is a thread-safe atomic counter (safeSink): a non-thread-
// safe sink would race against ITSELF (concurrent append), masking whether the
// production config read is safe. We are verifying OverflowStats' atomics, not
// the test fixture.
func Test_R20_OverflowStats_ConfigRaceConcurrentInc(t *testing.T) {
	var s OverflowStats
	s.SetAlertEvery(2, 2)
	s.SetAlertSink(&safeSink{})

	// Bound by wall-clock so the configurator yields instead of busy-looping.
	deadline := time.Now().Add(300 * time.Millisecond)
	stop := make(chan struct{})

	var wg sync.WaitGroup

	// Configurator: flip throttling + swap the sink while events fire.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				s.SetAlertEvery(uint64(i%5), uint64(i%7))
				s.SetAlertSink(&safeSink{})
				time.Sleep(time.Microsecond)
			}
			if time.Now().After(deadline) {
				return
			}
		}
	}()

	// Hot-path producers: read every/sink atomically under load.
	inc := func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				s.IncDropped()
				s.IncSpilled()
			}
			if time.Now().After(deadline) {
				return
			}
		}
	}
	wg.Add(4)
	go inc()
	go inc()
	go inc()
	go inc()

	wg.Wait()
	close(stop)

	// Sanity: counters advanced under concurrency (exact count is timing-
	// dependent now, so only assert forward progress, not a fixed value).
	if s.Dropped() == 0 || s.Spilled() == 0 {
		t.Fatalf("counters did not advance: dropped=%d spilled=%d", s.Dropped(), s.Spilled())
	}
}

// safeSink is a concurrency-safe AlertSink (atomic counter) used by the race
// tests so the fixture itself never reports a self-race, isolating the race to
// the production code under test.
type safeSink struct {
	n atomic.Uint64
}

func (k *safeSink) Send(AlertLevel, string, string) { k.n.Add(1) }
func (k *safeSink) Close() error                    { return nil }
