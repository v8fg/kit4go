package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
)

// channel-mode forwarder: Messages() yields, then Close stops the pump.
func TestPartitionConsumer_ChannelMode_Forward(t *testing.T) {
	mc := mocks.NewConsumer(t, mockAsyncCfg())
	pc := mc.ExpectConsumePartition("t", 0, sarama.OffsetNewest)
	pc.YieldMessage(&sarama.ConsumerMessage{Topic: "t", Partition: 0, Offset: 5, Value: []byte("z")})
	pc.ExpectMessagesDrainedOnClose()

	c, _ := newSaramaPartitionConsumer(
		Options{Brokers: []string{"x"}, Topic: "t", Partition: 0, Offset: OffsetNewest, DeliveryMode: "channel"}.withDefaults(),
		mockConsumerFactory(mc))
	ch := c.Messages()
	if ch == nil {
		t.Fatal("Messages() in channel mode should be non-nil")
	}
	select {
	case m := <-ch:
		if string(m.Value) != "z" {
			t.Errorf("got value=%q want z", m.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("no message on channel")
	}
	waitUntil(t, func() bool { return c.Metrics().Received == 1 }, "channel received")
	_ = c.Close()
}

// partition error path: YieldError -> pump Errors() -> Metrics.Failed + Errors().
func TestPartitionConsumer_ErrorPath(t *testing.T) {
	mc := mocks.NewConsumer(t, mockAsyncCfg())
	pc := mc.ExpectConsumePartition("t", 0, sarama.OffsetNewest)
	pc.YieldError(&sarama.ConsumerError{Topic: "t", Partition: 0, Err: errBoom})
	pc.ExpectErrorsDrainedOnClose()

	c, _ := newSaramaPartitionConsumer(
		Options{Brokers: []string{"x"}, Topic: "t", Partition: 0, Offset: OffsetNewest}.withDefaults(),
		mockConsumerFactory(mc))
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = c.Consume(ctx, func(Message) error { return nil }) }()
	waitUntil(t, func() bool { return c.Metrics().Failed == 1 }, "error counted")

	select {
	case e := <-c.Errors():
		if !errorIs(e, errBoom) {
			t.Errorf("Errors() got %v want errBoom", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no error on Errors()")
	}
	cancel()
	_ = c.Close()
}

func TestPartitionConsumer_CloseIdempotent(t *testing.T) {
	mc := mocks.NewConsumer(t, mockAsyncCfg())
	pc := mc.ExpectConsumePartition("t", 0, sarama.OffsetNewest)
	pc.ExpectMessagesDrainedOnClose()
	c, _ := newSaramaPartitionConsumer(
		Options{Brokers: []string{"x"}, Topic: "t", Partition: 0, Offset: OffsetNewest}.withDefaults(),
		mockConsumerFactory(mc))
	_ = c.Close()
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// ConsumerGroup accounting + hooks without a real broker: exercise the
// non-broker methods (Metrics/SetOnEvent/Errors/pushErr/fire) on a bare struct.
func TestConsumerGroup_AccountingAndHooks(t *testing.T) {
	s := &saramaConsumerGroup{}

	// Metrics zero
	if got := s.Metrics(); got != (ConsumerMetrics{}) {
		t.Errorf("zero Metrics=%+v", got)
	}

	// SetOnEvent + fire
	var fired []string
	s.SetOnEvent(func(e ConsumerEvent) { fired = append(fired, e.Name) })
	s.fire(ConsumerEvent{Name: "ack"})
	s.fire(ConsumerEvent{Name: "rebalance"})
	if len(fired) != 2 || fired[0] != "ack" || fired[1] != "rebalance" {
		t.Errorf("fired=%v", fired)
	}
	s.SetOnEvent(nil)                        // disables
	s.fire(ConsumerEvent{Name: "after-nil"}) // should not append
	if len(fired) != 2 {
		t.Errorf("SetOnEvent(nil) should disable; fired=%v", fired)
	}

	// Errors() lazy + pushErr
	ec := s.Errors()
	s.pushErr(errBoom)
	select {
	case e := <-ec:
		if !errorIs(e, errBoom) {
			t.Errorf("Errors()=%v want errBoom", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no error pushed")
	}
}

func TestProducer_SetOnEventNil(t *testing.T) {
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	p, _ := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })
	p.SetOnEvent(nil) // must not panic; drain goroutines still fire (nil fn = no-op)
	p.SetOnEvent(func(ProducerEvent) {})
	_ = p.Close()
}
