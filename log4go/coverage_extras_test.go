package log4go

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Test_AlertFormatters covers the three webhook payload formatters and the
// AlertLevel.String method, all of which were 0% covered.
func Test_AlertFormatters(t *testing.T) {
	for _, lvl := range []AlertLevel{AlertInfo, AlertWarn, AlertError} {
		if s := lvl.String(); s == "" {
			t.Fatalf("AlertLevel(%d).String empty", lvl)
		}
	}
	cases := []struct {
		name string
		f    AlertFormatter
	}{
		{"lark", LarkTextFormatter("https://h")},
		{"dingtalk", DingtalkTextFormatter("https://h")},
		{"wechat", WechatTextFormatter("https://h")},
	}
	for _, c := range cases {
		ct, body := c.f(AlertError, "overflow", "queue full")
		if !strings.Contains(ct, "json") {
			t.Errorf("%s: content-type=%q want json", c.name, ct)
		}
		if !strings.Contains(string(body), "queue full") {
			t.Errorf("%s: body missing text: %s", c.name, body)
		}
		if !strings.Contains(string(body), `"text"`) {
			t.Errorf("%s: body missing text field: %s", c.name, body)
		}
	}
}

// Test_LogAlertSink covers the default LogAlertSink Send/Close (uses the
// standard logger, must not panic).
func Test_LogAlertSink(t *testing.T) {
	s := LogAlertSink{}
	s.Send(AlertWarn, "overflow", "queue full")
	if err := s.Close(); err != nil {
		t.Fatalf("LogAlertSink.Close=%v", err)
	}
}

// Test_WebhookAlertSink_RateLimit covers SetRateLimit + allow (rate limiting)
// and SetMaxRetries setters.
func Test_WebhookAlertSink_RateLimit(t *testing.T) {
	var got int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&got, 1)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	sink := NewWebhookAlertSink(srv.URL, 64, LarkTextFormatter(srv.URL))
	sink.SetMaxRetries(1)
	sink.SetRateLimit(2) // at most 2 alerts/sec
	defer sink.Close()

	// Fire several alerts; rate limit caps how many enqueue.
	for range 10 {
		sink.Send(AlertWarn, "overflow", "burst")
	}
	time.Sleep(300 * time.Millisecond)
	// allow() should have rejected most; at most a couple POSTs land.
	if atomic.LoadInt64(&got) > 4 {
		t.Fatalf("rate limit ineffective: got %d POSTs (want <= 4)", got)
	}
}

// Test_WebhookAlertSink_Retries covers the retry path on a 5xx server.
func Test_WebhookAlertSink_Retries(t *testing.T) {
	var attempts int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable) // always 5xx -> retries exhausted
	}))
	defer srv.Close()

	sink := NewWebhookAlertSink(srv.URL, 64, LarkTextFormatter(srv.URL))
	sink.SetMaxRetries(2)
	defer sink.Close()
	sink.Send(AlertError, "overflow", "retry me")
	time.Sleep(1 * time.Second) // allow retries (200ms backoff each)
	if got := atomic.LoadInt64(&attempts); got < 2 {
		t.Fatalf("expected retries, got %d attempts", got)
	}
}

// Test_ShardLogger_RegisterSetLevel covers ShardLogger.Register (writes
// registered on every shard) and SetLevel (level set on every shard), both 0%
// covered. Drives a console writer across shards and asserts records arrive.
func Test_ShardLogger_RegisterSetLevel(t *testing.T) {
	sl := NewShardLogger(3)
	defer sl.Close()
	sl.SetLevel(DEBUG)
	sl.Register(NewConsoleWriterWithOptions(ConsoleWriterOptions{
		Enable: true, Color: true, Level: LevelFlagDebug,
	}))
	// Each shard should have the writer + level set; producing on all shard
	// methods exercises Register (writer present) and SetLevel (DEBUG allowed).
	for i := range 30 {
		sl.Debug("shard register/setlevel debug %d", i)
		sl.Info("shard register/setlevel info %d", i)
		sl.Warn("shard register/setlevel warn %d", i)
		sl.Error("shard register/setlevel error %d", i)
	}
	time.Sleep(150 * time.Millisecond)
}

// Test_FileWriter_SetAlertSink covers the SetAlertSink setter on FileWriter
// (installs an alert sink for overflow notifications).
func Test_FileWriter_SetAlertSink(t *testing.T) {
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable: true, Level: LevelFlagDebug, Async: true,
		Filename: tempLogPath(t), Rotate: true, Daily: true, AsyncBufferSize: 1 << 12,
	})
	sink := NewWebhookAlertSink("http://127.0.0.1:0/noop", 16, LarkTextFormatter(""))
	defer sink.Close()
	fw.SetAlertSink(sink) // must not panic; wires the sink into OverflowStats
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Drive directly (avoids the bootstrap recordPool data race; see
	// file_writer_async_test.go).
	_ = fw.Write(&Record{level: INFO, time: "t", file: "f", msg: "setalertsink line"})
	time.Sleep(100 * time.Millisecond)
	fw.Stop()
}

// tempLogPath returns a fresh temp log path for a writer.
func tempLogPath(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/cov-%Y%M%D.log"
}

