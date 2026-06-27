package log4go

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test_RecordString_NoFields confirms the no-With fast path still produces the
// canonical format with no trailing JSON object (the regression guard for the
// fields append being conditional).
func Test_RecordString_NoFields(t *testing.T) {
	r := &Record{level: INFO, time: "t", file: "f.go:1", msg: "hello"}
	got := r.String()
	want := "#0 t [INFO] <f.go:1> hello\n"
	if got != want {
		t.Fatalf("no-fields output changed:\n got=%q\nwant=%q", got, want)
	}
}

// Test_RecordString_WithFields verifies structured fields render as a trailing
// JSON object on the text format.
func Test_RecordString_WithFields(t *testing.T) {
	r := &Record{
		level:  INFO,
		time:   "t",
		file:   "f.go:1",
		msg:    "hello",
		fields: []field{fld("trace_id", "abc"), fld("user", 42)},
	}
	got := r.String()
	if !strings.Contains(got, "t [INFO] <f.go:1> hello ") {
		t.Fatalf("prefix wrong: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("missing newline: %q", got)
	}
	// extract the JSON object between the trailing space and the newline
	fj := strings.TrimSpace(strings.TrimPrefix(got[strings.Index(got, "t [INFO]"):], "t [INFO] <f.go:1> hello "))
	fj = strings.TrimSuffix(fj, "\n")
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(fj), &m); err != nil {
		t.Fatalf("fields not valid JSON (%q): %v", fj, err)
	}
	if m["trace_id"] != "abc" {
		t.Errorf("trace_id=%v want abc", m["trace_id"])
	}
	if num, ok := m["user"].(float64); !ok || num != 42 {
		t.Errorf("user=%v want 42", m["user"])
	}
}

// Test_LoggerWith_Chainable verifies With returns a child carrying accumulated
// fields and does NOT mutate the parent.
func Test_LoggerWith_Chainable(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 16))
	defer root.Close()

	child := root.With("a", 1).With("b", 2)
	if len(root.fields) != 0 {
		t.Fatalf("parent mutated: root has %d fields", len(root.fields))
	}
	if len(child.fields) != 2 {
		t.Fatalf("child fields=%d want 2", len(child.fields))
	}
	got := map[string]interface{}{}
	for _, f := range child.fields {
		got[f.key] = f.value()
	}
	if got["a"] != 1 || got["b"] != 2 {
		t.Fatalf("child fields wrong: %v", got)
	}
}

// Test_LoggerWithFields_Map verifies WithFields attaches a whole map in one
// clone and is immune to later mutation of the input map.
func Test_LoggerWithFields_Map(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 16))
	defer root.Close()

	in := map[string]interface{}{"k1": "v1", "k2": 99}
	child := root.WithFields(in)
	in["k3"] = "sneaky" // mutate after — must NOT appear on the child
	if len(child.fields) != 2 {
		t.Fatalf("child fields=%d want 2", len(child.fields))
	}
	for _, f := range child.fields {
		if f.key == "k3" {
			t.Fatal("child picked up post-WithFields mutation")
		}
	}
}

// Test_LoggerWith_ParentIndependent confirms two children of the same parent do
// not share field state (each With copies before append).
func Test_LoggerWith_ParentIndependent(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 16))
	defer root.Close()

	c1 := root.With("x", 1)
	c2 := root.With("y", 2)
	if len(c1.fields) != 1 || c1.fields[0].key != "x" {
		t.Fatalf("c1 wrong: %v", c1.fields)
	}
	if len(c2.fields) != 1 || c2.fields[0].key != "y" {
		t.Fatalf("c2 wrong: %v", c2.fields)
	}
}

// Test_LoggerWith_DeliversFieldsToRecord is the end-to-end check: a record
// emitted by a With-child Logger carries the fields into Record.String(). Uses a
// captureWriter (the only valid reader of the records channel besides the
// bootstrap goroutine).
func Test_LoggerWith_DeliversFieldsToRecord(t *testing.T) {
	records := make(chan *Record, 4)
	lg := newLoggerWithRecords(records)
	defer lg.Close()
	lg.SetLevel(DEBUG)

	cw := &captureWriter{}
	lg.Register(cw)

	child := lg.With("trace_id", "t-123").WithField("user", 7)
	child.Info("request handled")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		runtime.Gosched()
	}
	if cw.Len() == 0 {
		t.Fatal("record never reached writer")
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()
	if r.fields == nil || len(r.fields) != 2 {
		t.Fatalf("record fields=%v want 2", r.fields)
	}
	s := r.String()
	if !strings.Contains(s, `"trace_id":"t-123"`) || !strings.Contains(s, `"user":7`) {
		t.Fatalf("String() missing fields: %q", s)
	}
}

