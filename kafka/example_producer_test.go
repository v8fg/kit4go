package kafka_test

// These examples are compile-checked by `go test` but NOT executed (they have
// no // Output: line), because they require a live Kafka broker to dial. Run
// them for real against a broker via:
//
//	KAFKA_BROKERS=localhost:9092 go run ./path/to/your/program

import (
	"context"

	"github.com/v8fg/kit4go/kafka"
)

// ExampleNewProducer shows async produce with the success/error accounting.
// Send enqueues and returns immediately; the wrapper drains Successes/Errors
// internally and updates Metrics. Close blocks until in-flight messages are
// acked.
func ExampleNewProducer() {
	prod, err := kafka.NewProducer(
		kafka.WithBrokers("localhost:9092"),
		kafka.WithTopic("adx-logs"),
	)
	if err != nil {
		panic(err)
	}
	prod.SetOnEvent(func(e kafka.ProducerEvent) {
		if e.Name == "error" {
			// handle e.Err (e.g. log + retry)
		}
	})

	_ = prod.Send(context.Background(), kafka.Message{
		Key:   []byte("req-123"), // same key -> same partition -> ordered consume
		Value: []byte(`{"ad_id":"a1","price":0.5}`),
	})

	_ = prod.Close() // drain in-flight, then release
}

// ExampleNewSyncProducer shows the synchronous produce path: Send blocks until
// the broker acks and returns the assigned partition + offset.
func ExampleNewSyncProducer() {
	prod, err := kafka.NewSyncProducer(kafka.WithBrokers("localhost:9092"))
	if err != nil {
		panic(err)
	}
	defer prod.Close()

	partition, offset, err := prod.Send(context.Background(), kafka.Message{
		Topic: "billing",
		Value: []byte("charge 0.5"),
	})
	if err != nil {
		panic(err)
	}
	_, _ = partition, offset // broker-assigned location of the message
}