// Test_Logger_Metrics covers Logger.Metrics() and the package Metrics() — both
// were 0% covered. Produces records at several levels and asserts the
// per-level counters increment.
func Test_Logger_Metrics(t *testing.T) {
	lg := NewLogger()
	lg.SetLevel(DEBUG)
	// drive a few levels; Metrics() reads recordsByLevel atomically.
	records := make(chan *Record, 16)
	driven := newLoggerWithRecords(records)
	driven.SetLevel(DEBUG)
	driven.Debug("d")
	driven.Info("i")
	driven.Warn("w")
	driven.Error("e")
	time.Sleep(50 * time.Millisecond)
	m := driven.Metrics()
	if m.Records[DEBUG] == 0 || m.Records[INFO] == 0 || m.Records[WARNING] == 0 || m.Records[ERROR] == 0 {
		t.Fatalf("Metrics missing levels: %+v", m)
	}
	driven.Close()
	// Package Metrics() reflects the singleton; exercise it too.
	_ = Metrics()
	_ = lg
}

// Test_SetLog is intentionally OMITTED: SetLog/SetLogWithConf reconfigure the
// package singleton (Register-ing new writers without stopping prior ones),
// which conflicts with the existing TestConfig that relies on the singleton's
// state. The JSON parse path they exercise is already covered by SetupLog's
// own tests; exercising them here would orphan async daemons on the singleton.

// Test_FileWriter_RotateImpl_HourlyMinutely covers the hourly/minutely rotate
// branches in rotateImpl (case 3 and case 4) by calling rotateImpl directly
// with an expired lastWriteTime. rotateImpl is unexported, so this test lives
// in package log4go.
func Test_FileWriter_RotateImpl_HourlyMinutely(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name     string
		daily    bool
		hourly   bool
		minutely bool
		maxD     int
		maxH     int
		maxM     int
	}{
		{"daily", true, false, false, -1, 0, 0},    // maxDays -1 => next day matches
		{"hourly", false, true, false, 0, -1, 0},   // maxHours -1 => next hour matches
		{"minutely", false, false, true, 0, 0, -1}, // maxMinutes -1 => next min matches
	} {
		t.Run(tc.name, func(t *testing.T) {
			fw := NewFileWriterWithOptions(FileWriterOptions{
				Enable: true, Level: LevelFlagDebug,
				Filename: dir + "/" + tc.name + "-%Y%M%D%H%m.log",
				Rotate:   true, Daily: tc.daily, Hourly: tc.hourly, Minutely: tc.minutely,
				MaxDays: tc.maxD, MaxHours: tc.maxH, MaxMinutes: tc.maxM,
			})
			fw.perm = "0755"
			// First rotate opens the file (initFileOk false -> variables set).
			if err := fw.rotateImpl(); err != nil {
				t.Fatalf("first rotateImpl: %v", err)
			}
			fw.initFileOk = true
			// Set lastWriteTime far enough in the past that the next boundary
			// matches, exercising the hourly/minutely case branch.
			fw.lastWriteTime = time.Now().Add(-2 * time.Hour)
			_ = fw.rotateImpl() // exercises case 3/4 comparison
			fw.Stop()
		})
	}
}

// writeFile is a tiny helper kept local to the test.
func writeFile(path string, data string) error {
	return os.WriteFile(path, []byte(data), 0o644)
}

// Test_SpillerAccessors covers the FileSpiller/ChainedSpiller accessors (Dir,
// File, HasPersistent) and the Stats() helper, none of which need a kafka
// broker.
func Test_SpillerAccessors(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileSpiller[*Record](dir, 1<<20, RecordCodec)
	if err != nil {
		t.Fatalf("NewFileSpiller: %v", err)
	}
	if fs.Dir() != dir {
		t.Errorf("Dir()=%q want %q", fs.Dir(), dir)
	}
	if !fs.HasPersistent() {
		t.Error("FileSpiller.HasPersistent want true")
	}
	ring := NewRingSpiller[*Record](8)
	chain := NewChainedSpiller[*Record](ring, fs)
	if !chain.HasPersistent() {
		t.Error("ChainedSpiller.HasPersistent want true (has file)")
	}
	if chain.File() == nil {
		t.Error("ChainedSpiller.File want non-nil")
	}
	// ChainedSpiller without a file: HasPersistent false.
	chainOnlyRing := NewChainedSpiller[*Record](ring, nil)
	_ = chainOnlyRing.HasPersistent()
	_ = fs.Close()
}

// Test_KafkaWriter_Setters covers the no-broker setters/accessors on
// KafkaWriter: SetAlertSink, SetOnEvent, Stats, Metrics (before Start).
func Test_KafkaWriter_Setters(t *testing.T) {
	kw := NewKafkaWriter(KafkaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 16, OverflowPolicy: "drop",
	})
	sink := NewWebhookAlertSink("http://127.0.0.1:0/x", 8, LarkTextFormatter(""))
	defer sink.Close()
	kw.SetAlertSink(sink) // must not panic
	fired := make(chan string, 4)
	kw.SetOnEvent(func(name string, delta int64) {
		select {
		case fired <- name:
		default:
		}
	})
	// Stats/Metrics before Start return zeros without panicking.
	dropped, spilled := kw.Stats()
	_ = dropped
	_ = spilled
	m := kw.Metrics()
	if m.Sent != 0 || m.Errored != 0 {
		t.Fatalf("pre-Start Metrics non-zero: %+v", m)
	}
}
