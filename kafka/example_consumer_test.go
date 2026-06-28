package kafka_test

// Compile-checked examples (no // Output:, so not executed — they need a live
// broker). Run for real with: KAFKA_BROKERS=localhost:9092 go test -run Example

import (
	"context"
	"fmt"
	"os"

	"github.com/v8fg/kit4go/kafka"
)

// ExampleNewConsumerGroup shows the rebalance-aware group consumer. Consume
// runs until ctx is cancelled, invoking the handler per message; nil = ACK
// (commit offset), non-nil = NACK (re-delivered next session).
func ExampleNewConsumerGroup() {
	grp, err := kafka.NewConsumerGroup(
		kafka.WithBrokers("localhost:9092"),
		kafka.WithGroupID("indexer"),
		kafka.WithConsumerOffsetInitial(kafka.OffsetOldest),
	)
	if err != nil {
		panic(err)
	}
	defer grp.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for err := range grp.Errors() {
			fmt.Fprintln(os.Stderr, "consume error:", err)
		}
	}()

	_ = grp.Consume(ctx, []string{"adx-logs"}, func(m kafka.Message) error {
		// index m.Value ...; return nil to commit, non-nil to NACK.
		return nil
	})
}

// ExampleNewPartitionConsumer shows single-partition consumption from a fixed
// offset (the inverted_file_listener pattern). Callback delivery mode.
func ExampleNewPartitionConsumer() {
	c, err := kafka.NewPartitionConsumer(
		kafka.WithBrokers("localhost:9092"),
		kafka.WithTopic("adx-logs"),
		kafka.WithPartition(0),
		kafka.WithOffset(kafka.OffsetNewest),
	)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = c.Consume(ctx, func(m kafka.Message) error {
		// handle m.Value from partition 0, from newest onward
		return nil
	})
}

// ExamplePartitionConsumer_channel shows channel-delivery mode: Messages()
// yields messages on a channel instead of a callback.
func ExamplePartitionConsumer_channel() {
	c, _ := kafka.NewPartitionConsumer(
		kafka.WithBrokers("localhost:9092"),
		kafka.WithTopic("adx-logs"),
		kafka.WithPartition(0),
		kafka.WithOffset(kafka.OffsetOldest),
		kafka.WithDeliveryMode("channel"),
	)
	defer c.Close()

	for m := range c.Messages() {
		_ = m // handle m.Value
	}
}
