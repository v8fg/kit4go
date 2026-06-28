package kafka

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
)

func mockConsumerFactory(mc *mocks.Consumer) consumerFactory {
	return func([]string, *sarama.Config) (sarama.Consumer, error) { return mc, nil }
}

func TestPartitionConsumer_CallbackMode(t *testing.T) {
	mc := mocks.NewConsumer(t, mockAsyncCfg())
	pc := mc.ExpectConsumePartition("t", 0, sarama.OffsetNewest)
	pc.YieldMessage(&sarama.ConsumerMessage{Topic: "t", Partition: 0, Offset: 0, Value: []byte("a")})
	pc.YieldMessage(&sarama.ConsumerMessage{Topic: "t", Partition: 0, Offset: 1, Value: []byte("b")})
	pc.ExpectMessagesDrainedOnClose()

	c, err := newSaramaPartitionConsumer(
		Options{Brokers: []string{"x"}, Topic: "t", Partition: 0, Offset: OffsetNewest}.withDefaults(),
		mockConsumerFactory(mc))
	if err != nil {
		t.Fatal(err)
	}

	var got atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = c.Consume(ctx, func(m Message) error { got.Add(1); return nil }) }()

	waitUntil(t, func() bool { return c.Metrics().Received == 2 }, "receive 2")
	waitUntil(t, func() bool { return c.Metrics().Acked == 2 }, "ack 2")
	cancel()
	_ = c.Close()
	if got.Load() != 2 {
		t.Errorf("handler invoked %d times want 2", got.Load())
	}
}

func TestPartitionConsumer_NACK(t *testing.T) {
	mc := mocks.NewConsumer(t, mockAsyncCfg())
	pc := mc.ExpectConsumePartition("t", 0, sarama.OffsetOldest)
	pc.YieldMessage(&sarama.ConsumerMessage{Topic: "t", Value: []byte("x")})
	pc.ExpectMessagesDrainedOnClose()

	c, _ := newSaramaPartitionConsumer(
		Options{Brokers: []string{"x"}, Topic: "t", Partition: 0, Offset: OffsetOldest}.withDefaults(),
		mockConsumerFactory(mc))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = c.Consume(ctx, func(m Message) error { return errBoom }) // NACK every message
	}()
	waitUntil(t, func() bool { return c.Metrics().Failed == 1 }, "failed 1")
	if got := c.Metrics().Acked; got != 0 {
		t.Errorf("Acked=%d want 0 on NACK", got)
	}
	cancel()
	_ = c.Close()
}

func TestPartitionConsumer_Validate(t *testing.T) {
	if _, err := NewPartitionConsumer(WithBrokers("x")); err == nil {
		t.Error("missing topic should error")
	}
}

func TestPartitionConsumer_ChannelMode_NilInCallback(t *testing.T) {
	mc := mocks.NewConsumer(t, mockAsyncCfg())
	pc := mc.ExpectConsumePartition("t", 0, sarama.OffsetNewest)
	pc.ExpectMessagesDrainedOnClose()
	c, _ := newSaramaPartitionConsumer(
		Options{Brokers: []string{"x"}, Topic: "t", Partition: 0, Offset: OffsetNewest, DeliveryMode: "callback"}.withDefaults(),
		mockConsumerFactory(mc))
	if ch := c.Messages(); ch != nil {
		t.Errorf("Messages() in callback mode should be nil, got %v", ch)
	}
	_ = c.Close()
}

// guard against the channel-mode forwarder leaking forever in tests is handled
// by cancelling the context or closing the consumer; the mock's message channel
// is bounded by YieldMessage/Close.
