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
func buildSaramaConfig(o Options) (*sarama.Config, error) {
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

	// consumer
	cfg.Consumer.Return.Errors = o.ReturnErrors
	cfg.Consumer.Fetch.Min = int32(o.FetchMin)
	if o.FetchMin > 0 {
		cfg.Consumer.Fetch.Min = o.FetchMin
	}
	cfg.Consumer.Offsets.Initial = mapOffsetInitial(o.ConsumerOffsetInitial)

	return cfg, nil
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
