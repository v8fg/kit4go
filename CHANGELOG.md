# Changelog

All notable changes to **kit4go** are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

kit4go is a **multi-module** repository. Each version tag below covers the root
module and all sub-modules; sub-modules carry matching per-module tags
(e.g. `log4go/v0.3.0` alongside `v0.3.0`).

## [Unreleased]

Build-out of the primitive library, a full-repo quality-review pass, and a
coverage / benchmark / fuzz hardening sweep. 22 commits since v0.3.0.

### Added

#### New primitive packages

- **errcode** — composable error-code registry with categories, wrapping, and
  HTTP/gRPC mapping helpers.
- **objpool** — generic `sync.Pool` wrappers with typed acquire/release and
  optional reset hooks.
- **priorityqueue** — generic heap-based priority queue (min/max, stable tie-break).
- **signalbus** — typed in-process signal/event bus with non-blocking dispatch.
- **hotreload** — file-watcher driven live reload of config/values with debounce
  and observable swap callbacks.
- **signing** — HMAC / RSA / Ed25519 sign + verify helpers with safe default
  encodings.
- **adaptive** — adaptive limiter / EWMA-style controllers that self-tune under
  observed load.

These seven packages round out the P1–P3 primitive tiers (errcode/objpool/
priorityqueue/signalbus as P1, hotreload/signing/adaptive as P2/P3).

### Changed

- **uuid**: migrated from the deprecated satori/uuid to **gofrs/uuid**.
- **semaphore**: rewritten on channels for cleaner, race-free acquire/release.

### Fixed

Full-repo quality-review closure across the new packages and the wider
codebase, applied in two backlog batches plus targeted P0–P1 fixes:

- **P0 quality-review findings**: bounds/edge correctness in `file`, `number`,
  `topk`, `ip`, `otp`.
- **P1 fixes**: `otp` error propagation, `str` unsafe-path migration, `health`
  docs; panic/bound fixes for `datetime.RangeTime`, `bit.Swap`, and a
  `hotreload` regression.
- **quality-review P0/P1** on the four P1 primitives
  (`errcode`/`signalbus`/`signing`/`adaptive`), and **P2/P3** polish on the new
  packages with README gap-fill.
- **batch1 backlog**: `random`, `hotkey`, `freqcap`, `featureflag`, `base62`,
  `errcode`, `ip`.
- **batch2 breaking backlog** (v0.x, behavior-changing): `maxprocs`, `bit`,
  `random`, `str`.

### Tests / Quality

- **batch3a–batch3d coverage push** across all root packages and the seven
  sub-modules — `datetime`/`debounce`/`otp`/`shortlink` to 100%, `grpcclient`
  100%, kafka default backend 99.8% / franzgo 96%; documented unreachable
  branches in `budget`/`countmin`.
- **batch5 Go-native fuzz tests** for `signing`, `otp`, `hash`, `errcode`,
  `priorityqueue`, `shortlink`, `base62`.
- **batch6 benchmarks** — new `bench_test.go` for `bloom`, `topk`,
  `consistenthash`; verified `countmin`/`hyperloglog`/`loadbalance`/`auction`/
  `fsm`/`errcode`/`objpool`.
- **batch7 runnable godoc examples** for `number`, `xlo`, `json`, `backoff`,
  `health`, `datetime`, `hash`, `file`, `random`, `ip`.
- **batch8 coverage**: `shutdown`/`trie`/`udpclient` to 100%; documented
  unreachable paths in `tcpclient`/`freqcap`/`budget`.
- **batch9 sentinel align** — consistent test sentinel values across packages,
  refreshed `BENCHMARK.md`, new `CONTRIBUTING.md`, and fuzz expansion.
- **CI**: added `clickhouse` to the module loop; fixed gofmt gate on
  `fuzz_test.go` files.

## [0.3.0] — 2026-07-05

The ClickHouse release plus a breaking `datetime` refinement, a quality-review
closure on the 7 packages introduced late in v0.2.0, and a coverage push to
≥95%. 21 commits since v0.2.0.

### Added

- **clickhouse** — thin wrapper module around `clickhouse-go/v2` (new
  sub-module; `clickhouse/v0.3.0`). Designed as a focused query/exec surface
  over the official driver rather than a re-imagined client.
- **featureflag**, **backpressure**, **middleware** — Tier-1 service primitives.
- **decimal**, **auction** — Tier-2 domain primitives (fixed-point decimal math;
  second-price / generalized auction utilities).
