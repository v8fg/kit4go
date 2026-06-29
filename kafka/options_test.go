package kafka

import (
	"testing"
	"time"
)

func TestOptions_WithDefaults_Zero(t *testing.T) {
	o := Options{}.withDefaults()
	d := defaultOptions()
	if o.ProducerTimeout != d.ProducerTimeout {
		t.Errorf("ProducerTimeout=%v want %v", o.ProducerTimeout, d.ProducerTimeout)
	}
	if o.RetryMax != d.RetryMax {
		t.Errorf("RetryMax=%d want %d", o.RetryMax, d.RetryMax)
	}
	if o.ProducerLinger != DefaultProducerLinger {
		t.Errorf("ProducerLinger=%v want %v", o.ProducerLinger, DefaultProducerLinger)
	}
	if o.MaxBufferedRecords != DefaultMaxBufferedRecords {
		t.Errorf("MaxBufferedRecords=%d want %d", o.MaxBufferedRecords, DefaultMaxBufferedRecords)
	}
	// ChannelBufferSize is derived from MaxBufferedRecords (保持一致), not from
	// defaultOptions, so it tracks the in-flight cap.
	if o.ChannelBufferSize != o.MaxBufferedRecords {
		t.Errorf("ChannelBufferSize=%d want %d (tracks MaxBufferedRecords)", o.ChannelBufferSize, o.MaxBufferedRecords)
	}
	if !o.ReturnSuccesses {
		t.Error("ReturnSuccesses must default true for producers")
	}
	if !o.ReturnErrors {
		t.Error("ReturnErrors must default true")
	}
	if o.ConsumerOffsetInitial != OffsetNewest {
		t.Errorf("ConsumerOffsetInitial=%d want %d", o.ConsumerOffsetInitial, OffsetNewest)
	}
	if o.DeliveryMode != "callback" {
		t.Errorf("DeliveryMode=%q want callback", o.DeliveryMode)
	}
}

func TestOptions_WithDefaults_PreserveOverrides(t *testing.T) {
	o := Options{
		ProducerTimeout:       3 * time.Second,
		RetryMax:              9,
		ChannelBufferSize:     64,
		ConsumerOffsetInitial: OffsetOldest,
	}.withDefaults()
	if o.ProducerTimeout != 3*time.Second {
		t.Errorf("override ProducerTimeout not preserved: %v", o.ProducerTimeout)
	}
	if o.RetryMax != 9 {
		t.Errorf("override RetryMax not preserved: %d", o.RetryMax)
	}
	if o.ChannelBufferSize != 64 {
		t.Errorf("override ChannelBufferSize not preserved: %d", o.ChannelBufferSize)
	}
	if o.ConsumerOffsetInitial != OffsetOldest {
		t.Errorf("override ConsumerOffsetInitial not preserved: %d", o.ConsumerOffsetInitial)
	}
}

func TestOptions_FunctionalSetters(t *testing.T) {
	o := applyOptions([]Option{
		WithBrokers("a:9092", "b:9092"),
		WithVersion("3.5.0"),
		WithTopic("t"),
		WithGroupID("g"),
		WithPartition(3),
		WithOffset(OffsetOldest),
		WithReturnSuccesses(false),
		WithRetryMax(5),
		WithCodec(CodecRaw{}),
		WithDeliveryMode("channel"),
	})
	if len(o.Brokers) != 2 || o.Brokers[0] != "a:9092" {
		t.Errorf("Brokers=%v", o.Brokers)
	}
	if o.Version != "3.5.0" || o.Topic != "t" || o.GroupID != "g" {
		t.Errorf("basic setters: %+v", o)
	}
	if o.Partition != 3 || o.Offset != OffsetOldest {
		t.Errorf("partition/offset: %d %d", o.Partition, o.Offset)
	}
	if o.RetryMax != 5 || o.DeliveryMode != "channel" {
		t.Errorf("retry/delivery: %d %q", o.RetryMax, o.DeliveryMode)
	}
	if o.Codec == nil {
		t.Error("Codec not set")
	}
}

func TestOptions_Validate(t *testing.T) {
	if err := (Options{}).validate("producer"); err == nil {
		t.Error("no brokers should fail")
	}
	o := Options{Brokers: []string{"x"}}
	if err := o.validate("producer"); err != nil {
		t.Errorf("producer: %v", err)
	}
	if err := o.validate("consumer-group"); err == nil {
		t.Error("consumer-group needs group_id")
	}
	o.GroupID = "g"
	if err := o.validate("consumer-group"); err != nil {
		t.Errorf("consumer-group with group_id: %v", err)
	}
	if err := o.validate("partition-consumer"); err == nil {
		t.Error("partition-consumer needs topic")
	}
	o.Topic = "t"
	if err := o.validate("partition-consumer"); err != nil {
		t.Errorf("partition-consumer with topic: %v", err)
	}
}
