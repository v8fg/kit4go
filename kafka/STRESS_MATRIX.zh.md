# kafka producer 压测矩阵（2026-06-29，修复后）

> English: [`STRESS_MATRIX.md`](STRESS_MATRIX.md)

apache/kafka 3.8.0 KRaft，单节点 RF=1，本机，Apple M5 10c，Go 1.26。
`sync` = `NewSyncProducer`；`async` = `NewProducer` + `SendBatch`。linger 10ms。
内存列：`bufMB` = `producer.Metrics().BufferedBytes`（峰值在途缓冲），
`heapMB` = Go `HeapAlloc` 相对 GC 基线的增量（clamp 到 0）。msgs 切片复用；建 producer 后静置 200ms。
数据量：100B/1KB → 1M 条，10KB → 200K，100KB → 20K。

送达已核实：两后端 topic offset = N（franz-go 在 `Close()` flush 修复后；修复前差 ~0.4%）。
下面是全量送达吞吐，不是入队吞吐。

## 同步（100B；延迟瓶颈，与 size 无关）。bufMB = 0

| 方法(批) | sarama rec/s | franz-go rec/s |
|---|---|---|
| 逐条 Send | 3,227 | 2,699 |
| SendBatch 128 | 160,575 | 266,642 |
| SendBatch 1024 | 339,303 | 739,819 |
| SendBatch 10000 | 505,682 | 1,147,169 |

逐条同步两后端都 ~3K rec/s（每条一次 broker 往返）。要冲 QPS 用 SendBatch；10000 批 franz-go 1.15M，sarama 506K（2.3×）。

## 异步 rec/s 矩阵（行 = payload，列 = batchsize）

sarama
| payload | 128 | 256 | 512 | 1024 | 2048 | 4096 | 10000 |
|---|---|---|---|---|---|---|---|
| 100B | 638.7K | 571.9K | 637.9K | 647.2K | 622.1K | 671.2K | 720.5K |
| 1KB | 187.7K | 199.7K | 188.1K | 195.7K | 195.2K | 201.9K | 213.6K |
| 10KB | 23.7K | 23.9K | 24.1K | 25.2K | 25.4K | 25.0K | 23.7K |
| 100KB | 2.47K | 2.52K | 2.57K | 2.57K | 2.39K | 2.46K | 2.61K |

franz-go
| payload | 128 | 256 | 512 | 1024 | 2048 | 4096 | 10000 |
|---|---|---|---|---|---|---|---|
| 100B | 737.1K | 796.8K | 817.3K | 846.7K | 757.3K | 719.7K | 840.4K |
| 1KB | 423.3K | 415.6K | 387.9K | 436.8K | 403.8K | 442.4K | 435.4K |
| 10KB | 123.6K | 117.5K | 137.6K | 135.4K | 119.1K | 123.9K | 125.2K |
| 100KB | 9.92K | 10.68K | 9.68K | 9.27K | 11.13K | 10.41K | 13.26K |

## 异步 MB/s 矩阵

sarama（10KB+ 饱和 ~250 MB/s）
| payload | 128 | 256 | 512 | 1024 | 2048 | 4096 | 10000 |
|---|---|---|---|---|---|---|---|
| 100B | 60.9 | 54.5 | 60.8 | 61.7 | 59.3 | 64.0 | 68.7 |
| 1KB | 183.3 | 195.0 | 183.7 | 191.1 | 190.6 | 197.2 | 208.6 |
| 10KB | 231.5 | 233.2 | 235.6 | 246.2 | 248.0 | 244.6 | 231.9 |
| 100KB | 241.4 | 246.0 | 251.2 | 250.8 | 233.2 | 240.0 | 254.5 |

franz-go（10KB+ ~1.2–1.34 GB/s）
| payload | 128 | 256 | 512 | 1024 | 2048 | 4096 | 10000 |
|---|---|---|---|---|---|---|---|
| 100B | 70.3 | 76.0 | 77.9 | 80.8 | 72.2 | 68.6 | 80.1 |
| 1KB | 413.3 | 405.8 | 378.8 | 426.5 | 394.4 | 432.0 | 425.2 |
| 10KB | 1206.9 | 1147.2 | 1344.2 | 1322.5 | 1162.8 | 1210.0 | 1222.2 |
| 100KB | 968.5 | 1042.6 | 945.4 | 904.9 | 1086.8 | 1016.9 | 1294.9 |

## 异步内存 bufMB（在途缓冲，跨 batchsize 几乎恒定，按 payload 汇总）

