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

// kgoProducerOpts builds kgo client options for an ASYNC producer.
//
// RequiredAcks, ProducerLinger, and MaxBufferedRecords are ALWAYS set explicitly
// (via kgoAcks / effectiveLinger / the resolved value) so franz-go's native
// defaults (acks=all-ISR, linger=10ms, maxBufferedRecords=10000) never leak in —
// behavior is pinned to the kit4go resolved Options (acks default = leader, for
// parity with sarama) and matches what Snapshot() reports.
func kgoProducerOpts(o Options) []kgo.Opt {
	opts := []kgo.Opt{
		kgo.SeedBrokers(o.Brokers...),
		kgo.AllowAutoTopicCreation(),
		kgo.RecordRetries(5), // retry on UNKNOWN_TOPIC_OR_PART (auto-create race)
		kgo.RequiredAcks(kgoAcks(o.Acks)),
		kgo.ProducerLinger(effectiveLinger(o.ProducerLinger)),
		kgo.MaxBufferedRecords(o.MaxBufferedRecords),
	}
	// BatchMaxBytes caps a single batch's byte size (0 = kgo default ~1MiB).
	if o.BatchMaxBytes > 0 {
		opts = append(opts, kgo.ProducerBatchMaxBytes(int32(o.BatchMaxBytes)))
	}
	// acks=leader/none is incompatible with franz-go's idempotent producer
	// (which requires acks=all) — disable it explicitly in those modes.
	if kgoNeedsIdempotencyDisabled(o.Acks) {
		opts = append(opts, kgo.DisableIdempotentWrite())
	}
	return opts
}

// kgoAcks maps Options.Acks to kgo's RequiredAcks. Empty/unknown → LeaderAcks
// (acks=1, the package default — throughput-first; unifies both backends on
// leader unless AcksAll/AcksNone is set. NOTE: this changes franz-go's behavior
// from its native all-ISR default to leader — set AcksAll for durability).
// kgoAcks maps Options.Acks to kgo's RequiredAcks. Default (empty or AcksLeader)
// → LeaderAck — UNIFIED with sarama (both backends default to leader).
// Only explicit AcksAll → AllISRAcks (keeps the idempotent producer).
func kgoAcks(a string) kgo.Acks {
	switch a {
	case AcksAll:
		return kgo.AllISRAcks()
	case AcksNone:
		return kgo.NoAck()
	default: // "" or AcksLeader → leader (the unified default)
		return kgo.LeaderAck()
	}
}

// kgoNeedsIdempotencyDisabled reports whether the chosen acks is incompatible
// with franz-go's idempotent producer (which requires acks=all). Only AcksAll
// keeps it; everything else (including the default "") disables it.
func kgoNeedsIdempotencyDisabled(a string) bool {
	return a != AcksAll
}

// kgoSyncProducerOpts builds kgo client options for a SYNC producer. ProduceSync
// is synchronous (blocks per send, no async batching), so ProducerLinger /
// MaxBufferedRecords are NOT applied — they're inert for sync. Omitting them
// keeps franz-go sync behavior identical to sarama's Flush-off sync path
// (buildSaramaConfig sync=true), so the two backends stay consistent.
func kgoSyncProducerOpts(o Options) []kgo.Opt {
	opts := []kgo.Opt{
		kgo.SeedBrokers(o.Brokers...),
		kgo.AllowAutoTopicCreation(),
		kgo.RecordRetries(5),
		kgo.RequiredAcks(kgoAcks(o.Acks)),
	}
	if kgoNeedsIdempotencyDisabled(o.Acks) {
		opts = append(opts, kgo.DisableIdempotentWrite())
	}
	return opts
}

// kgoConsumerGroupOpts builds kgo client options for a consumer group.
// AutoCommitMarks + MarkCommitRecords on ACK gives at-least-once (NACK = not
// marked = re-delivered next session), matching the sarama backend's semantics.
func kgoConsumerGroupOpts(o Options) []kgo.Opt {
	// Map the initial offset (OffsetNewest/Oldest) to the kgo reset offset.
	var reset kgo.Offset
	switch o.ConsumerOffsetInitial {
	case OffsetOldest:
		reset = kgo.NewOffset().AtStart()
	default:
		reset = kgo.NewOffset().AtEnd()
	}
	return []kgo.Opt{
		kgo.SeedBrokers(o.Brokers...),
		kgo.ConsumerGroup(o.GroupID),
		kgo.AutoCommitMarks(),
		kgo.ConsumeResetOffset(reset),
		kgo.AllowAutoTopicCreation(),
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
	logConfig("async-producer", o)
	cl, err := kgo.NewClient(kgoProducerOpts(o)...)
	if err != nil {
		return nil, err
	}
	return &franzProducer{opts: o, cl: cl, history: newSnapshotHistory(o.SnapshotHistory)}, nil
}

// NewSyncProducer builds a sync Producer (Send blocks until broker ack).
func NewSyncProducer(opts ...Option) (SyncProducer, error) {
	o := applyOptions(opts)
	o = o.withDefaults()
	if err := o.validate("producer"); err != nil {
		return nil, err
	}
	logConfig("sync-producer", o)
	cl, err := kgo.NewClient(kgoSyncProducerOpts(o)...)
	if err != nil {
		return nil, err
	}
	return &franzSyncProducer{opts: o, cl: cl}, nil
}

// NewConsumerGroup builds a rebalance-aware ConsumerGroup. WithBrokers and
// WithGroupID are required. The kgo client is created lazily in Consume() so
// the topics can be wired to the client at creation time (franz-go requires
// ConsumeTopics at client creation for group consuming).
func NewConsumerGroup(opts ...Option) (ConsumerGroup, error) {
	o := applyOptions(opts)
	o = o.withDefaults()
	if err := o.validate("consumer-group"); err != nil {
		return nil, err
	}
	return &franzConsumerGroup{opts: o}, nil
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
