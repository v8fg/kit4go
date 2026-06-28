// Package kafka is a library-agnostic Kafka producer + consumer wrapper.
//
// Callers depend only on the interfaces declared here (Producer, SyncProducer,
// ConsumerGroup, PartitionConsumer) and the value types (Message, Codec,
// Options). The default implementation wraps github.com/IBM/sarama (see the
// sarama_*.go files); alternate backends (segmentio/kafka-go,
// confluent-kafka-go) can be added later behind the same interfaces with zero
// change to consuming code — the "无感切换" (seamless swap) goal.
//
// Quick start — produce (async) and consume (group):
//
//	prod, _ := kafka.NewProducer(kafka.WithBrokers("kafka:9092"), kafka.WithTopic("adx-logs"))
//	_ = prod.Send(ctx, kafka.Message{Value: []byte("hello")})
//	defer prod.Close()
//
//	grp, _ := kafka.NewConsumerGroup(kafka.WithBrokers("kafka:9092"), kafka.WithGroupID("idx"))
//	_ = grp.Consume(ctx, []string{"adx-logs"}, func(m kafka.Message) error {
//	    // handle m.Value ... return nil to ACK (commit offset), non-nil to NACK.
//	    return nil
//	})
//	defer grp.Close()
//
// See example_producer_test.go and example_consumer_test.go for runnable demos.
package kafka

import (
	"context"
	"time"
)

// Offset sentinels for "consume from". They intentionally mirror sarama's
// sentinel values but are owned here so callers never import sarama. A concrete
// int64 >= 0 means "consume from this absolute offset".
const (
	OffsetNewest int64 = -1
	OffsetOldest int64 = -2
)

// Header is a single Kafka record header (key/value pairs on a message).
type Header struct {
	Key, Value []byte
}

// Message is the library-agnostic envelope used for both produce and consume.
// On produce, Topic/Key/Value/Headers are sent; Partition/Offset/Timestamp are
// assigned by the broker and surfaced on the success path (sync producer return
// / async OnSuccess event). On consume, all fields are populated from the
// broker's ConsumerMessage.
type Message struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Value     []byte
	Headers   []Header
	Timestamp time.Time
}

// MessageHandler is the consume callback. Return nil to ACK the message (the
// wrapper marks/commits the offset); return a non-nil error to NACK (offset is
// NOT committed, the failure is surfaced via OnError / the consumer's Errors
// channel, and the loop continues — at-least-once semantics are the caller's
// responsibility, e.g. via retry/idempotency).
type MessageHandler func(Message) error

// Codec (de)serializes Message.Value so callers can hand structured values to
// Send and receive typed values from a handler. It is OPTIONAL: a nil codec
// means raw byte pass-through (the common case for pre-encoded payloads, e.g.
// log4go's KafkaWriter). Built-ins: CodecJSON, CodecProto, CodecRaw.
type Codec interface {
	// Encode marshals v to bytes for Message.Value.
	Encode(v any) ([]byte, error)
	// Decode unmarshals b (a Message.Value) into out (a pointer).
	Decode(b []byte, out any) error
	// ContentType is informational (e.g. "application/json"); used for headers
	// and diagnostics.
	ContentType() string
}

// Producer is the async producer. Send enqueues a message and returns immediately
// (it does NOT wait for the broker ack); the wrapper drains Successes/Errors
// internally, updates Metrics, and fires the optional OnSuccess/OnError hooks.
// Use Close to block until all in-flight messages are acked.
type Producer interface {
	// Send enqueues msg. It returns an error only for client-side failures
	// (closed producer, a full input buffer when configured to block-fail, a
	// codec encode error). Broker-level delivery is reported asynchronously
	// via Metrics.Failed / the OnError hook.
	Send(ctx context.Context, msg Message) error
	// Close drains in-flight messages and releases resources. Idempotent.
	Close() error
	// Metrics returns a consistent snapshot of the producer counters.
	Metrics() ProducerMetrics
	// SetOnEvent installs a per-event hook (send/success/error/close). Pass nil
	// to disable (zero overhead). Not safe to call concurrently with Send.
	SetOnEvent(fn func(ProducerEvent))
}

// SyncProducer blocks each Send until the broker acks. Use it when you need the
// assigned partition/offset synchronously (lower throughput, simpler reasoning).
type SyncProducer interface {
	// Send sends msg and blocks until the broker acks, returning the assigned
	// partition and offset.
	Send(ctx context.Context, msg Message) (partition int32, offset int64, err error)
	Close() error
	Metrics() ProducerMetrics
	SetOnEvent(fn func(ProducerEvent))
}

// ConsumerGroup is the rebalance-aware group consumer (the engine-master
// consumerGroupProxy pattern). Consume runs the infinite Consume-loop —
// recreating the session after a server-side rebalance — and returns only when
// ctx is cancelled or a fatal error occurs.
type ConsumerGroup interface {
	// Consume subscribes to topics and invokes handler for each message, in a
	// loop that survives rebalances. Returns ctx.Err() when ctx is cancelled,
	// or a non-nil error on a fatal consume failure.
	Consume(ctx context.Context, topics []string, handler MessageHandler) error
	// Errors returns a channel of background errors (rebalance, broker, etc.).
	Errors() <-chan error
	Close() error
	Metrics() ConsumerMetrics
	SetOnEvent(fn func(ConsumerEvent))
}

// PartitionConsumer consumes ONE specified partition from a specified offset
// (the engine-master inverted_file_listener pattern). Use it when you want
// fixed-partition / fixed-offset consumption outside a consumer group.
type PartitionConsumer interface {
	// Consume invokes handler for each message on the configured partition. In
	// callback delivery mode it blocks until ctx is cancelled; Messages() is
	// nil in this mode.
	Consume(ctx context.Context, handler MessageHandler) error
	// Messages returns the message channel in channel delivery mode, or nil in
	// callback mode.
	Messages() <-chan Message
	Errors() <-chan error
	Close() error
}

// ProducerMetrics is a snapshot of async/sync producer counters.
type ProducerMetrics struct {
	Enqueued uint64 // messages handed to the underlying client (Input/SendMessage)
	Success  uint64 // broker-acked
	Failed   uint64 // errors drained from the underlying client
}

// ConsumerMetrics is a snapshot of consumer counters.
type ConsumerMetrics struct {
	Received  uint64 // messages handed to a handler / Messages() channel
	Acked     uint64 // handler returned nil (offset committed)
	Failed    uint64 // handler returned non-nil, or decode error
	Rebalance uint64 // consumer-group sessions recreated after a rebalance
}

// ProducerEvent feeds Producer.SetOnEvent. Name is one of "send","success",
// "error","close".
type ProducerEvent struct {
	Name  string
	Topic string
	Bytes int
	Err   error
}

// ConsumerEvent feeds ConsumerGroup.SetOnEvent. Name is one of "message",
// "ack","nack","error","rebalance","close".
type ConsumerEvent struct {
	Name string
	Msg  Message
	Err  error
}
