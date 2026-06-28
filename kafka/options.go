package kafka

import (
	"errors"
	"time"
)

// Options configures every Producer/Consumer constructor. Zero values are
// replaced with sensible defaults by withDefaults at construction time, so the
// zero Options is usable for a default client (only Brokers is required).
//
// Field tags carry both json and mapstructure names so the struct can be loaded
// from a JSON config file or a Viper-style mapstructure source. Codec is tagged
// "-" because a live codec object cannot be (de)serialised.
type Options struct {
	// Brokers is the bootstrap broker address list (host:port each). Required —
	// a constructor with no brokers returns an error.
	Brokers []string `json:"brokers" mapstructure:"brokers"`

	// Name is the instance name for per-instance monitoring (exposed via
	// Producer/Consumer Name()). Default: the Topic (producer/partition) or
	// GroupID (consumer group) — so each instance is identifiable in metrics
	// like log4go's per-writer monitoring.
	Name string `json:"name" mapstructure:"name"`

	// Version is the Kafka cluster version, e.g. "3.5.0". Default "" selects
	// the wrapper's default (sarama V2_5_0). An invalid string is a
	// construction error.
	Version string `json:"version" mapstructure:"version"`

	// Topic is the default topic for produce, and for partition-consumer
	// subscription. Consumer groups subscribe via Consume(topics, ...) so the
	// group does not require this field.
	Topic string `json:"topic" mapstructure:"topic"`

	// GroupID is the consumer-group name. Required for NewConsumerGroup;
	// ignored by producers.
	GroupID string `json:"group_id" mapstructure:"group_id"`

	// Partition selects a single partition for NewPartitionConsumer. Required
	// there; ignored otherwise.
	Partition int32 `json:"partition" mapstructure:"partition"`

	// Offset is the starting offset for NewPartitionConsumer, and the fallback
	// "initial offset" for a consumer group when the group has no committed
	// offset. Use OffsetNewest / OffsetOldest, or a concrete int64 >= 0.
	Offset int64 `json:"offset" mapstructure:"offset"`

	// --- producer tuning ---

	// ReturnSuccesses enables Successes() delivery (required for the success
	// accounting and OnSuccess hook). Default true. Disabling it makes the
	// async producer pure fire-and-forget (no success path).
	ReturnSuccesses bool `json:"return_successes" mapstructure:"return_successes"`

	// ReturnErrors enables Errors() delivery. Default true (almost always
	// wanted; disabling drops broker errors silently).
	ReturnErrors bool `json:"return_errors" mapstructure:"return_errors"`

	// ProducerTimeout is sarama's Producer.Timeout (the flush deadline on
	// Close). Default 10s.
	ProducerTimeout time.Duration `json:"producer_timeout" mapstructure:"producer_timeout"`

	// RetryMax is the producer's retry count for transient errors. Default 3.
	RetryMax int `json:"retry_max" mapstructure:"retry_max"`

	// ChannelBufferSize is the async producer Input() channel size. Default
	// 256. When full, Send blocks (backpressure); configure up for bursty load.
	ChannelBufferSize int `json:"channel_buffer_size" mapstructure:"channel_buffer_size"`

	// ProducerLinger is how long the backend waits before flushing a partial
	// batch. 0 (default) = flush immediately (current behavior, lowest latency).
	// >0 (e.g. 1ms) = accumulate records for up to this duration before flushing
	// → larger batches → fewer RPCs → higher throughput, at the cost of added
	// latency per record. Typical: 1-10ms for high-throughput pipelines.
	// Pairs with SendBatch for maximum effect. Memory impact: see Metrics.InFlight.
	//
	// DATA-LOSS RISK: records buffered during the linger window are lost if the
	// process crashes before Close() flushes them. Keep linger small (≤10ms) and
	// always defer Close() on shutdown. Monitor Metrics.InFlight for buffer depth.
	ProducerLinger time.Duration `json:"producer_linger" mapstructure:"producer_linger"`

	// MaxBufferedRecords caps the number of records buffered in the backend's
	// internal accumulator before backpressure kicks in (Send/SendBatch blocks).
	// 0 = use the backend's default (sarama: unlimited via channel buffer;
	// franz-go: 10,000). Lower this to bound memory; raise it for bursty loads.
	// Memory bound: MaxBufferedRecords × avg_msg_size.
	MaxBufferedRecords int `json:"max_buffered_records" mapstructure:"max_buffered_records"`

	// BatchMaxBytes caps a single batch's byte size. 0 = backend default
	// (sarama: 1MB via MaxMessageBytes; franz-go: 1MB via ProducerBatchMaxBytes).
	// Larger batches amortize RPC overhead but increase memory per batch.
	BatchMaxBytes int `json:"batch_max_bytes" mapstructure:"batch_max_bytes"`

	// --- consumer tuning ---

	// ConsumerOffsetInitial is the group's Offsets.Initial (OffsetNewest /
	// OffsetOldest) when the group has no committed offset. Default OffsetNewest.
	ConsumerOffsetInitial int64 `json:"consumer_offset_initial" mapstructure:"consumer_offset_initial"`

	// FetchMin is sarama's Consumer.Fetch.Min bytes. Default 1 (match sarama).
	FetchMin int32 `json:"fetch_min" mapstructure:"fetch_min"`

	// --- behaviour ---

	// Codec (de)serialises Message.Value. nil (default) = raw byte pass-through.
	Codec Codec `json:"-"`

	// DeliveryMode selects how a PartitionConsumer yields messages:
	// "callback" (default) — invoke the handler in Consume; or "channel" —
	// expose Messages().
	DeliveryMode string `json:"delivery_mode" mapstructure:"delivery_mode"`
}

