package log4go

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

// decodeNumber unmarshals JSON preserving full int64 precision (json.Number) so
// large values like unix_nano are not rounded through float64.
func decodeNumber(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("payload not JSON: %v\n%s", err, b)
	}
	return m
}

// intField extracts a JSON number field as int64 (via json.Number.Int64).
func numField(t *testing.T, m map[string]any, k string) int64 {
	t.Helper()
	n, ok := m[k].(json.Number)
	if !ok {
		t.Fatalf("field %q is not a number: %v", k, m[k])
	}
	v, err := n.Int64()
	if err != nil {
		t.Fatalf("field %q not int64: %v", k, err)
	}
	return v
}

// Test_KafkaWriter_Payload_BaseFieldPriority is the core Kafka→ES integration
// invariant: fields carried on the Record (Base Fields via SetBaseField, plus
// With/Context) take priority over the legacy KafkaMSGFields struct members.
// A Base Field "server_ip" must override MSG.ServerIP; where no Base Field is
// supplied ("es_index" here), the MSG struct member still works as a fallback.
func Test_KafkaWriter_Payload_BaseFieldPriority(t *testing.T) {
	w := &KafkaWriter{options: KafkaWriterOptions{
		ProducerTopic: "t",
		MSG: KafkaMSGFields{
			ServerIP: "from-msg",     // overridden by the base field below
			ESIndex:  "from-msg-idx", // no base field for es_index -> fallback
		},
	}}
	r := &Record{
		level:    INFO,
		msg:      "hi",
		file:     "s.go:1",
		unixNano: 1782392990_123456789,
		seq:      42,
		fields: []field{
			fld("server_ip", "from-base"), // base field wins over MSG.ServerIP
			fld("trace_id", "t1"),
		},
	}
	b := w.buildPayload(r)
	if b == nil {
		t.Fatal("nil payload")
	}
	m := decodeNumber(t, b)
	cases := []struct{ k, want string }{
		{"server_ip", "from-base"},   // base field overrides MSG.ServerIP
		{"es_index", "from-msg-idx"}, // no base field -> MSG fallback
		{"trace_id", "t1"},
		{"message", "hi"},
		{"level", "INFO"},
	}
	for _, c := range cases {
		if got, _ := m[c.k].(string); got != c.want {
			t.Errorf("payload[%q]=%v want %q", c.k, m[c.k], c.want)
		}
	}
	// strict-ordering keys are present and carry the record's exact values (ES
	// sorts on seq then unix_nano). Decode with json.Number so the 18-digit
	// unix_nano is not rounded through float64.
	if got := numField(t, m, "unix_nano"); got != r.unixNano {
		t.Errorf("payload[unix_nano]=%d want %d", got, r.unixNano)
	}
	if got := numField(t, m, "seq"); got != int64(r.seq) {
		t.Errorf("payload[seq]=%d want %d", got, r.seq)
	}
}

// Test_KafkaWriter_Payload_TimestampFromRecordTime guards the "one clock read per
// record" invariant: the payload timestamp is derived from r.unixNano (captured
// once in deliverRecordToWriter), not a second time.Now(). It must be the ISO
// layout (RFC3339-ish, timezone-aware) so ES auto-maps it to its date type.
func Test_KafkaWriter_Payload_TimestampFromRecordTime(t *testing.T) {
	w := &KafkaWriter{options: KafkaWriterOptions{ProducerTopic: "t"}}
	const fixed int64 = 1782392990_123456789
	r := &Record{level: INFO, msg: "x", unixNano: fixed, seq: 7}

	b := w.buildPayload(r)
	m := decodeNumber(t, b)
	want := time.Unix(0, fixed).UTC().Format(timestampLayout) // UTC (Z), unified across JSON/Kafka
	if got, _ := m["timestamp"].(string); got != want {
		t.Errorf("timestamp=%q want %q", got, want)
	}
	// ISO/RFC3339 shape: a 'T' date/time separator and a zone offset or Z.
	if !strings.Contains(want, "T") {
		t.Errorf("timestamp %q is not ISO (no 'T' separator)", want)
	}
	// "now" is the unix-seconds companion, also from the record time.
	if got := numField(t, m, "now"); got != fixed/1e9 {
		t.Errorf("now=%d want %d", got, fixed/1e9)
	}
}

