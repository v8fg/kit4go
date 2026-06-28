package kafka

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestIntegration_ProduceConsumeRoundTrip is the end-to-end proof: produce a
// message, then consume it back via a consumer group. It is gated by the
// KAFKA_BROKERS env var (comma-separated host:port list), so it is SKIPPED in
// CI and only runs against a real broker, e.g.:
//
//	docker run -d -p 9092:9092 ... (a Kafka broker with auto-topic-create)
//	KAFKA_BROKERS=localhost:9092 go test -run Integration -v ./...
//
// The broker must allow auto-topic creation (the test uses a fresh topic).
func TestIntegration_ProduceConsumeRoundTrip(t *testing.T) {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("KAFKA_BROKERS unset; skipping integration test")
	}

	topic := fmt.Sprintf("kit4go-kafka-it-%d", time.Now().UnixNano())
	groupID := "kit4go-kafka-it-group"
	payload := []byte("hello-integration")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Produce (sync: blocks until broker acks; franz-go's client is lazy and
	// needs a moment to connect on first use, so a short sleep avoids the
	// initial metadata/connection race).
	time.Sleep(2 * time.Second)
	prod, err := NewSyncProducer(WithBrokers(brokers), WithTopic(topic))
	if err != nil {
		t.Fatalf("NewSyncProducer: %v", err)
	}
	if _, _, err := prod.Send(ctx, Message{Key: []byte("k1"), Value: payload}); err != nil {
		t.Fatalf("SyncProducer.Send: %v", err)
	}
	prod.Close()

	// Consume (group) until the payload is seen.
	grp, err := NewConsumerGroup(
		WithBrokers(brokers),
		WithGroupID(groupID),
		WithConsumerOffsetInitial(OffsetOldest), // read from start to catch the just-produced msg
	)
	if err != nil {
		t.Fatalf("NewConsumerGroup: %v", err)
	}
	defer grp.Close()

	got := make(chan Message, 1)
	consumeErr := make(chan error, 1)
	go func() {
		consumeErr <- grp.Consume(ctx, []string{topic}, func(m Message) error {
			if bytes.Equal(m.Value, payload) {
				select {
				case got <- m:
					cancel() // stop the consume loop once observed
				default:
				}
			}
			return nil
		})
	}()

	select {
	case m := <-got:
		if !bytes.Equal(m.Value, payload) {
			t.Errorf("got %q want %q", m.Value, payload)
		}
	case err := <-consumeErr:
		t.Fatalf("Consume returned before payload: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for the produced message")
	}
}
