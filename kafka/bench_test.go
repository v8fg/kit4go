package kafka

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// The codec Encode/Decode is shared by BOTH backends (sarama + franz-go), so
// these micro-benchmarks measure the per-message serialization cost that every
// backend pays. End-to-end produce→consume throughput depends on the broker
// (see BenchmarkThroughput_ProduceConsume, env-gated).

type benchEvent struct {
	ID    int      `json:"id"`
	AdID  string   `json:"ad_id"`
	Price float64  `json:"price"`
	Tags  []string `json:"tags"`
}

func BenchmarkKafkaCodec_JSON_Encode(b *testing.B) {
	c := CodecJSON{}
	in := benchEvent{ID: 7, AdID: "a1", Price: 0.5, Tags: []string{"rtb", "bid"}}
	b.ReportAllocs()

	for b.Loop() {
		_, _ = c.Encode(in)
	}
}

func BenchmarkKafkaCodec_JSON_Decode(b *testing.B) {
	c := CodecJSON{}
	payload, _ := c.Encode(benchEvent{ID: 7, AdID: "a1", Price: 0.5, Tags: []string{"rtb"}})
	b.ReportAllocs()

	for b.Loop() {
		var out benchEvent
		_ = c.Decode(payload, &out)
	}
}

// BenchmarkThroughput_ProduceConsume measures the end-to-end produce→consume
// round-trip throughput against a REAL broker. It is gated by KAFKA_BROKERS
// (skipped without a broker). Run both backends:
//
//	KAFKA_BROKERS=localhost:9092 go test -run='^$' -bench=Throughput -benchtime=2s ./...
//	KAFKA_BROKERS=localhost:9092 go test -run='^$' -bench=Throughput -benchtime=2s -tags franzgo ./...
func BenchmarkThroughput_ProduceConsume(b *testing.B) {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		b.Skip("KAFKA_BROKERS unset; throughput needs a real broker (run both with and without -tags franzgo)")
	}
	topic := "kit4go-kafka-bench"
	backend := "sarama"
	if val, _ := os.LookupEnv("GOFLAGS"); strings.Contains(val, "franzgo") {
		backend = "franz-go"
	}
	b.Logf("backend=%s brokers=%s", backend, brokers)

	prod, err := NewProducer(WithBrokers(brokers), WithTopic(topic))
	if err != nil {
		b.Fatal(err)
	}
	defer prod.Close()

	payload := []byte(`{"ad_id":"a1","price":0.5,"tags":["rtb"]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	b.ReportAllocs()

	for b.Loop() {
		if err := prod.Send(ctx, Message{Value: payload}); err != nil {
			b.Fatal(err)
		}
	}
	// Metrics assertion only (consume side is a separate concern; a full
	// round-trip benchmark belongs in the env-gated integration test).
	b.StopTimer()
	m := prod.Metrics()
	b.Logf("Enqueued=%d Success=%d Failed=%d Bytes=%d", m.Enqueued, m.Success, m.Failed, m.Bytes)
}
