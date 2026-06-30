# metrics

Opinionated wrappers around `prometheus/client_golang`: validated typed builders,
an ad-tech-tuned latency histogram, a registry, and a ready HTTP exposition
handler. One module, isolated deps (prometheus/client_golang).

## Why

prometheus/client_golang is fine-grained; services still copy-paste the same
registry + builder + `/metrics`-handler boilerplate and pick latency buckets
ad hoc. This package bakes in the kit4go conventions and a latency bucket set
concentrated in the 1-50ms bidding range so the bid hot-path tail (p99/p999) is
visible without wasting buckets on multi-second ranges.

## API

```go
reg := metrics.NewRegistry()                       // isolated; or metrics.Default()

bids := reg.NewCounter("bids_total", "bid requests", "ssp")
lat  := reg.NewLatencyHistogram("bid_latency_seconds", "decision latency", "ssp")
budget := reg.NewGauge("budget_remaining", "spend budget", "campaign")

bids.WithLabelValues("rubicon").Inc()
lat.WithLabelValues("rubicon").Observe(0.004)      // 4ms
budget.WithLabelValues("c42").Set(1000)

http.Handle("/metrics", reg.Handler())             // prometheus text exposition
```

| Symbol | Behavior |
|---|---|
| `NewRegistry()` / `Default()` | Fresh isolated / package-level shared registry |
| `NewCounter/NewGauge/NewHistogram(name, help, …labels)` | Build + register a vec |
| `NewLatencyHistogram(name, help, …labels)` | Histogram with `LatencyBuckets` (ad-tech-tuned, seconds) |
| `Register` / `MustRegister` / `Gather` / `Prometheus()` | Registry passthrough |
| `Handler()` | `/metrics` HTTP handler |

`LatencyBuckets` (seconds): `0.001 0.0025 0.005 0.01 0.025 0.05 0.1 0.25 0.5 1 2.5 5 10`.

## Ad-tech use

- **Bid pipeline**: bid QPS, win rate, decision latency, error rate, per-SSP
  breakdown, budget/pacing gauges.
- Pair `NewLatencyHistogram` with `kit4go/latency` for a consistent tail view.

## Testing

100% statement coverage, `-race` clean. Counter/Gauge/Histogram values via
`testutil.ToFloat64`, latency-bucket presence in the exposition text, HTTP
handler output, Gather families, duplicate-Register error, MustRegister, and the
shared Default registry.

```bash
go test -race -cover ./...
```