// Test_KafkaWriter_Payload_FastPathOmitsEmptyRouting confirms the no-fields fast
// path omits empty routing fields (es_index/server_ip/file) so ES documents are
// not polluted with empty strings, while always emitting the framework +
// ordering fields.
func Test_KafkaWriter_Payload_FastPathOmitsEmptyRouting(t *testing.T) {
	w := &KafkaWriter{options: KafkaWriterOptions{ProducerTopic: "t"}} // no MSG, no fields
	r := &Record{level: INFO, msg: "x", unixNano: 1, seq: 1}
	b := w.buildPayload(r)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("payload not JSON: %v\n%s", err, b)
	}
	for _, k := range []string{"es_index", "server_ip", "file"} {
		if _, ok := m[k]; ok {
			t.Errorf("empty %q should be omitted, got %v", k, m[k])
		}
	}
	for _, k := range []string{"unix_nano", "seq", "level", "message", "timestamp", "now"} {
		if _, ok := m[k]; !ok {
			t.Errorf("framework field %q missing from fast-path payload", k)
		}
	}
}

// Test_KafkaWriter_Payload_MSGFallback confirms the legacy MSG struct routing
// members still emit (as fallback) when the Record carries no matching field —
// backward compatibility for callers that configure KafkaMSGFields directly.
func Test_KafkaWriter_Payload_MSGFallback(t *testing.T) {
	w := &KafkaWriter{options: KafkaWriterOptions{
		ProducerTopic: "t",
		MSG:           KafkaMSGFields{ServerIP: "10.0.0.1", ESIndex: "logs-2026.06"},
	}}
	r := &Record{level: WARNING, msg: "m", unixNano: 2, seq: 2}
	b := w.buildPayload(r)
	var m map[string]any
	json.Unmarshal(b, &m)
	if got, _ := m["server_ip"].(string); got != "10.0.0.1" {
		t.Errorf("server_ip=%v want 10.0.0.1 (MSG fallback)", m["server_ip"])
	}
	if got, _ := m["es_index"].(string); got != "logs-2026.06" {
		t.Errorf("es_index=%v want logs-2026.06 (MSG fallback)", m["es_index"])
	}
}

// Test_SetFormat_JSON_CarriesBaseFields guards the base-field JSON bug: under
// FormatJSON, records must include SetBaseField fields (previously the JSON path
// serialized l.fields only, dropping base fields from every JSON writer and the
// Kafka payload).
func Test_SetFormat_JSON_CarriesBaseFields(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)
	root.SetFormat(FormatJSON)
	root.SetBaseField("hostname", "adx-prod-01")
	root.SetBaseField("app", "adx-dsp")

	cw := &captureWriter{}
	root.Register(cw)

	root.With("trace_id", "t-9").Info("hello")

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
	if len(r.formattedBytes) == 0 {
		t.Fatal("formattedBytes empty under FormatJSON")
	}
	var m map[string]any
	if err := json.Unmarshal(r.formattedBytes, &m); err != nil {
		t.Fatalf("formattedBytes not valid JSON: %v\n%s", err, r.formattedBytes)
	}
	fields, _ := m["fields"].(map[string]any)
	for k, want := range map[string]string{"hostname": "adx-prod-01", "app": "adx-dsp", "trace_id": "t-9"} {
		if fields[k] != want {
			t.Errorf("fields.%s=%v want %q (base field dropped from JSON?)", k, fields[k], want)
		}
	}
}

// Test_BaseField_PropagatesToChildLoggers guards the clone() sharing fix: base
// fields set on the root must appear on records emitted by child Loggers created
// via With (the common usage). It also checks the live-sharing invariant — a
// SetBaseField on the root AFTER a child was created is visible to that child.
func Test_BaseField_PropagatesToChildLoggers(t *testing.T) {
	records := make(chan *Record, 4)
	root := newLoggerWithRecords(records)
	defer root.Close()
	root.SetLevel(DEBUG)
	root.SetBaseField("app", "adx-dsp")

	child := root.With("trace_id", "t-1")
	// set a base field AFTER the child exists — must still propagate (shared holder)
	root.SetBaseField("env", "prod")

	cw := &captureWriter{}
	root.Register(cw)
	child.Info("via child")

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
	got := map[string]any{}
	for _, f := range r.fields {
		got[f.key] = f.value()
	}
	for k, want := range map[string]string{"app": "adx-dsp", "env": "prod", "trace_id": "t-1"} {
		if got[k] != want {
			t.Errorf("child record field %q=%v want %q (base field not shared to child?)", k, got[k], want)
		}
	}
}