| payload | sarama bufMB | franz-go bufMB | franz-go/sarama |
|---|---|---|---|
| 100B | 0.3 | 0.1 | ~0.3× |
| 1KB | 3.0 | 0.1–1.0 | ~0.3× |
| 10KB | 20.7 | 9.3 | 0.45× |
| 100KB | 197.3 | 96.9 | 0.49× |

bufMB 随 payload 线性增长（≈ MaxBufferedRecords(1000) × payload），与 batchsize 无关；franz-go 约为 sarama 一半。

异步峰值：100B rec/s sarama 720K / franz-go 847K；10KB+ MB/s sarama ~250 / franz-go ~1.3 GB/s。franz-go 需 batch≥1024（更小批更慢），sarama 对 batch 不敏感。

## 多节点（3 broker，RF=3，12 分区，loopback）

3-broker KRaft 单机集群，topic `stress-mn`（RF=3，12 分区，leader 跨 broker），null-key 记录轮询到各分区：

| 模式 | payload | sarama（acks=leader） | franz-go（acks=all） |
|---|---|---|---|
| async rec/s | 100B | 715,860 | 32,376 |
| async MB/s | 1KB | 67.6 | 慢（acks=all）|
| async MB/s | 10KB | 72.2 | 慢 |
| sync SendBatch | 100B | 55,959（负载下投递失败）| — |

RF=3 复制把字节瓶颈吞吐压到约三分之一：sarama async 在 100B/1KB/10KB 都饱和在 ~70 MB/s，单节点是 210–250 MB/s，因为每条记录写三份。acks=all（franz-go）比 acks=leader（sarama）慢很多，每条 produce 等全 ISR 复制；100B 从 715K 掉到 32K。100B 在 acks=leader 下不受 RF=3 影响（715K，与单节点持平），复制成本只影响字节瓶颈的 payload。

三个 broker 跑在一台主机上不扩展：共享磁盘/CPU/内存，没有 3× 增益，持续负载下还会 OOM。loopback 多节点只能测复制开销，测不出集群扩展。

## 节点数

上面单节点矩阵都是 1 broker、loopback、RF=1，是最佳上限（无网络、无复制）。真实集群有网络和复制，绝对吞吐更低。

独立主机的多节点在 acks=leader 下能 ~N× 扩展（每个 broker 独占资源）；acks=all 受复制网络限制。本地 loopback 测不出这个。要超过单 producer 峰值，分片到多 producer + 分区（acks=leader 下约 0.85M rec/s/片）。

调参（batchsize 1024、linger 10ms、MaxBufferedRecords 1000）与节点数无关；只有吞吐上限随节点数变化，acks 决定复制下的吞吐/持久权衡。

多节点测试：`stress_multinode_test.go`（`-tags integration`），需 3-broker 集群和预建 RF=3 topic（见测试注释）。

## acks 扫描（leader vs all），3-broker RF=3，loopback，100K/50K 量

acks 仅在 RF>1 时有影响（RF=1 时 leader 和 all 相同）。在 3-broker RF=3 集群（topic `stress-acks`，12 分区）扫 acks：

| 模式 | payload | sarama leader | sarama all | franz-go leader | franz-go all |
|---|---|---|---|---|---|
| async rec/s | 100B | 322K | 563K（噪声）| 337K | 46K |
| async MB/s | 1KB | 113 | 130 | 245 | 200 |
| async MB/s | 10KB | 141 | 133 | 722 | 599 |
| sync rec/s | 100B | 122K | 78K | 725K | 521K |

franz-go 100B 在 leader 下比 all 快 7.3×（337K vs 46K）：小 payload 下每条的 ISR 等待主导。大 payload 差距收窄到 ~1.2×。franz-go sync 在 leader 下 725K rec/s，是可行的高 QPS 路径（all 降到 521K）。sarama 在 100B/1KB 这个量上噪声大；10KB 和 sync 上 leader 领先 all。

`acks` 可通过 `Options.Acks` / `WithAcks` 配置（`AcksLeader`/`AcksAll`/`AcksNone`）。默认 AcksLeader（两后端统一——franz-go 幂等 producer 关闭；设 AcksAll 开启）；显式设置可统一。acks=leader 或 none 会关闭 franz-go 的幂等 producer（幂等需要 all）。

## acks 指南

