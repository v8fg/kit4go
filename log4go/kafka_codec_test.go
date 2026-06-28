package log4go

import (
	"testing"
)

func TestKafkaCodec_JSON_Default(t *testing.T) {
	w := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "t", BufferSize: 16})
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
	w := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "t", BufferSize: 16})
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
	w := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "t", BufferSize: 16})
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

func Benchmark_KafKaCodec_JSON(b *testing.B) {
	w := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "t", BufferSize: 16})
	r := &Record{level: INFO, msg: "bid served ad_id=123 price=0.5", file: "bidder.go:42", unixNano: 1782563343536622000, seq: 42}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.buildPayload(r)
	}
}

func Benchmark_KafKaCodec_Proto(b *testing.B) {
	w := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "t", BufferSize: 16})
	w.SetKafkaCodec(KafkaCodecProto{})
	r := &Record{level: INFO, msg: "bid served ad_id=123 price=0.5", file: "bidder.go:42", unixNano: 1782563343536622000, seq: 42}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.buildPayload(r)
	}
}
