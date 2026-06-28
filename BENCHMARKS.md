# kit4go Benchmarks

Verified performance profile across all modules. Measured 2026-06-28 on
**Apple M5 (10-core) / Go 1.26.2 / darwin-arm64** via `go test -bench -benchmem`.
Reproduce with `go test -run='^$' -bench=. -benchmem -benchtime=1s ./...` in each
module dir (or `make bench` where defined).

> log4go has its own detailed doc: [`log4go/PERFORMANCE.en.md`](log4go/PERFORMANCE.en.md)
> (§21 holds the latest stress/soak numbers). This file covers the rest of the kit:
> the microservice infrastructure, the network clients, and the end-to-end stress suite.

## Design principle: hot path = zero allocation

Every high-frequency primitive keeps its hot path allocation-free. The pattern across
the kit:

- **atomic CAS** for state transitions (breaker/limiter/NetWriter Stop), no locks on the
  hot path.
- **fixed buckets / ring buffers** (latency histogram, limiter sliding window) — no
  per-op map or slice growth.
- **typed scalar fields / unboxed values** (log4go) — no `interface{}` boxing.
- **object pools** (tcpclient conn pool, log4go record pool) — reuse, not allocate.

## Microservice infrastructure (hot path, single-core)

All sub-100 ns and **0 alloc/op** on the steady-state path. Factory/construct paths
allocate once (expected).

| Package | Operation | ns/op | allocs/op |
|---|---|---|---|
| `breaker` | `Execute` (success) | **67.8** | 0 |
| `breaker` | `Execute` (fail) | 62.2 | 0 |
| `breaker` | `Execute` (parallel) | 270 | 0 |
| `breaker` | `State()` | 0.5 | 0 |
| `breaker` | `Metrics()` | 2.3 | 0 |
| `limiter` | `TokenBucket.Allow` | **60.5** | 0 |
| `limiter` | `TokenBucket.Allow` (parallel) | 224 | 0 |
| `limiter` | `SlidingWindow.Allow` | 61.5 | 0 |
| `limiter` | `TokenBucket.Wait` | 61.6 | 0 |
| `limiter` | `NewLimiter` (factory) | 76.9 | 1 |
| `latency` | `Histogram.Observe` | **60.1** | 0 |
| `latency` | `Histogram.Observe` (parallel) | 217 | 0 |
| `latency` | `ShardHistogram.Observe` (parallel) | 86.2 | 0 |
| `latency` | `Histogram.Quantile` (rare read) | 903 | 0 |
| `latency` | `Histogram.Snapshot` (rare read) | 933 | 0 |
| `latency` | `NewHistogram` (factory) | 2258 | 64 |

`ShardHistogram` keeps `Observe` at 86 ns under parallel contention (vs 217 ns
unsharded) — the sharding removes the single-lock bottleneck for million-QPS
single-instance use.

## Network clients (end-to-end, incl. a local echo/test server)

These include a real round-trip + serialization, so they are µs-scale and allocate
per-call (net/http, grpc, etc. are allocation-heavy by nature). Parallel columns show
the connection-pool payoff.

| Package | Operation | ns/op | B/op | allocs |
|---|---|---|---|---|
| `httpclient` | `Get` | 55 759 | 6 927 | 80 |
| `httpclient` | `Get` (parallel) | **13 234** | 6 986 | 80 |
| `httpclient` | `Post` | 56 715 | 8 020 | 96 |
| `tcpclient` | `Send` | 6 138 | 258 | 5 |
| `tcpclient` | `SendReceive` (parallel) | **44 022** | 10 249 | 40 |
| `tcpclient` | `Pool.GetPut` | 130.0 | 48 | 1 |
| `udpclient` | `Send` | **4 733** | 72 | 1 |
| `udpclient` | `Send` (parallel) | 6 606 | 86 | 2 |
| `udpclient` | `Client.Metrics()` | 2.3 | 0 | 0 |
| `grpcclient` | `Unary` | 31 216 | 9 981 | 165 |
| `grpcclient` | `Unary` (parallel) | **10 429** | 9 640 | 149 |
| `grpcclient` | `Middleware.Metrics` | 2.5 | 0 | 0 |

`udpclient.Send` is the lightest (fire-and-forget, 1 alloc). Pool reuse
(`tcpclient.Pool.GetPut` 130 ns) and parallel dispatch cut latency 2–4×. The metrics
hooks are 0-alloc (nil-overhead when unused).

## log4go (cross-reference)

| Scenario | ns/op (@8 CPU) | allocs | rec/s/core |
|---|---|---|---|
| `LoggerInfo` (caller + writer) | 1468 | 3 | ~680K |
| `NoCaller` (writer) | 1374 | **1** | ~728K |
| `SampledActive` | 1519 | 1 | ~660K |
| `Filtered` (level drop) | 11.5 | **0** | ~87M |
| `SampledOut` (sample drop) | 3.27 | **0** | ~306M |

Multi-core: 1 → 4 CPU ≈ 4× scaling, plateau at 4–8 CPU (channel scheduling).
See [`log4go/PERFORMANCE.en.md`](log4go/PERFORMANCE.en.md) §21 for the full matrix,
codec numbers, and the soak/leak verification.

## End-to-end stress & soak

- **`stress/`** — `TestStress_AllClients` + `TestStress_ConcurrentSafety`: 10K ops × 5
  client types, **PASS under `-race`** (no data races).
- **log4go soak** (10s sustained @ 8 CPU): throughput **identical to the 2s baseline**
  (no degradation); **1M records + GC → +3 KB heap** (no leak); **goleak → 0 goroutine
  leak** after Close.

## Verdict

The kit meets the ad-tech target band (**100k–1M+ QPS**):

- every hot-path primitive is sub-100 ns and 0-alloc;
- log4go delivers ~700K rec/s/core with a writer, 0-alloc drop paths;
- clients pool/reuse and scale 2–4× under parallel load;
- sustained load is stable, no memory/goroutine leak, race-clean.

Re-run after any hot-path change: `go test -run='^$' -bench=. -benchmem ./...` in each
module, and `go test -race -short ./...` for the concurrency gate.
