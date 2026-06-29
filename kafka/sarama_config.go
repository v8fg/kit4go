//go:build !franzgo

package kafka

import (
	"github.com/IBM/sarama"
)

// backendName identifies the underlying client library for Backend()/monitoring.
const backendName = "sarama"

// defaultKafkaVersion is the sarama version assumed when Options.Version is
// empty. Pinned to V2_5_0_0 (broad compatibility; matches log4go's KafKaWriter).
var defaultKafkaVersion = sarama.V2_5_0_0

// buildSaramaConfig assembles a *sarama.Config from Options. It is the single
// place sarama config is constructed so a future backend has a parallel
// kafkago_config.go and the mapping stays in one spot.
//
// sync selects whether batch/flush tuning is applied: async producers apply the
// resolved linger (default 10ms) + buffer (default 1000) so records accumulate
// into fewer, larger requests; sync producers force Flush off (see B5) because
// sarama's SyncProducer wraps an AsyncProducer — a non-zero Flush.Frequency
// would stall each SendMessage up to the linger window, and Flush.Messages would
// stall waiting for N messages, diverging from franz-go's immediate ProduceSync.
func buildSaramaConfig(o Options, sync bool) (*sarama.Config, error) {
	cfg := sarama.NewConfig()

	ver := defaultKafkaVersion
	if o.Version != "" {
		v, err := sarama.ParseKafkaVersion(o.Version)
		if err != nil {
			return nil, err
		}
		ver = v
	}
	cfg.Version = ver

	// producer
	cfg.Producer.Return.Successes = o.ReturnSuccesses
	cfg.Producer.Return.Errors = o.ReturnErrors
	cfg.Producer.RequiredAcks = saramaAcks(o.Acks)
	cfg.Producer.Timeout = o.ProducerTimeout
	cfg.Producer.Retry.Max = o.RetryMax
	if cfg.Producer.Retry.Max < 0 {
		cfg.Producer.Retry.Max = 0
	}
	cfg.ChannelBufferSize = o.ChannelBufferSize
	// partitioner defaults to hash-by-key (same-key -> same-partition, ordered
	// consumption); empty key -> round-robin. sarama.HashPartitioner is the
	// sensible default for request-scoped ordering.
	cfg.Producer.Partitioner = sarama.NewHashPartitioner

	if sync {
		// Sync: per-send blocking, no batching. Flush off → SendMessage flushes
		// immediately (no linger stall, no wait-for-N-messages stall). Linger/
		// MaxBufferedRecords defaults apply ONLY to async (see withDefaults).
		cfg.Producer.Flush.Frequency = 0
		cfg.Producer.Flush.Messages = 0
		cfg.Producer.Flush.Bytes = 0
	} else {
		// Async: linger (default 10ms) triggers periodic flush; Messages/Bytes
		// trigger flush at thresholds — whichever comes first. effectiveLinger
		// maps LingerOff → 0 (batching disabled) and passes the resolved value
		// (DefaultProducerLinger or user-set) through otherwise. Frequency is
		// always set here (never 0 for async), so sarama's "Flush set without
		// Frequency" warning never fires.
		cfg.Producer.Flush.Frequency = effectiveLinger(o.ProducerLinger)
		cfg.Producer.Flush.Messages = o.MaxBufferedRecords
		if o.BatchMaxBytes > 0 {
			cfg.Producer.Flush.Bytes = o.BatchMaxBytes
		}
	}

	// consumer
	cfg.Consumer.Return.Errors = o.ReturnErrors
	cfg.Consumer.Fetch.Min = int32(o.FetchMin)
	if o.FetchMin > 0 {
		cfg.Consumer.Fetch.Min = o.FetchMin
	}
	cfg.Consumer.Offsets.Initial = mapOffsetInitial(o.ConsumerOffsetInitial)

	return cfg, nil
}

// saramaAcks maps Options.Acks to sarama's RequiredAcks. Empty/unknown → leader
// (acks=1, the package default — throughput-first, matches the historical sarama
// default and unifies both backends on leader unless AcksAll/AcksNone is set).
func saramaAcks(a string) sarama.RequiredAcks {
	switch a {
	case AcksAll:
		return sarama.WaitForAll
	case AcksNone:
		return sarama.NoResponse
	default:
		return sarama.WaitForLocal // AcksLeader / ""
	}
}

// mapOffsetInitial maps the package offset sentinels to sarama's. A concrete
// int64 >= 0 is a valid absolute offset for a partition consumer (sarama accepts
// it directly); the sentinels map to sarama's own.
func mapOffsetInitial(o int64) int64 {
	switch o {
	case OffsetNewest:
		return sarama.OffsetNewest
	case OffsetOldest:
		return sarama.OffsetOldest
	default:
		return o
	}
}
