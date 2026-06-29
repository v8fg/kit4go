# kafka producer stress matrix (2026-06-29, post-fix)

> 中文版: [`STRESS_MATRIX.zh.md`](STRESS_MATRIX.zh.md)

apache/kafka 3.8.0 KRaft, single node, RF=1, localhost, Apple M5 10c, Go 1.26.
`sync` = `NewSyncProducer`; `async` = `NewProducer` + `SendBatch`. Linger 10ms.
Memory columns: `bufMB` = `producer.Metrics().BufferedBytes` (in-flight buffer at peak),
`heapMB` = Go `HeapAlloc` delta vs a GC'd baseline (clamped at 0). msgs slice reused;
200ms settle after `NewProducer`. Volumes: 100B/1KB → 1M, 10KB → 200K, 100KB → 20K.

Delivery verified: topic offsets = N for both backends (franz-go after the `Close()` flush
fix; was N−0.4% before). Throughput numbers are full-delivery, not enqueue-only.

## sync (100B; latency-bound, payload-independent). bufMB = 0

| method (batch) | sarama rec/s | franz-go rec/s |
|---|---|---|
| per-record | 3,227 | 2,699 |
| SendBatch 128 | 160,575 | 266,642 |
| SendBatch 1024 | 339,303 | 739,819 |
| SendBatch 10000 | 505,682 | 1,147,169 |

Per-record sync is ~3K rec/s on both backends (one broker round-trip per record). For QPS,
use `SendBatch`; at batch 10000 franz-go does 1.15M vs sarama 506K (2.3×).

## async (payload × batchsize). bufMB is flat across batch sizes.

### sarama (byte-bound, batch-insensitive)

| payload | peak rec/s (batch) | peak MB/s | bufMB | heapMB |
|---|---|---|---|---|
| 100B | 720,464 (10000) | 69 | 0.3 | 1–3 |
| 1KB | 213,602 (10000) | 209 | 3.0 | 4–7 |
| 10KB | 25,392 (2048) | 248 | 20.7 | 4–6 |
| 100KB | 2,606 (10000) | 255 | 197.3 | 5–6 |

### franz-go (batch-sensitive, higher MB/s, ~half sarama's buffer memory)

| payload | peak rec/s (batch) | peak MB/s | bufMB | heapMB |
|---|---|---|---|---|
| 100B | 846,740 (1024) | 81 | 0.1 | 0–2 |
| 1KB | 442,365 (4096) | 432 | ~0.1–1 | 0–15 |
| 10KB | 137,644 (512) | 1,344 | 9.3 | 0–6 |
| 100KB | 13,260 (10000) | 1,295 | 96.9 | 0–12 |

Async peaks: 100B rec/s sarama 720K / franz-go 847K; 10KB+ MB/s sarama ~250 / franz-go
~1.3 GB/s. franz-go needs batch ≥1024 (smaller batches are slower); sarama is flat.
Memory (`bufMB`) tracks payload, not batch size, bounded by `MaxBufferedRecords` (1000) ×
payload; franz-go holds about half what sarama does at the same payload.

## Multi-node (3 brokers, RF=3, 12 partitions, loopback)

3-broker KRaft on one host, topic `stress-mn` (RF=3, 12 partitions, leaders across brokers),
null-key records round-robined across partitions:

| mode | payload | sarama (acks=leader) | franz-go (acks=all) |
|---|---|---|---|
| async rec/s | 100B | 715,860 | 32,376 |
| async MB/s | 1KB | 67.6 | slow (acks=all) |
| async MB/s | 10KB | 72.2 | slow |
| sync SendBatch | 100B | 55,959 (delivery errors under load) | — |

RF=3 replication cuts byte-bound throughput to about a third: sarama async saturates near
70 MB/s across 100B/1KB/10KB vs single-node 210–250 MB/s, because each record is written to
three brokers. `acks=all` (franz-go) is much slower than `acks=leader` (sarama) here,
waiting on full ISR replication per produce; 100B drops from 715K to 32K. 100B with
acks=leader is unaffected by RF=3 (715K, same as single-node) since replication cost only
bites byte-bound payloads.

Three brokers on one host do not scale: they share one disk/CPU/memory, so there is no 3×
gain, and under sustained load brokers OOM-crashed. Loopback multi-node shows replication
overhead, not cluster scaling.

## Node count

The single-node numbers above are 1 broker, loopback, RF=1, a best-case ceiling (no network,
no replication). Real clusters with network and replication are lower.

Multi-node on separate hosts does scale ~N× for acks=leader (each broker has its own
resources); acks=all is bounded by the replication network. Local loopback cannot show this.
To exceed one producer's peak, shard across producers and partitions (~0.85M rec/s per shard
at acks=leader).

The tuning params (batchsize 1024, linger 10ms, MaxBufferedRecords 1000) are independent of
node count; only the throughput ceiling scales with nodes, and acks decides the
throughput/durability tradeoff under replication.

Multi-node test: `stress_multinode_test.go` (`-tags integration`), needs a 3-broker cluster
and a pre-created RF=3 topic (see the test comment).

## acks sweep (leader vs all), 3-broker RF=3, loopback, 100K/50K volume

acks only matters with RF>1 (on RF=1, leader and all are identical). Sweeping on a 3-broker
RF=3 cluster (topic `stress-acks`, 12 partitions):