// Test_KafKaWriter_BuildPayload_RecordFields verifies the kafka JSON payload
// hoists record-level With fields to the top level, behind built-in fields.
func Test_KafKaWriter_BuildPayload_RecordFields(t *testing.T) {
	w := &KafKaWriter{options: KafKaWriterOptions{
		ProducerTopic: "t",
		MSG: KafKaMSGFields{
			ServerIP: "1.2.3.4",
			ExtraFields: map[string]interface{}{
				"global_tag": "prod",
				"level":      "SHADOW", // built-in wins
			},
		},
	}}
	r := &Record{
		level: ERROR,
		msg:   "boom",
		file:  "f.go:9",
		fields: []field{
			fld("trace_id", "abc"),
			fld("level", "OVERRIDE"), // built-in wins, not record field
		},
	}
	b := w.buildPayload(r)
	if b == nil {
		t.Fatal("nil payload")
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("payload not JSON: %v\n%s", err, b)
	}
	for _, c := range []struct{ k, want string }{
		{"server_ip", "1.2.3.4"},
		{"trace_id", "abc"},
		{"global_tag", "prod"},
		{"level", "ERROR"}, // built-in beats both record field and ExtraFields
		{"message", "boom"},
	} {
		got, _ := m[c.k].(string)
		if got != c.want {
			t.Errorf("payload[%q]=%v want %q", c.k, m[c.k], c.want)
		}
	}
}

// Test_Sampler_Policy is the core sampling math check: initial=5, thereafter=10
// => first 5 emitted, then 1-in-10 thereafter. Over 95 records (5 + 90) that is
// 5 + 9 = 14 emitted.
func Test_Sampler_Policy(t *testing.T) {
	s := newSampler(5, 10)
	const total = 95
	emitted := 0
	for i := 0; i < total; i++ {
		if s.allow(INFO) {
			emitted++
		}
	}
	// records 1..5 emitted (5), then (6..95) => 90 records, period 10 => 9 more.
	want := 5 + 9
	if emitted != want {
		t.Fatalf("emitted=%d want %d (initial=5, thereafter=10, total=%d)", emitted, want, total)
	}
}

// Test_Sampler_PerLevel confirms counts are independent per level (a DEBUG flood
// must not advance the ERROR counter). 500 DEBUG allow() calls must leave the
// ERROR counter at 0 until an ERROR allow() is made.
func Test_Sampler_PerLevel(t *testing.T) {
	s := newSampler(0, 100) // emit 1-in-100 from the start
	// flood DEBUG
	for i := 0; i < 500; i++ {
		s.allow(DEBUG)
	}
	// ERROR counter must be untouched by the DEBUG flood.
	if errN := atomic.LoadUint64(&s.counts[ERROR]); errN != 0 {
		t.Fatalf("DEBUG flood advanced ERROR counter to %d", errN)
	}
	if debugN := atomic.LoadUint64(&s.counts[DEBUG]); debugN != 500 {
		t.Fatalf("DEBUG counter=%d want 500", debugN)
	}
	// one ERROR allow() advances ERROR counter to exactly 1.
	s.allow(ERROR)
	if errN := atomic.LoadUint64(&s.counts[ERROR]); errN != 1 {
		t.Fatalf("ERROR counter=%d want 1 after one allow", errN)
	}
}

// captureWriter is a test Writer that records every Record handed to Write,
// for asserting what actually reached the writer pipeline. It does NOT share
// state with the bootstrap goroutine beyond a mutex-guarded slice.
type captureWriter struct {
	mu      sync.Mutex
	records []*Record
}

func (c *captureWriter) Init() error { return nil }
func (c *captureWriter) Write(r *Record) error {
	// Copy because the bootstrap goroutine returns r to the recordPool after
	// Write returns; holding the pointer would race with reuse.
	cr := *r
	c.mu.Lock()
	c.records = append(c.records, &cr)
	c.mu.Unlock()
	return nil
}
func (c *captureWriter) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.records)
}

// Test_LoggerWithSampling_EndToEnd drives sampling through the full
// deliverRecordToWriter path: 1000 INFO records with sampling{10, 100} should
// reach exactly 19 records at the writer (10 initial + 9 thereafter). The
// captureWriter is the only writer, so its count == emitted count. Sampled-out
// records must NOT inflate Metrics.
func Test_LoggerWithSampling_EndToEnd(t *testing.T) {
	records := make(chan *Record, 2048)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)

	cw := &captureWriter{}
	root.Register(cw)

	child := root.WithSampling(10, 100)
	for i := 0; i < 1000; i++ {
		child.Info("sampled line %d", i)
	}

	// The bootstrap goroutine drains records async; wait until the writer has
	// seen all 19 emitted records (the expected count) or time out.
	want := 10 + 9
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cw.Len() >= want {
			break
		}
		runtime.Gosched()
	}
	if got := cw.Len(); got != want {
		t.Errorf("reached writer=%d want %d", got, want)
	}
	// Metrics must reflect emitted count, not the 1000 attempts. Children share
	// the root's counter array, so root.Metrics() sees the child's emits.
	m := root.Metrics()
	if m.Records[INFO] != uint64(want) {
		t.Errorf("Metrics INFO=%d want %d (sampled records must not inflate counters)", m.Records[INFO], want)
	}
}

