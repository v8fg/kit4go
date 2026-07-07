package log4go

import (
	"strings"
	"testing"

	"github.com/v8fg/kit4go/kafka"
)

func TestKafkaCodec_JSON_Default(t *testing.T) {
	w := NewKafkaWriter(KafkaWriterOptions{ProducerTopic: "t", BufferSize: 16})
	r := &Record{level: INFO, msg: "hello", file: "svc.go:42", unixNano: 1782563343536622000, seq: 42}
	payload := w.buildPayload(r)
	if len(payload) == 0 {
		t.Fatal("JSON payload empty")
	}
	if payload[0] != '{' {
		t.Errorf("JSON payload should start with '{', got %c", payload[0])
	}
}

func TestKafkaCodec_Proto_SmallerThanJSON(t *testing.T) {
	w := NewKafkaWriter(KafkaWriterOptions{ProducerTopic: "t", BufferSize: 16})
	r := &Record{level: INFO, msg: "hello world ad request served", file: "svc.go:42", unixNano: 1782563343536622000, seq: 42}
	jsonPayload := w.buildPayload(r)
	w.SetKafkaCodec(KafkaCodecProto{})
	protoPayload := w.buildPayload(r)
	if len(protoPayload) >= len(jsonPayload) {
		t.Errorf("proto (%dB) should be smaller than json (%dB)", len(protoPayload), len(jsonPayload))
	}
	t.Logf("json=%dB proto=%dB (%.0f%%)", len(jsonPayload), len(protoPayload), float64(len(protoPayload))/float64(len(jsonPayload))*100)
}

func TestKafkaCodec_Switch(t *testing.T) {
	w := NewKafkaWriter(KafkaWriterOptions{ProducerTopic: "t", BufferSize: 16})
	r := &Record{level: INFO, msg: "x", unixNano: 1, seq: 1}
	j := w.buildPayload(r)
	w.SetKafkaCodec(KafkaCodecProto{})
	p := w.buildPayload(r)
	w.SetKafkaCodec(nil) // back to JSON
	j2 := w.buildPayload(r)
	if string(j) != string(j2) {
		t.Error("switch back to JSON should produce same output")
	}
	if string(j) == string(p) {
		t.Error("proto should differ from json")
	}
}

func TestKafkaCodec_ContentTypes(t *testing.T) {
	jc := KafkaCodecJSON{}
	if jc.ContentType() != "application/json" {
		t.Error("json content type")
	}
	pc := KafkaCodecProto{}
	if pc.ContentType() != "application/x-protobuf" {
		t.Error("proto content type")
	}
}

// TestSetKafkaCodec_PackageLevel_AppliesToRegisteredWriters verifies the
// package-level SetKafkaCodec iterates the singleton's writers, applies the
// codec to each KafkaWriter (proto → JSON via nil), and skips non-Kafka writers
// without panic. Uses a sarama mock producer so Register→Start needs no broker.
func TestSetKafkaCodec_PackageLevel_AppliesToRegisteredWriters(t *testing.T) {

	w := NewKafkaWriter(KafkaWriterOptions{ProducerTopic: "t", BufferSize: 16})
	w.producerFactory = func() (kafka.Producer, error) {
		return newMockKafkaProducer(), nil
	}

	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	root.Register(w)               // Init -> Start via mock factory (no broker)
	root.Register(discardWriter{}) // non-Kafka writer: type-assert must skip it

	// swap the singleton so package-level SetKafkaCodec sees this logger.
	old := loggerDefault.Swap(root)
	defer func() { loggerDefault.Store(old) }()

	r := &Record{level: INFO, msg: "hello world ad request", unixNano: 1782563343536622000, seq: 1}

	// default codec is JSON
	if b := w.buildPayload(r); len(b) == 0 || b[0] != '{' {
		t.Fatalf("default codec should be JSON, got %v", b)
	}

	// package-level switch to protobuf applies to the registered KafkaWriter.
	SetKafkaCodec(KafkaCodecProto{})
	b := w.buildPayload(r)
	if len(b) == 0 || b[0] == '{' {
		t.Fatalf("SetKafkaCodec(Proto) should switch to protobuf, got %v", b)
	}

	// package-level nil restores JSON default.
	SetKafkaCodec(nil)
	if b := w.buildPayload(r); len(b) == 0 || b[0] != '{' {
		t.Fatalf("nil codec should restore JSON, got %v", b)
	}
}