- **shortlink** — short-code generation + resolution helper.
- **fsm** — minimal deterministic finite-state machine.
- **Mermaid architecture diagram** added to the root README.
- Root README rewritten for v0.2.0/v0.3.0 (50+ packages, 11 sub-modules,
  quality story).

### Changed

- **Makefile** aligned with CI (11 sub-modules + golangci-lint v2).
- **datetime** (BREAKING in a v0.x sense): week first-day is now parameterized,
  and parse errors are surfaced instead of swallowed. Callers passing implicit
  locale defaults may need to supply the first-day argument explicitly.

### Fixed

- **quality-review fixes for 7 new packages** (10 issues), closing the audit
  opened against the late-v0.2.0 batch.
- **clickhouse** P2 polish from quality-review (F1/D9/I6/H3/H6).
- **redis**: `PoolStats` returned zero for every real client.
- **log4go**: atomicized writer `onEvent` hooks (L5/F2 race); replaced a
  hardcoded public broker IP with loopback (I7).
- **kafka + log4go**: exposed `CloseFlushTimeout`; documented the L6 composite
  bound.
- Injected a fake clock into `limiter`/`breaker`/`debounce`/`wtimer` to remove
  an E5 flaky-test class.
- `grpcserver` bumped `x/net` to v0.56.0 (dependabot #1).

### Tests / Quality

- Coverage raised to ≥95% across all packages, including mock-based coverage
  for `postgres`/`email`/`tracing` and boosts for `limiter`/`debounce`/
  `pipeline`/`grpcserver`/`httpserver`. README, `example_test`, and `bench_test`
  added for the 7 new packages.

## [0.2.0] — 2026-07-05

The concurrency + infrastructure release. Adds 18 new feature packages spanning
async/concurrency primitives, network-server wrappers, distributed-coordination
modules, and streaming/algorithm primitives, then closes a 6-round quality audit
across the whole repo. 63 commits since v0.1.0.

### Added

#### Concurrency & async primitives

- **batcher**, **fanout**, **pipeline**, **shutdown**, **workerpool**,
  **semaphore**, **debounce**, **wtimer**, **lru**, **retry**, **backoff**.
- **Library-owned workers recover panics** (callback-recover policy, adopted
  2026-07-05): `workerpool`, `pipeline`, `shutdown`, `wtimer`, and `debounce`
  recover job/callback panics and expose `Recovered()` / `SetOnPanic` hooks. The
  synchronous caller path is left raw.

#### Network-server wrappers

- **httpserver** — high-performance HTTP server + middleware + graceful shutdown.
- **grpcserver** — gRPC server with interceptors + graceful shutdown (new
  sub-module; `grpcserver/v0.2.0`).

#### Distributed-coordination modules

- **redis** — option-configured `go-redis/v9` wrapper (new sub-module).
- **redislock** — distributed lock on Redis (new sub-module).
- **rate** — distributed Redis rate limiter (GCRA) (new sub-module).
- Introduced a committed **`go.work`** so `go build`/`go test` resolve all
  sub-modules together.

#### Streaming & algorithm primitives

- **hyperloglog**, **countmin** — cardinality / frequency sketching (0-alloc
  hashing, hot-path benchmarks).
- **reservoir**, **topk** — reservoir sampling + top-K frequency tracking.
- **trie**, **ringbuffer**.
- **bloom**, **consistenthash**, **loadbalance**.
- **hash**, **idempotency**, **freqcap**, **hotkey**, **budget**, **cache**.

#### Infra wrapper modules

- **email** — go-mail SMTP wrapper (new sub-module).
- **metrics** — Prometheus wrapper (new sub-module).
- **tracing** — OpenTelemetry wrapper (new sub-module).
- **health** — liveness + readiness probes with dependency checks.

#### Utility

- **base62** — short-code codec; **random** gained a numeric verification-code
  helper. **money**, **config**, **limiter** (multi-algorithm: fixed-window /
  leaky / GCRA).

#### log4go & kafka enhancements

- **log4go**: batch daemon + kafka monitoring bridge + overflow constants +
  funnel docs; inline kafka circuit breaker with error-path spill (L4);
  observable daemon death + bounded shutdown (L5b + L6); counts recovered
  field-marshal panics (L5). Added `RESILIENCE.md`.
- **kafka**: configurable acks + `Snapshot`/`History`; franz-go `Close` flush;
  real-time memory monitoring (`BufferedBytes` + `Snapshot`) + batch metrics;
  `sarama` sync batch uses `SendMessages` (real batch); `franzProducer.Send`
  error path reports `bytesFailed`.

### Fixed

6-round full-repo quality audit (commits tagged `audit R1`–`R6`):

- **R1 / audit batch**: close-race data loss + start map race in `batcher`/
  `shutdown`; `fanout` `PublishBlocking`/`Close` deadlock; `pipeline` close
  deadlock + dropped submitter ctx; `workerpool` close/submit panic + results
  deadlock.
- **R2**: `lru`/`debounce` re-entrant deadlock + post-close / double-fire.
- **R3**: `udpclient`/`grpcclient` RNG race + retry-disable + replay-unsafe
  default; `httpclient` retry idempotency gating + RNG race.
- **R4**: `redislock` renewer ctx leak + spurious `onLost` + lost-close-on-panic.
- **R5**: `httpserver`/`grpcserver` shutdown fd leak + surfaced bind error.
- **R6**: `email`/`otp` secure TLS default + otp secret/period correctness.
- P0 quality-review findings in `random`/`money`/`trie`/`pipeline`.

### Changed

- **CI**: aligned Go version (1.26.2) and extended CI to all 11 sub-modules;
  aligned `go.work` directive with `go.mod` (1.26.0); skip timing-sensitive
  batch-vs-per-record test under `-short`.

### Tests / Quality

- Runnable godoc examples for 6 algorithm primitives and 5 pure-utility
  packages; hot-path benchmarks for `hyperloglog`/`countmin`.
- **quality rules**: published a package quality checklist (8 dimensions × 5
  roles), enhanced with industry standards (Uber/Google/K8s/OTel/golangci-lint),
  plus language-neutral rules + a Do-No-Harm dimension + simplicity ethos.
- **lint**: enabled the K2 high-signal `golangci-lint` v2 subset; trimmed the
  lint menu to match the codebase stance (K0); fixed 2 `errorlint` findings.
- Detailed concurrency-safety contracts documented for 10 packages.
- Package READMEs added for 17 previously-undocumented packages.

## [0.1.0] — 2026-06-28

Initial tagged release of the modernized multi-module kit4go. Establishes the
root module plus four standalone sub-modules, the high-performance `log4go`
logger, and the core utility surface. 61 commits.

### Added

#### Core utilities (root module)

- **bit**, **datetime**, **file**, **ip**, **json**, **number**, **otp** (TOTP
  / HOTP), **random**, **str**, **uuid** (requestID, go.uuid, ksuid, xid),
  **xlo** (lightweight helpers, ref `samber/lo`).
- **maxprocs** — GOMAXPROCS auto-tuner.
- **breaker** — circuit breaker.
- **limiter** — rate limiter.
- **latency** — sliding-window p50/p99/p999 histogram + client `LatencyObserver`.
- **httpclient** — HTTP client.
- **tcpclient**, **udpclient**, **grpcclient** — network clients (HTTP/2) +
  stress harness (`grpcclient` is its own sub-module).

#### Sub-modules

- **log4go** — high-performance async structured logger for ad-tech scale:
  console/file/kafka/net/io writers, structured fields (`With`/`WithField`/
  `WithFields`), JSON format (`FormatJSON`), sampling, context binding
  (zerolog-style), request-id middleware, generic overflow
  (ring→file→drop), crash resume, metrics + webhook alerts, multi-core
  ShardLogger, switchable JSON codec (goccy/std/sonic), ~1M qps/core (no-caller).
  See `log4go/PERFORMANCE.md`.
- **postgres** — pgx pool wrapper (pure Go, cross-platform).
- **kafka** — library-swappable producer + consumer wrapper (sync/async, group,
  partition; `sarama` backend at v0.1.0) with `BufferedBytes`/`Snapshot`
  monitoring.

### Infrastructure

- Modernized CI: Go version read from `go.mod`, `go vet` + `gofmt` + `test -race`;
  `golangci-lint` installed via the official script; CI actions bumped to latest
  majors (checkout@v7, setup-go@v6, codecov@v7).
- **log4go** split into an independent module; **postgres** and **grpcclient**
  likewise isolated so importing one does not pull the others' dependencies.
- Verified benchmarks recorded in `BENCHMARKS.md` and `log4go/PERFORMANCE.md`
  §21.

### Fixed

- **log4go**: caller resolution now walks past internal frames (cross-arch
  safe); `NetWriter`/`KafKaWriter` `Stop` made race-free via `CompareAndSwap`.

[Unreleased]: https://github.com/v8fg/kit4go/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/v8fg/kit4go/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/v8fg/kit4go/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/v8fg/kit4go/releases/tag/v0.1.0
