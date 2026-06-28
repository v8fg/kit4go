//go:build franzgo

package kafka

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
)

// This is the franz-go (kgo) backend, selected at build time with
// `-tags franzgo`. It implements the SAME constructors and interfaces as the
// default sarama backend (sarama_*.go, //go:build !franzgo), so callers switch
// backends with zero source change — only the build tag differs (mirrors the
// kit4go json/ package's build-tag-selected codec pattern).

// backendName identifies the underlying client library for Backend()/monitoring.
const backendName = "franz-go"

// offsetToKgo maps the package offset sentinels to kgo.Offset.
func offsetToKgo(o int64) kgo.Offset {
	switch o {
	case OffsetNewest:
		return kgo.NewOffset().AtEnd()
	case OffsetOldest:
		return kgo.NewOffset().AtStart()
	default:
		return kgo.NewOffset().At(o)
	}
}

// kgoProducerOpts builds kgo client options for a producer. franz-go's default
// acks is all-ISR; no explicit RequiredAcks needed.
func kgoProducerOpts(o Options) []kgo.Opt {
	return []kgo.Opt{
		kgo.SeedBrokers(o.Brokers...),
	}
}

// kgoConsumerGroupOpts builds kgo client options for a consumer group.
// AutoCommitMarks + MarkCommitRecords on ACK gives at-least-once (NACK = not
// marked = re-delivered next session), matching the sarama backend's semantics.
func kgoConsumerGroupOpts(o Options) []kgo.Opt {
	return []kgo.Opt{
		kgo.SeedBrokers(o.Brokers...),
		kgo.ConsumerGroup(o.GroupID),
		kgo.AutoCommitMarks(), // commit the records we MarkCommitRecords on ACK
	}
}

// kgoPartitionConsumerOpts builds kgo client options for a single-partition
// consumer (ConsumePartitions takes a topic→partition→offset map).
func kgoPartitionConsumerOpts(o Options) []kgo.Opt {
	partitions := map[string]map[int32]kgo.Offset{
		o.Topic: {o.Partition: offsetToKgo(o.Offset)},
	}
	return []kgo.Opt{
		kgo.SeedBrokers(o.Brokers...),
		kgo.ConsumePartitions(partitions),
	}
}

// toKgoRecord maps a library Message to a kgo.Record. Topic falls back to
// defTopic when empty (matches the sarama backend's per-message topic override).
func toKgoRecord(msg Message, defTopic string) *kgo.Record {
	topic := msg.Topic
	if topic == "" {
		topic = defTopic
	}
	r := &kgo.Record{Topic: topic, Key: msg.Key, Value: msg.Value}
	if n := len(msg.Headers); n > 0 {
		hdrs := make([]kgo.RecordHeader, n)
		for i, h := range msg.Headers {
			hdrs[i] = kgo.RecordHeader{Key: string(h.Key), Value: h.Value}
		}
		r.Headers = hdrs
	}
	return r
}

// fromKgoRecord maps a kgo.Record (consumed) to a library Message.
func fromKgoRecord(r *kgo.Record) Message {
	m := Message{
		Topic:     r.Topic,
		Partition: r.Partition,
		Offset:    r.Offset,
		Key:       r.Key,
		Value:     r.Value,
		Timestamp: r.Timestamp,
	}
	if n := len(r.Headers); n > 0 {
		m.Headers = make([]Header, n)
		for i, h := range r.Headers {
			m.Headers[i] = Header{Key: []byte(h.Key), Value: h.Value}
		}
	}
	return m
}

// --- constructors (same signatures as the sarama backend) ---

// NewProducer builds an async Producer backed by franz-go. opts are applied with
// defaults; only WithBrokers is required.
func NewProducer(opts ...Option) (Producer, error) {
	o := applyOptions(opts)
	o = o.withDefaults()
	if err := o.validate("producer"); err != nil {
		return nil, err
	}
	cl, err := kgo.NewClient(kgoProducerOpts(o)...)
	if err != nil {
		return nil, err
	}
	return &franzProducer{opts: o, cl: cl}, nil
}

// NewSyncProducer builds a sync Producer (Send blocks until broker ack).
func NewSyncProducer(opts ...Option) (SyncProducer, error) {
	o := applyOptions(opts)
	o = o.withDefaults()
	if err := o.validate("producer"); err != nil {
		return nil, err
	}
	cl, err := kgo.NewClient(kgoProducerOpts(o)...)
	if err != nil {
		return nil, err
	}
	return &franzSyncProducer{opts: o, cl: cl}, nil
}

// NewConsumerGroup builds a rebalance-aware ConsumerGroup. WithBrokers and
// WithGroupID are required.
func NewConsumerGroup(opts ...Option) (ConsumerGroup, error) {
	o := applyOptions(opts)
	o = o.withDefaults()
	if err := o.validate("consumer-group"); err != nil {
		return nil, err
	}
	cl, err := kgo.NewClient(kgoConsumerGroupOpts(o)...)
	if err != nil {
		return nil, err
	}
	return &franzConsumerGroup{opts: o, cl: cl}, nil
}

// NewPartitionConsumer builds a single-partition consumer. WithBrokers,
// WithTopic, WithPartition and WithOffset are required.
func NewPartitionConsumer(opts ...Option) (PartitionConsumer, error) {
	o := applyOptions(opts)
	o = o.withDefaults()
	if err := o.validate("partition-consumer"); err != nil {
		return nil, err
	}
	cl, err := kgo.NewClient(kgoPartitionConsumerOpts(o)...)
	if err != nil {
		return nil, err
	}
	return &franzPartitionConsumer{opts: o, cl: cl}, nil
}

// ctxDone is a tiny helper to keep the consume loops readable.
func ctxDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