// TestKafkaCodec_Proto_UserFields drives the protobuf user-field path
// (appendFieldProto -> scalarToJSON over every scalar kind, appendBytesField)
// by encoding a Record carrying business fields of each type. Asserts each
// value is rendered exactly as scalarToJSON produces it (strings inline,
// scalars JSON-encoded into the value sub-field).
func TestKafkaCodec_Proto_UserFields(t *testing.T) {
	w := NewKafkaWriter(KafkaWriterOptions{ProducerTopic: "t", BufferSize: 16})
	w.SetKafkaCodec(KafkaCodecProto{})
	r := &Record{
		level: INFO, msg: "bid served", file: "bidder.go:7",
		unixNano: 1782563343536622000, seq: 7,
		fields: []field{
			strField("trace_id", "abc123"),       // string -> inline
			intField("count", 42),                // scalarToJSON: int
			int64Field("n64", 7),                 // scalarToJSON: int64
			uint64Field("depth", 9),              // scalarToJSON: uint64
			floatField("bid_price", 0.5),         // scalarToJSON: float64
			boolField("served", true),            // scalarToJSON: bool true
			boolField("timeout", false),          // scalarToJSON: bool false
			anyField("missing", nil),             // scalarToJSON: nil -> "null"
			anyField("tags", []string{"a", "b"}), // scalarToJSON: default -> Sprintf
		},
	}
	b := w.buildPayload(r)
	if len(b) == 0 || b[0] == '{' {
		t.Fatalf("expected protobuf output, got %q", b)
	}
	// Every field key and its rendered value must appear in the proto bytes.
	body := string(b)
	for _, s := range []string{
		"trace_id", "abc123",
		"count", "42",
		"n64", "7",
		"depth", "9",
		"bid_price", "0.5",
		"served", "true",
		"timeout", "false",
		"missing", "null",
		"tags", "[a b]",
	} {
		if !strings.Contains(body, s) {
			t.Errorf("proto payload missing %q\nbody=%x", s, b)
		}
	}
}

// TestKafkaCodec_Proto_RoutingFields covers the ServerIP/ESIndex emission
// branches of KafkaCodecProto.Encode (legacy routing fields sourced from
// options.MSG). Both must appear in the proto bytes when set.
func TestKafkaCodec_Proto_RoutingFields(t *testing.T) {
	w := NewKafkaWriter(KafkaWriterOptions{
		ProducerTopic: "t", BufferSize: 16,
		MSG: KafkaMSGFields{ServerIP: "10.0.0.1", ESIndex: "adx-logs-2026.06"},
	})
	w.SetKafkaCodec(KafkaCodecProto{})
	r := &Record{level: INFO, msg: "x", unixNano: 1782563343536622000, seq: 1}
	b := w.buildPayload(r)
	body := string(b)
	if !strings.Contains(body, "10.0.0.1") {
		t.Errorf("proto missing ServerIP\nbody=%x", b)
	}
	if !strings.Contains(body, "adx-logs-2026.06") {
		t.Errorf("proto missing ESIndex\nbody=%x", b)
	}
}

func Benchmark_KafkaCodec_JSON(b *testing.B) {
	w := NewKafkaWriter(KafkaWriterOptions{ProducerTopic: "t", BufferSize: 16})
	r := &Record{level: INFO, msg: "bid served ad_id=123 price=0.5", file: "bidder.go:42", unixNano: 1782563343536622000, seq: 42}
	b.ReportAllocs()

	for b.Loop() {
		_ = w.buildPayload(r)
	}
}

func Benchmark_KafkaCodec_Proto(b *testing.B) {
	w := NewKafkaWriter(KafkaWriterOptions{ProducerTopic: "t", BufferSize: 16})
	w.SetKafkaCodec(KafkaCodecProto{})
	r := &Record{level: INFO, msg: "bid served ad_id=123 price=0.5", file: "bidder.go:42", unixNano: 1782563343536622000, seq: 42}
	b.ReportAllocs()

	for b.Loop() {
		_ = w.buildPayload(r)
	}
}
