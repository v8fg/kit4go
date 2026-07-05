package kafka

import (
	"errors"
	"log"
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

	// ChannelBufferSize is the sarama channel buffer size — for the producer it
	// sizes the Input() channel (no equivalent on franz-go); for consumers it
	// sizes the per-partition message channel. For the producer it IS sarama's
	// real hard backpressure point (vs MaxBufferedRecords, which on sarama is
	// only a soft flush trigger — see that field). Default tracks
	// MaxBufferedRecords (1000) so producer channel backpressure stays aligned
	// with the in-flight cap (consumers get the same per-partition buffering).
	// Configure up for bursty load, down to bound memory. SIZE: peak sarama
	// producer in-flight ≈ ChannelBufferSize + current batch (≤MaxBufferedRecords)
	// ≈ 2× the value. REFRESH: producer flush timing is independent of this
	// (governed by ProducerLinger / MaxBufferedRecords / BatchMaxBytes on the
	// batch); the channel adds no latency — only burst headroom + backpressure.
	ChannelBufferSize int `json:"channel_buffer_size" mapstructure:"channel_buffer_size"`

	// ProducerLinger is how long the backend waits before flushing a partial
	// batch. Default DefaultProducerLinger (10ms) — applied when left at 0, so
	// both backends batch identically (seamless swap). >0 (e.g. 1ms) accumulates
	// records for up to this duration before flushing → larger batches → fewer
	// RPCs → higher throughput, at the cost of added per-record latency. Pass
	// LingerOff to flush every record immediately (lowest latency, no batching).
	// Pairs with SendBatch for maximum effect. Memory impact: see Metrics.InFlight.
	//
	// DATA-LOSS RISK: records buffered during the linger window are lost if the
	// process crashes before Close() flushes them. Keep linger small (≤10ms) and
	// always defer Close() on shutdown. Monitor Metrics.InFlight for buffer depth.
	ProducerLinger time.Duration `json:"producer_linger" mapstructure:"producer_linger"`

	// CloseFlushTimeout bounds the franz-go producer's final Flush during Close
	// (drains in-flight records before the client closes). <=0 → 30s. This sits
	// OUTSIDE log4go's daemon-shutdown bound, so total Stop ≈ daemon-timeout +
	// CloseFlushTimeout; lower it when the deployment's shutdown grace is tight.
	CloseFlushTimeout time.Duration `json:"close_flush_timeout" mapstructure:"close_flush_timeout"`

	// Acks controls the producer's required broker acknowledgments — the core
	// durability-vs-throughput knob. A string (AcksLeader / AcksAll / AcksNone).
	// Default AcksLeader (UNIFIED across both backends — applied to both async
	// and sync). Both backends default to acks=leader for seamless swap.
	//
	// Set AcksAll for durability (enables franz-go's idempotent producer; slower
	// under RF>1 — waits for all in-sync replicas). DATA-LOSS: acks=leader can
	// lose records if the leader fails before replication — acceptable for
	// ad-tech logs, NOT for money/critical state (use AcksAll + RF=3 there).
	// See STRESS_MATRIX.md for the acks sweep data.
	Acks string `json:"acks" mapstructure:"acks"`

	// MaxBufferedRecords caps in-flight records. Default DefaultMaxBufferedRecords
	// (1000) — applied when left at 0. SEMANTICS DIFFER BY BACKEND (honest
	// contract, not smoothed-over):
	//   - franz-go: MaxBufferedRecords — a HARD cap; Send/SendBatch blocks
	//     (backpressure) when ≥N records are in-flight unacked.
	//   - sarama: Flush.Messages — a SOFT flush trigger (flush when N messages
	//     accumulate; does NOT block Send). sarama's real hard backpressure is
	//     ChannelBufferSize (which defaults to this value — see that field).
	// Memory bound: ≈ MaxBufferedRecords × avg_msg_size (franz-go hard; sarama
	// ≈2× due to channel+batch). Raise for bursty loads, lower to bound memory.
	MaxBufferedRecords int `json:"max_buffered_records" mapstructure:"max_buffered_records"`

	// BatchMaxBytes caps a single batch's byte size. 0 = backend default
	// (sarama: 1MB via MaxMessageBytes; franz-go: 1MB via ProducerBatchMaxBytes).
	// Larger batches amortize RPC overhead but increase memory per batch.
	BatchMaxBytes int `json:"batch_max_bytes" mapstructure:"batch_max_bytes"`

	// SnapshotHistory is the number of recent ProducerSnapshot samples to retain
	// for trend analysis (obtained via the optional SnapshotHistory interface /
	// History()). 0 (default) disables history — Snapshot() still works but no
	// samples are retained (zero memory). Recommended: 60-288 (1-5 min at a 1s
	// scrape cadence, or 1-5h at 1min). Memory: ~160B per sample (bounded by
	// this cap). Negative values are clamped to 0. Sampling is scrape-driven:
	// each Snapshot() call records one sample (Prometheus model); no background
	// goroutine.
	SnapshotHistory int `json:"snapshot_history" mapstructure:"snapshot_history"`

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

// logConfig prints the resolved configuration at startup so operators see
// exactly what is running. Both backends call this from their constructors.
// The durability hint reminds ad-tech users (default acks=leader) how to
// switch to durable mode.
func logConfig(mode string, o Options) {
	acks := o.Acks
	if acks == "" {
		acks = AcksLeader + " (default)"
	}
	log.Printf("[kafka] %s backend=%s acks=%s linger=%s maxBufferedRecords=%d batchMaxBytes=%d "+
		"topic=%s name=%s — for durability set WithAcks(AcksAll)",
		mode, backendName, acks, effectiveLinger(o.ProducerLinger),
		o.MaxBufferedRecords, o.BatchMaxBytes, o.Topic, nameOr(o.Name, o.Topic))
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

// WithCloseFlushTimeout sets the franz-go producer's final Flush timeout on
// Close (<=0 → 30s). See Options.CloseFlushTimeout.
func WithCloseFlushTimeout(d time.Duration) Option {
	return func(o *Options) { o.CloseFlushTimeout = d }
}

// WithAcks sets the producer's required acknowledgments: AcksLeader (default),
// AcksAll (all in-sync replicas — durable, slower under RF>1), or AcksNone
// (fire-and-forget). Applied uniformly to both backends.
func WithAcks(a string) Option { return func(o *Options) { o.Acks = a } }

// WithMaxBufferedRecords caps the in-flight record count (memory bound).
func WithMaxBufferedRecords(n int) Option { return func(o *Options) { o.MaxBufferedRecords = n } }

// WithBatchMaxBytes caps a single batch's byte size.
func WithBatchMaxBytes(n int) Option { return func(o *Options) { o.BatchMaxBytes = n } }

// WithSnapshotHistory enables retaining the last n Snapshot() samples for trend
// analysis (obtained via the optional SnapshotHistory interface). n ≤ 0 disables
// history (the default). See the SnapshotHistory option field for sizing guidance.
func WithSnapshotHistory(n int) Option {
	return func(o *Options) {
		if n < 0 {
			n = 0
		}
		o.SnapshotHistory = n
	}
}

// WithCodec installs a value (de)serialiser.
func WithCodec(c Codec) Option { return func(o *Options) { o.Codec = c } }

// WithConsumerOffsetInitial sets the group's initial offset (OffsetNewest/Oldest).
func WithConsumerOffsetInitial(o int64) Option {
	return func(opts *Options) { opts.ConsumerOffsetInitial = o }
}

// WithDeliveryMode selects PartitionConsumer delivery ("callback"/"channel").
func WithDeliveryMode(m string) Option { return func(o *Options) { o.DeliveryMode = m } }

// Package-level producer tuning defaults. These are the kit4go-chosen values,
// applied by withDefaults when the caller leaves the corresponding option at its
// zero value. They deliberately DIFFER from the underlying backends' native
// defaults (documented inline) so both backends converge on identical behavior
// (the "无感切换" / seamless-swap goal).
const (
	// DefaultProducerLinger is the batch flush delay applied when
	// WithProducerLinger is not used. 10ms.
	//
	// Why 10ms:
	//  1. Matches franz-go's native default (lingered batching is part of why
	//     franz-go measures ~2.9× faster than sarama on raw Produce).
	//  2. Unified across both backends → seamless swap (erases the sarama=0 /
	//     franz-go=10ms silent asymmetry that existed when this defaulted to 0).
	//  3. Ad-tech / log pipelines tolerate ~10ms latency in exchange for larger
	//     batches → fewer RPCs → higher throughput.
	//
	// Cost: each record may wait up to 10ms before flush; records buffered
	// during the linger window are LOST if the process crashes before Close()
	// flushes them — always defer Close() on shutdown. Disable batching
	// (per-record flush, lowest latency) with WithProducerLinger(LingerOff).
	//
	// Native defaults this overrides: sarama Flush.Frequency=0 (off);
	// franz-go linger=10ms (on by default — we now set it explicitly to pin
	// behavior rather than rely on the native default).
	DefaultProducerLinger = 10 * time.Millisecond

	// DefaultMaxBufferedRecords caps in-flight records applied when
	// WithMaxBufferedRecords is not used. 1000.
	//
	// Contract (honest, not "smoothed-over"): best-effort in-flight record
	// bound for backpressure/memory guidance, applied via each backend's
	// native primitive:
	//   - franz-go: MaxBufferedRecords — a HARD cap; Send blocks (backpressure)
	//     when ≥N records are in-flight unacked.
	//   - sarama: Flush.Messages — a SOFT flush trigger (flush when N messages
	//     accumulate, does NOT block Send). sarama's real hard backpressure
	//     point is the Input channel (ChannelBufferSize, see below).
	// Neither backend can grow in-flight without bound (sarama: channel+batch;
	// franz-go: hard cap). Monitoring (InFlight/BufferedBytes) uses a UNIFORM
	// formula across backends → consistent VISIBILITY; but under broker
	// slowness the sarama value may differ from franz-go's (soft vs hard).
	// Monitoring gives visibility, not an isomorphic guarantee.
	//
	// More conservative than franz-go's native 10000 (1000 × ~200B ≈ 200KB/
	// instance) to bound memory. Native default overridden: franz-go
	// maxBufferedRecords=10000; sarama Flush.Messages=0 (no count trigger).
	DefaultMaxBufferedRecords = 1000

	// LingerOff disables batch lingering (flush every record immediately,
	// lowest latency). Pass to WithProducerLinger for latency-sensitive
	// single-message sends. Negative sentinel, mirroring the OffsetNewest=-1 /
	// OffsetOldest=-2 convention used for offset sentinels.
	LingerOff time.Duration = -1
)

// Acks values for Options.Acks (the producer durability-vs-throughput knob).
// Industry guidance:
//   - AcksLeader (acks=1, the default): ad-tech / telemetry / logs / metrics —
//     throughput-first, a lost record on leader failure is acceptable. kit4go's
//     domain.
//   - AcksAll (acks=all, all in-sync replicas): finance / payments / orders /
//     audit — durability-first, no data loss; pair with RF=3 + min.insync=2.
//     Slower under RF>1 (every record replicated before ack).
//   - AcksNone (acks=0, fire-and-forget): extreme throughput, fully loss-tolerant
//     (some metrics, best-effort).
const (
	AcksLeader = "leader" // default
	AcksAll    = "all"
	AcksNone   = "none"
)

// defaultOptions returns the package defaults — the single source of truth.
// Note: ChannelBufferSize is intentionally NOT set here — withDefaults derives
// it from MaxBufferedRecords (see withDefaults) so the two stay coupled.
func defaultOptions() Options {
	return Options{
		ReturnSuccesses:       true,
		ReturnErrors:          true,
		ProducerTimeout:       10 * time.Second,
		RetryMax:              3,
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
	// Batch-tuning defaults (async producers): 0 → kit4go defaults so both
	// backends converge (see DefaultProducerLinger / DefaultMaxBufferedRecords).
	// LingerOff(-1) is preserved (explicit disable); a positive value is honored.
	if o.ProducerLinger == 0 {
		o.ProducerLinger = DefaultProducerLinger
	}
	if o.MaxBufferedRecords == 0 {
		o.MaxBufferedRecords = DefaultMaxBufferedRecords
	}
	// ChannelBufferSize tracks MaxBufferedRecords by default (保持一致): sarama's
	// Input channel IS its real hard backpressure point, so sizing it to match
	// the in-flight cap keeps the two aligned (and narrows the soft/hard gap
	// vs franz-go). SIZE: peak sarama in-flight ≈ ChannelBufferSize (input
	// queue) + current batch (≤MaxBufferedRecords) ≈ 2×MaxBufferedRecords —
	// bounded; lower either to halve peak. REFRESH: flush timing is governed by
	// Flush.Frequency/Flush.Messages/Flush.Bytes on the BATCH and is INDEPENDENT
	// of ChannelBufferSize (the channel is burst headroom + backpressure only;
	// no added latency, no stall — the dispatch goroutine drains it continuously).
	if o.ChannelBufferSize <= 0 {
		o.ChannelBufferSize = o.MaxBufferedRecords
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