// applyOptions folds a chain of functional options into an Options value. It is
// the common entry point for every constructor.
func applyOptions(opts []Option) Options {
	o := Options{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// nameOr returns name if non-empty, else fallback. Used by constructors to
// default the instance Name to the Topic (producer/partition) or GroupID
// (consumer group) when WithName is not set.
func nameOr(name, fallback string) string {
	if name != "" {
		return name
	}
	return fallback
}

// Option is a functional option applied to Options at construction.
type Option func(*Options)

// WithBrokers sets the bootstrap broker list.
func WithBrokers(brokers ...string) Option { return func(o *Options) { o.Brokers = brokers } }

// WithName sets the instance name for per-instance monitoring.
func WithName(name string) Option { return func(o *Options) { o.Name = name } }

// WithVersion sets the Kafka cluster version string (e.g. "3.5.0").
func WithVersion(v string) Option { return func(o *Options) { o.Version = v } }

// WithTopic sets the default topic.
func WithTopic(t string) Option { return func(o *Options) { o.Topic = t } }

// WithGroupID sets the consumer-group id.
func WithGroupID(g string) Option { return func(o *Options) { o.GroupID = g } }

// WithPartition selects a partition for a PartitionConsumer.
func WithPartition(p int32) Option { return func(o *Options) { o.Partition = p } }

// WithOffset sets the starting/initial offset.
func WithOffset(o int64) Option { return func(opts *Options) { opts.Offset = o } }

// WithReturnSuccesses toggles async-producer success accounting.
func WithReturnSuccesses(b bool) Option { return func(o *Options) { o.ReturnSuccesses = b } }

// WithProducerTimeout sets the flush deadline on Close.
func WithProducerTimeout(d time.Duration) Option { return func(o *Options) { o.ProducerTimeout = d } }

// WithRetryMax sets the producer retry count.
func WithRetryMax(n int) Option { return func(o *Options) { o.RetryMax = n } }

// WithChannelBufferSize sets the async producer Input() buffer size.
func WithChannelBufferSize(n int) Option { return func(o *Options) { o.ChannelBufferSize = n } }

// WithProducerLinger sets the batch flush delay (0 = off; 1-10ms typical).
func WithProducerLinger(d time.Duration) Option { return func(o *Options) { o.ProducerLinger = d } }

// WithMaxBufferedRecords caps the in-flight record count (memory bound).
func WithMaxBufferedRecords(n int) Option { return func(o *Options) { o.MaxBufferedRecords = n } }

// WithBatchMaxBytes caps a single batch's byte size.
func WithBatchMaxBytes(n int) Option { return func(o *Options) { o.BatchMaxBytes = n } }

// WithCodec installs a value (de)serialiser.
func WithCodec(c Codec) Option { return func(o *Options) { o.Codec = c } }

// WithConsumerOffsetInitial sets the group's initial offset (OffsetNewest/Oldest).
func WithConsumerOffsetInitial(o int64) Option {
	return func(opts *Options) { opts.ConsumerOffsetInitial = o }
}

// WithDeliveryMode selects PartitionConsumer delivery ("callback"/"channel").
func WithDeliveryMode(m string) Option { return func(o *Options) { o.DeliveryMode = m } }

// defaultOptions returns the package defaults — the single source of truth.
func defaultOptions() Options {
	return Options{
		ReturnSuccesses:       true,
		ReturnErrors:          true,
		ProducerTimeout:       10 * time.Second,
		RetryMax:              3,
		ChannelBufferSize:     256,
		ConsumerOffsetInitial: OffsetNewest,
		FetchMin:              1,
		DeliveryMode:          "callback",
	}
}

// withDefaults returns a copy of o with every zero field replaced by the
// corresponding default. Non-zero fields are preserved.
func (o Options) withDefaults() Options {
	d := defaultOptions()
	if o.ProducerTimeout <= 0 {
		o.ProducerTimeout = d.ProducerTimeout
	}
	if o.RetryMax <= 0 {
		o.RetryMax = d.RetryMax
	}
	if o.ChannelBufferSize <= 0 {
		o.ChannelBufferSize = d.ChannelBufferSize
	}
	if !o.ReturnErrors { // default true; keep an explicit false only if caller set ReturnSuccesses
		// ReturnErrors defaults to true; a zero value (false) is treated as
		// "unset" → default true, because silently dropping broker errors is
		// almost never wanted. Callers who truly want fire-and-forget errors
		// can't disable via the struct zero; this matches sarama's own default.
		o.ReturnErrors = d.ReturnErrors
	}
	if !o.ReturnSuccesses && o.consumerNeedsSuccesses() {
		// producers always need the success path for Metrics/OnSuccess.
		o.ReturnSuccesses = d.ReturnSuccesses
	}
	if o.ConsumerOffsetInitial == 0 {
		o.ConsumerOffsetInitial = d.ConsumerOffsetInitial
	}
	if o.FetchMin <= 0 {
		o.FetchMin = d.FetchMin
	}
	if o.DeliveryMode == "" {
		o.DeliveryMode = d.DeliveryMode
	}
	return o
}

// consumerNeedsSuccesses reports whether the success path must be on. Producers
// always need it (for Metrics.Success / OnSuccess). Consumer options ignore it.
func (o Options) consumerNeedsSuccesses() bool { return true }

// validate checks the options required by the named constructor role.
// role is "producer", "consumer-group", or "partition-consumer".
func (o Options) validate(role string) error {
	if len(o.Brokers) == 0 {
		return errors.New("kafka: brokers required")
	}
	switch role {
	case "producer":
		// Topic is optional per-message (Message.Topic can override), so allow empty.
	case "consumer-group":
		if o.GroupID == "" {
			return errors.New("kafka: group_id required for consumer group")
		}
	case "partition-consumer":
		if o.Topic == "" {
			return errors.New("kafka: topic required for partition consumer")
		}
	}
	return nil
}
