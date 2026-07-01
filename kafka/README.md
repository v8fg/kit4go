# kafka: producer & consumer (sarama / franz-go)

A unified Kafka client that swaps between two backends behind one options API:
sarama (default build tag) and franz-go (`-tags franzgo`). Pick the backend at
build time; application code is unchanged. Both backends default to `acks=leader`
for seamless swap; set `AcksAll` for durability.

See `MONITORING.md` for metrics, and `STRESS_MATRIX.md` / `STRESS_MATRIX.zh.md`
for throughput / durability / data-loss trade-offs and recommended parameters
per workload.

## Features

- **Producer**: sync + async; codec layer (JSON / protobuf / raw bytes); acks
  knob (leader / all / none) applied uniformly to both backends; graceful flush
  before close.
- **Consumer**: partition consumer (channel mode) + consumer-group handler with
  ACK/NACK accounting and setup/cleanup hooks.
- **Options** shared across backends; backend-specific guards (sarama Flush
  deadlock prevention, franz-go idempotent-producer auto-disable on acks != all).
- Codecs: `CodecJSON`, `CodecProto`, `CodecRaw`.

## Usage

- `NewProducer(opts ...Option) (Producer, error)` async producer.
- `NewSyncProducer(opts ...Option) (SyncProducer, error)` sync producer.
- `NewConsumerGroup(opts ...Option) (ConsumerGroup, error)` group consumer w/ ACK/NACK.
- `NewPartitionConsumer(opts ...Option) (PartitionConsumer, error)` single-partition.
- Options: `WithBrokers`, `WithTopic`, `WithGroupID`, `WithPartition`,
  `WithAcks(AcksLeader|AcksAll|AcksNone)`, `WithRetryMax`, `WithProducerLinger`,
  `WithMaxBufferedRecords`, `WithBatchMaxBytes`, `WithChannelBufferSize`, ...
- Codecs: `CodecJSON`, `CodecProto`, `CodecRaw`.

## Build & test

```bash
cd kafka
go test -race -count=1 ./...                 # sarama
go test -tags franzgo -race -count=1 ./...   # franz-go
```

## Notes

- `acks=leader` can lose records if the leader fails before replication: fine for
  loss-tolerant ad-tech logs, not for money or critical state (use `AcksAll`).
- Local integration: apache/kafka KRaft single node must set RF=1 env vars, else
  consumers time out reading 0 despite offsets showing data.
