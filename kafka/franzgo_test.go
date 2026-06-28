//go:build franzgo

package kafka

import (
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
)

// These tests exercise the franz-go backend without a real broker (kgo.NewClient
// is lazy — it does not connect until first use, so constructors succeed with a
// dummy broker address, and Name/Backend/Metrics/SetOnEvent/Close + the
// Record/offset mappers are verifiable broker-free). The full produce→consume
// round-trip is the env-gated integration_test.go (run with KAFKA_BROKERS +
// -tags franzgo).

func TestFranzgo_RecordMappers(t *testing.T) {
	in := Message{
		Topic:   "adx",
		Key:     []byte("k1"),
		Value:   []byte("v1"),
		Headers: []Header{{Key: []byte("h1"), Value: []byte("hv")}},
	}
	r := toKgoRecord(in, "fallback")
	if r.Topic != "adx" || string(r.Key) != "k1" || string(r.Value) != "v1" {
		t.Errorf("toKgoRecord mismatch: %+v", r)
	}
	if len(r.Headers) != 1 || r.Headers[0].Key != "h1" || string(r.Headers[0].Value) != "hv" {
		t.Errorf("headers mismatch: %+v", r.Headers)
	}
	// topic fallback when empty
	r2 := toKgoRecord(Message{Value: []byte("x")}, "deft")
	if r2.Topic != "deft" {
		t.Errorf("topic fallback: got %q want deft", r2.Topic)
	}
	// reverse
	out := fromKgoRecord(r)
	if out.Topic != "adx" || string(out.Key) != "k1" || string(out.Value) != "v1" || len(out.Headers) != 1 || string(out.Headers[0].Key) != "h1" {
		t.Errorf("fromKgoRecord mismatch: %+v", out)
	}
}

func TestFranzgo_OffsetMapping(t *testing.T) {
	// just ensure the sentinels map without panic; kgo.Offset is opaque.
	_ = offsetToKgo(OffsetNewest)
	_ = offsetToKgo(OffsetOldest)
	_ = offsetToKgo(42)
}

func TestFranzgo_ProducerMetadata(t *testing.T) {
	p, err := NewProducer(WithBrokers("x"), WithTopic("t"), WithName("my-prod"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Backend() != "franz-go" {
		t.Errorf("Backend=%q want franz-go", p.Backend())
	}
	if p.Name() != "my-prod" {
		t.Errorf("Name=%q want my-prod", p.Name())
	}
	if got := p.Metrics(); got != (ProducerMetrics{}) {
		t.Errorf("zero Metrics=%+v", got)
	}
	var fired []string
	p.SetOnEvent(func(e ProducerEvent) { fired = append(fired, e.Name) })
	p.SetOnEvent(nil) // disable; must not panic
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := p.Close(); err != nil { // idempotent
		t.Errorf("second Close: %v", err)
	}
	_ = fired
}

func TestFranzgo_SyncProducerMetadata(t *testing.T) {
	p, err := NewSyncProducer(WithBrokers("x"), WithTopic("t"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Backend() != "franz-go" || p.Name() != "t" { // name defaults to topic
		t.Errorf("Backend=%q Name=%q", p.Backend(), p.Name())
	}
	_ = p.Close()
}

func TestFranzgo_ConsumerGroupMetadata(t *testing.T) {
	c, err := NewConsumerGroup(WithBrokers("x"), WithGroupID("g1"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Backend() != "franz-go" || c.Name() != "g1" { // name defaults to group-id
		t.Errorf("Backend=%q Name=%q", c.Backend(), c.Name())
	}
	_ = c.Close()
}

func TestFranzgo_PartitionConsumerMetadata(t *testing.T) {
	c, err := NewPartitionConsumer(WithBrokers("x"), WithTopic("t"), WithPartition(0), WithOffset(OffsetNewest))
	if err != nil {
		t.Fatal(err)
	}
	if c.Backend() != "franz-go" || c.Name() != "t" {
		t.Errorf("Backend=%q Name=%q", c.Backend(), c.Name())
	}
	if ch := c.Messages(); ch != nil {
		t.Errorf("Messages() in callback mode should be nil, got %v", ch)
	}
	_ = c.Close()
}

func TestFranzgo_Validate(t *testing.T) {
	if _, err := NewProducer(); err == nil {
		t.Error("no brokers should error")
	}
	if _, err := NewConsumerGroup(WithBrokers("x")); err == nil {
		t.Error("missing group_id should error")
	}
	if _, err := NewPartitionConsumer(WithBrokers("x")); err == nil {
		t.Error("missing topic should error")
	}
}

// ensure kgo is actually imported (sanity that the build tag pulls franz-go).
var _ = kgo.SeedBrokers