| mode | payload | sarama leader | sarama all | franz-go leader | franz-go all |
|---|---|---|---|---|---|
| async rec/s | 100B | 322K | 563K (noise) | 337K | 46K |
| async MB/s | 1KB | 113 | 130 | 245 | 200 |
| async MB/s | 10KB | 141 | 133 | 722 | 599 |
| sync rec/s | 100B | 122K | 78K | 725K | 521K |

franz-go at 100B is 7.3× faster on leader than all (337K vs 46K): at small payloads the
per-record ISR wait dominates. Larger payloads narrow the gap to ~1.2×. franz-go sync on
leader hits 725K rec/s, a viable high-QPS path (all drops it to 521K). sarama is noisy at
100B/1KB at this volume; 10KB and sync show leader ahead of all.

`acks` is configurable via `Options.Acks` / `WithAcks` (`AcksLeader`/`AcksAll`/`AcksNone`).
Default is AcksLeader (unified across both backends — franz-go's idempotent producer is
auto-disabled since it requires acks=all; set AcksAll to enable it).

## acks guidance

| acks | use when | tradeoff |
|---|---|---|
| AcksLeader (acks=1) | ad-tech, RTB, telemetry, logs, metrics | throughput-first; a record can be lost if the leader fails before replicating. sarama default. |
| AcksAll (acks=all) | finance, payments, orders, audit, compliance | durability-first; pair with RF=3 + min.insync=2 + franz-go idempotent producer. Slower, especially at small payloads. franz-go default. |
| AcksNone (acks=0) | extreme throughput, fully loss-tolerant (some metrics, best-effort) | fire-and-forget, no broker reply. |

Pick leader for throughput (ad-tech/logs), all for money and critical state (with RF=3). On
RF=1 clusters acks is irrelevant; use leader.

### Ad-tech tiered acks (reference pattern)

Production ad-tech stacks split acks by tier:

| tier (role) | acks | reason |
|---|---|---|
| hot RTB bidder (per-request RR/detail logs) | leader | lowest latency; RR logs are loss-tolerant |
| budget / spend control | all | money; no record may be lost; RF=3 + min.insync=2 |
| log collection / transfer | all | analytics pipeline must not drop events |
| tracking / attribution (impressions, clicks, conversions) | all | revenue attribution integrity |
| postback / S2S forwarding | all | downstream partners must receive every event |

Latency-critical hot path uses leader; money and data-integrity downstream services use all.
The `Acks` option covers both tiers (`WithAcks(AcksLeader)` for the bidder, `WithAcks(AcksAll)`
for downstream).

## Recommended defaults

SendBatch size 1024 is the right default: franz-go needs ≥1024 to reach peak (smaller batches
are 10–40% slower on some payloads), sarama is flat so 1024 costs nothing, and memory is
unaffected (the in-flight buffer is bounded by MaxBufferedRecords × payload, not batch size).
log4go `KafKaWriterOptions.BatchSize` defaults to 1024 (`DefaultKafkaBatchSize`, was 100).

| param | recommended | why |
|---|---|---|
| SendBatch size | 1024 | good default; log4go default |
| ProducerLinger | 10ms | throughput/latency balance |
| MaxBufferedRecords | 1000 (lower for big records) | bounds memory (≈ Mbr × payload) |
| backend | sarama (acks=leader) for throughput; franz-go for durability/EOS and lower memory | franz-go acks=all is far slower under RF=3 |
| acks | leader for throughput; all for durability | under RF=3, all is far slower (100B: 32K vs 715K) |
| payload | 100B–1KB for max rec/s; 10KB+ for max MB/s | byte ceiling ~250 MB/s (sarama) / ~1.3 GB/s (franz-go), divided by RF under replication |
| cluster | match partitions to brokers; shard past one producer's peak (~0.85M/shard at acks=leader) | scales with nodes only on separate hosts |

## Tuning

| goal | config | expected |
|---|---|---|
| max sync QPS | SyncProducer.SendBatch, batch ≥10000, franz-go | ~1.15M rec/s (sarama ~0.5M) |
| max async rec/s | 100B, batch ≥1024, franz-go | ~0.85M rec/s (sarama ~0.72M) |
| max async MB/s | 10KB+, batch ≥512–1024, franz-go | ~1.3 GB/s (sarama ~250 MB/s) |
| min memory | small payload; franz-go (half sarama buffer) | 100B <1MB; 1KB 1–3MB |
| cap memory for big records | lower MaxBufferedRecords (default 1000) | 100KB at Mbr=100 → ~10MB |
| batch size | sarama: any; franz-go: ≥1024 | — |
| past peak QPS | shard: N producers + partitions | ~0.85M/shard franz-go |

## Changes (2026-06-29)

- franz-go `Close()` flushes before closing (`flushAndClose` in `franzgo_producer.go`:
  `cl.Flush(30s ctx)` then `cl.Close()`). Offsets now = N (was N−0.4%); removes the shutdown
  data-loss gap.
- stress test `heapMB` underflow clamped at 0 (GC shrinking the heap mid-cell).
- `acks` now configurable (`Options.Acks`, `WithAcks`); default keeps each backend's native
  acks.
- log4go `BatchSize` default 100 → 1024.

Rerun: `KAFKA_BROKERS=localhost:9092 go test -tags integration -run StressMatrix -timeout 4h ./...`
then with `-tags 'integration franzgo'`. Single node needs RF=1 env vars (see
`reference_kafka_local_broker`).
