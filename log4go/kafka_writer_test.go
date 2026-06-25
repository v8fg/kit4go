package log4go

import (
	"runtime"
	"strings"
	"testing"

	"github.com/IBM/sarama"
)

// Test_KafKaWriter_SendDrop verifies that drop policy discards when full and
// counts the drop (never blocks the caller, never spawns a goroutine).
func Test_KafKaWriter_SendDrop(t *testing.T) {
	w := &KafKaWriter{policy: OverflowDrop, messages: make(chan *sarama.ProducerMessage, 1)}
	w.messages <- spillerMsg("t", "1") // fill the single-slot channel

	w.send(spillerMsg("t", "2")) // must drop, not block
	if got := w.stats.Dropped(); got != 1 {
		t.Fatalf("dropped=%d want 1", got)
	}
}

// Test_KafKaWriter_SendSpillRing verifies spill policy routes overflow to the
// ring store and Drain recovers it.
func Test_KafKaWriter_SendSpillRing(t *testing.T) {
	w := &KafKaWriter{
		policy:   OverflowSpill,
		spiller:  NewRingSpiller[*sarama.ProducerMessage](8),
		messages: make(chan *sarama.ProducerMessage, 1),
	}
	w.messages <- spillerMsg("t", "1") // full

	w.send(spillerMsg("t", "2")) // spill
	if got := w.stats.Spilled(); got != 1 {
		t.Fatalf("spilled=%d want 1", got)
	}
	if w.spiller.Len() != 1 {
		t.Fatalf("spiller Len=%d want 1", w.spiller.Len())
	}
	out := w.spiller.Drain()
	if len(out) != 1 {
		t.Fatalf("drain=%d want 1", len(out))
	}
}

// Test_KafKaWriter_SendBlock verifies block policy blocks (backs up) instead of
// dropping/spilling. We assert it by filling the channel and confirming the
// next value is still pending (no drop counter increment).
func Test_KafKaWriter_SendBlock(t *testing.T) {
	w := &KafKaWriter{policy: OverflowBlock, messages: make(chan *sarama.ProducerMessage, 1)}
	w.messages <- spillerMsg("t", "1") // full

	done := make(chan struct{})
	go func() {
		w.send(spillerMsg("t", "2")) // blocks until space
		close(done)
	}()
	// still blocked (no drop, no spill)
	if w.stats.Dropped() != 0 || w.stats.Spilled() != 0 {
		t.Fatal("block policy must not drop/spill")
	}
	<-w.messages // free one slot
	<-done       // send completes
	if w.stats.Dropped() != 0 {
		t.Fatal("block policy dropped unexpectedly")
	}
}

// Test_KafKaWriter_WriteNoGoroutineBurst is the core OOM-prevention check: a
// burst of Write calls must NOT spawn per-record goroutines (the old impl did).
func Test_KafKaWriter_WriteNoGoroutineBurst(t *testing.T) {
	w := &KafKaWriter{
		level:    INFO,
		policy:   OverflowDrop,
		messages: make(chan *sarama.ProducerMessage, 1000),
		options:  KafKaWriterOptions{ProducerTopic: "t"},
	}
	before := runtime.NumGoroutine()
	for i := 0; i < 10000; i++ {
		if err := w.Write(&Record{level: INFO, msg: "burst message"}); err != nil {
			t.Fatal(err)
		}
	}
	after := runtime.NumGoroutine()
	// allow small slack for background bootstrap goroutines (global loggerDefault)
	if after > before+2 {
		t.Errorf("Write spawned goroutines: before=%d after=%d (allow 2 for background bootstrap)", before, after)
	}
}

// Test_KafKaWriter_BuildPayload verifies single-pass JSON and ExtraFields hoist.
func Test_KafKaWriter_BuildPayload(t *testing.T) {
	w := &KafKaWriter{options: KafKaWriterOptions{
		ProducerTopic: "t",
		MSG: KafKaMSGFields{
			ServerIP: "1.2.3.4",
			ExtraFields: map[string]interface{}{
				"request_id": "abc",
				"level":      "SHADOW", // must NOT override built-in "level"
			},
		},
	}}
	b := w.buildPayload(&Record{level: ERROR, msg: "boom", file: "f.go:9"})
	if b == nil {
		t.Fatal("nil payload")
	}
	s := string(b)
	for _, want := range []string{`"server_ip":"1.2.3.4"`, `"request_id":"abc"`, `"level":"ERROR"`, `"message":"boom"`} {
		if !strings.Contains(s, want) {
			t.Errorf("payload missing %q\ngot: %s", want, s)
		}
	}
}

// Benchmark_KafKaWriter_buildPayload measures the per-record JSON cost (hot path).
func Benchmark_KafKaWriter_buildPayload(b *testing.B) {
	w := &KafKaWriter{options: KafKaWriterOptions{
		ProducerTopic: "t",
		MSG:           KafKaMSGFields{ServerIP: "1.2.3.4", ExtraFields: map[string]interface{}{"rid": "x"}},
	}}
	r := &Record{level: INFO, msg: "benchmark message payload", file: "f.go:1"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.buildPayload(r)
	}
}

// Benchmark_KafKaWriter_buildPayload_baseFields measures the realistic Kafka→ES
// path: a record carrying Base Fields (hostname/server_ip/app/es_index — set once
// via SetBaseField) plus a per-request trace field, all flowing through r.fields.
// This is the common integrated configuration and exercises the manual-append
// slow path (no map marshal).
func Benchmark_KafKaWriter_buildPayload_baseFields(b *testing.B) {
	w := &KafKaWriter{options: KafKaWriterOptions{
		ProducerTopic: "t",
		MSG:           KafKaMSGFields{}, // legacy struct empty — Base Fields are the source of truth
	}}
	r := &Record{
		level:    INFO,
		msg:      "benchmark message payload",
		file:     "f.go:1",
		unixNano: 1782392990_123456789,
		seq:      1234567,
		fields: []field{
			{key: "hostname", val: "adx-prod-01"},
			{key: "server_ip", val: "10.0.1.5"},
			{key: "app", val: "adx-dsp"},
			{key: "es_index", val: "adx-logs-2026.06"},
			{key: "trace_id", val: "a1b2c3d4e5f6"},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.buildPayload(r)
	}
}