| acks | 何时用 | 权衡 |
|---|---|---|
| AcksLeader（acks=1）| 广告、RTB、遥测、日志、指标 | 吞吐优先；leader 在复制前宕机会丢记录。sarama 默认。 |
| AcksAll（acks=all）| 金融、支付、订单、审计、合规 | 持久优先；配 RF=3 + min.insync=2 + franz-go 幂等。更慢，尤其小 payload。franz-go 默认。 |
| AcksNone（acks=0）| 极限吞吐、完全容忍丢失（部分指标、尽力而为）| fire-and-forget，broker 不回执。 |

吞吐优先（广告/日志）选 leader；资金和关键状态选 all（配 RF=3）。RF=1 集群 acks 无意义，用 leader。

### ad-tech 分层 acks（参考模式）

生产 ad-tech 栈按层分 acks：

| 层（角色）| acks | 原因 |
|---|---|---|
| 热 RTB 竞价器（每请求 RR/detail 日志）| leader | 最低延迟；RR 日志可丢 |
| 预算/消耗控制 | all | 钱；一条都不能丢；RF=3 + min.insync=2 |
| 日志归集/转发 | all | 分析管道不能丢事件 |
| 追踪/归因（展示、点击、转化）| all | 收入归因完整 |
| postback / S2S 转发 | all | 下游伙伴必须收到每个事件 |

延迟敏感热路径用 leader，资金和数据完整性下游用 all。`Acks` 选项覆盖两层（竞价器 `WithAcks(AcksLeader)`，下游 `WithAcks(AcksAll)`）。

## 推荐默认值

SendBatch size 1024 是合适默认值：franz-go 需 ≥1024 才到峰值（更小批在部分 payload 上慢 10–40%），sarama 对 batch 不敏感所以 1024 无代价，内存不受 batchsize 影响（在途缓冲受 MaxBufferedRecords × payload 约束）。log4go `KafKaWriterOptions.BatchSize` 默认 1024（`DefaultKafkaBatchSize`，原 100）。

| 参数 | 推荐 | 原因 |
|---|---|---|
| SendBatch size | 1024 | 合适默认值；log4go 默认 |
| ProducerLinger | 10ms | 吞吐/延迟平衡 |
| MaxBufferedRecords | 1000（大记录调低）| 限内存（≈ Mbr × payload） |
| 后端 | sarama（acks=leader）冲吞吐；franz-go 冲持久/EOS 且更省内存 | franz-go acks=all 在 RF=3 下慢很多 |
| acks | leader 冲吞吐；all 冲持久 | RF=3 下 all 慢很多（100B：32K vs 715K） |
| payload | 100B–1KB 冲 rec/s；10KB+ 冲 MB/s | 字节天花板 ~250 MB/s（sarama）/ ~1.3 GB/s（franz-go），复制下 ÷ RF |
| 集群 | 分区数对齐 broker 数；超单 producer 峰值就分片（~0.85M/片，acks=leader）| 随节点数扩展仅在独立主机成立 |

## 调参

| 目标 | 配置 | 预期 |
|---|---|---|
| 同步最高 QPS | SyncProducer.SendBatch，批≥10000，franz-go | ~1.15M rec/s（sarama ~0.5M） |
| 异步最高 rec/s | 100B，批≥1024，franz-go | ~0.85M rec/s（sarama ~0.72M） |
| 异步最高 MB/s | 10KB+，批≥512–1024，franz-go | ~1.3 GB/s（sarama ~250 MB/s） |
| 最省内存 | 小 payload；franz-go（半内存）| 100B <1MB；1KB 1–3MB |
| 限制大记录内存 | 降 MaxBufferedRecords（默认 1000）| 100KB @ Mbr=100 → ~10MB |
| batch size | sarama：任意；franz-go：≥1024 | — |
| 超峰值 QPS | 分片：N producer + 分区 | ~0.85M/片 franz-go |

## 变更（2026-06-29）

- franz-go `Close()` 先 flush 再关（`franzgo_producer.go` 的 `flushAndClose`：`cl.Flush(30s ctx)` 后 `cl.Close()`）。offset 现 = N（原差 ~0.4%），消除关停丢数据。
- 压测 `heapMB` 下溢 clamp 到 0（GC 中途缩堆）。
- `acks` 可配置（`Options.Acks`、`WithAcks`）；默认 AcksLeader（两后端统一——franz-go 幂等 producer 关闭；设 AcksAll 开启）。
- log4go `BatchSize` 默认 100 → 1024。

复跑：`KAFKA_BROKERS=localhost:9092 go test -tags integration -run StressMatrix -timeout 4h ./...`
再加 `-tags 'integration franzgo'`。单节点需 RF=1 环境变量（见 `reference_kafka_local_broker`）。