// Test_LoggerWithSampling_Disable confirms initial<=0 && thereafter<=0 returns a
// logger with no sampler (no-op).
func Test_LoggerWithSampling_Disable(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	child := root.WithSampling(0, 0)
	if child.sampler.Load() != nil {
		t.Fatal("WithSampling(0,0) must produce a nil sampler")
	}
}

// Test_Context_DefaultExtractor verifies WithContext uses the default trace-id
// key lookup when no custom extractor is set.
func Test_Context_DefaultExtractor(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()

	ctx := context.WithValue(context.Background(), "trace_id", "trace-xyz")
	ctx = context.WithValue(ctx, "x-request-id", "req-1")
	child := root.WithContext(ctx)

	if len(child.fields) != 2 {
		t.Fatalf("child fields=%d want 2 (trace_id + x-request-id)", len(child.fields))
	}
	got := map[string]interface{}{}
	for _, f := range child.fields {
		got[f.key] = f.value()
	}
	if got["trace_id"] != "trace-xyz" || got["x-request-id"] != "req-1" {
		t.Fatalf("context fields wrong: %v", got)
	}
}

// Test_Context_CustomExtractor verifies SetContextExtractor overrides the default.
func Test_Context_CustomExtractor(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	root.SetContextExtractor(func(ctx context.Context) map[string]interface{} {
		return map[string]interface{}{"span_id": "s-1", "custom": true}
	})

	child := root.WithContext(context.Background())
	got := map[string]interface{}{}
	for _, f := range child.fields {
		got[f.key] = f.value()
	}
	if got["span_id"] != "s-1" || got["custom"] != true {
		t.Fatalf("custom extractor fields wrong: %v", got)
	}
}

// Test_Context_NilContext confirms WithContext(nil) is a safe no-op.
func Test_Context_NilContext(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	child := root.WithContext(nil)
	if len(child.fields) != 0 {
		t.Fatalf("nil ctx produced %d fields", len(child.fields))
	}
}

// Test_Context_EmptyContext confirms a context with no trace keys yields a child
// with no extra fields (the default extractor returns nil -> no append).
func Test_Context_EmptyContext(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	child := root.WithContext(context.Background())
	if len(child.fields) != 0 {
		t.Fatalf("empty ctx produced %d fields", len(child.fields))
	}
}

// Test_RecordPool_FieldsReset verifies the bootstrap goroutine clears r.fields
// before returning the record to the pool, so a pooled record doesn't pin a
// stale fields slice. We register a captureWriter (the only reader of the
// records channel is the bootstrap goroutine), emit a With-field record, wait
// for it to be written (which is when the bootstrap has run Put), then grab a
// fresh record from the pool and check its fields are nil.
func Test_RecordPool_FieldsReset(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)

	cw := &captureWriter{}
	root.Register(cw)

	child := root.With("trace_id", "t-1")
	child.Info("first") // carries fields, gets pooled after Write

	// wait until the captureWriter saw the record — at that point bootstrap has
	// also run recordPool.Put(r) (Put happens right after Write in the loop).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
		runtime.Gosched()
	}
	if cw.Len() == 0 {
		t.Fatal("record never reached writer")
	}
	// grab a record from the pool directly — it should be the same pooled record
	// with fields cleared.
	r := recordPool.Get().(*Record)
	if r.fields != nil {
		t.Fatalf("pooled record still carries fields: %v", r.fields)
	}
	recordPool.Put(r)
}

// Test_LoggerClone_ConcurrentSafe is a light race detector exercise: many
// goroutines building distinct children and emitting through them must not trip
// the race detector. Run with -race.
func Test_LoggerClone_ConcurrentSafe(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4096))
	defer root.Close()
	root.SetLevel(DEBUG)

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cl := root.With("g", id)
			for i := 0; i < 100; i++ {
				cl.With("i", i).Info("concurrent %d/%d", id, i)
			}
		}(g)
	}
	wg.Wait()
}

// Test_FieldsJSON_MarshalError exercises the error branch of FieldsJSON by
// injecting a failing json.Marshal (a value json.Marshal cannot encode). The
// method must return "" rather than panic.
func Test_FieldsJSON_MarshalError(t *testing.T) {
	// Typed scalars never reach the JSON codec, so they cannot fail. A kindAny
	// value JSON cannot encode (a channel) degrades to null in place — FieldsJSON
	// keeps the field (never silently drops it) and never panics.
	r := &Record{level: INFO, time: "t", file: "f", msg: "m",
		fields: []field{anyField("k", make(chan int))}}
	if got := r.FieldsJSON(); got != `{"k":null}` {
		t.Fatalf("FieldsJSON on unmarshallable any = %q, want {\"k\":null}", got)
	}
	// String() still renders the canonical line (fields appended after).
	s := r.String()
	if !strings.Contains(s, "t [INFO] <f> m") {
		t.Fatalf("String() wrong on unmarshallable field: %q", s)
	}
}
