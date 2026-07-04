# Kit4go

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/v8fg/kit4go?color=red)
![Top Languages](https://img.shields.io/github/languages/top/v8fg/kit4go)
[![Go Report Card](https://goreportcard.com/badge/github.com/v8fg/kit4go)](https://goreportcard.com/report/github.com/v8fg/kit4go)
[![License](https://img.shields.io/:license-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/release/v8fg/kit4go.svg?style=flat-square)](https://github.com/v8fg/kit4go/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/v8fg/kit4go/pr.yml?branch=release&label=CI)](https://github.com/v8fg/kit4go/actions/workflows/pr.yml?query=branch%3Arelease)
[![Last Commit](https://img.shields.io/github/last-commit/v8fg/kit4go?label=last%20commit)](https://github.com/v8fg/kit4go)
[![Tests](https://img.shields.io/github/actions/workflow/status/v8fg/kit4go/pr.yml?branch=release&label=tests)](https://github.com/v8fg/kit4go/actions/workflows/pr.yml?query=event%3Apush)
[![codecov](https://codecov.io/gh/v8fg/kit4go/branch/release/graph/badge.svg)](https://codecov.io/gh/v8fg/kit4go)
[![PR](https://img.shields.io/github/issues-pr/v8fg/kit4go?lable=PR)](https://github.com/v8fg/kit4go/pulls)
[![Sourcegraph](https://sourcegraph.com/github.com/v8fg/kit4go/-/badge.svg)](https://sourcegraph.com/github.com/v8fg/kit4go?badge)
[![Open Source Helpers](https://www.codetriage.com/v8fg/kit4go/badges/users.svg)](https://www.codetriage.com/v8fg/kit4go)
[![TODOs](https://badgen.net/https/api.tickgit.com/badgen/github.com/v8fg/kit4go)](https://www.tickgit.com/browse?repo=github.com/v8fg/kit4go)

> Common Go utility library for ad-tech, finance, and blockchain infrastructure.

## Package list

### Root module (`github.com/v8fg/kit4go`)

| Category | Packages |
|----------|----------|
| **Concurrency** | [workerpool](workerpool) · [pipeline](pipeline) · [semaphore](semaphore) · [retry](retry) · [wtimer](wtimer) · [debounce](debounce) · [fanout](fanout) · [shutdown](shutdown) · [batcher](batcher) |
| **Algorithms** | [bloom](bloom) · [countmin](countmin) · [hyperloglog](hyperloglog) · [topk](topk) · [reservoir](reservoir) · [trie](trie) · [ringbuffer](ringbuffer) · [consistenthash](consistenthash) · [loadbalance](loadbalance) |
| **Rate & budget** | [limiter](limiter) (token-bucket/sliding-window/fixed-window/leaky/GCRA) · [budget](budget) · [rate](rate) (Redis-backed) · [hotkey](hotkey) · [freqcap](freqcap) · [idempotency](idempotency) |
| **Cache & storage** | [cache](cache) (unified memory=lru/redis) · [lru](lru) · [breaker](breaker) |
| **Clients** | [httpclient](httpclient) · [tcpclient](tcpclient) · [udpclient](udpclient) |
| **Servers** | [httpserver](httpserver) · [grpcserver](grpcserver) |
| **Utilities** | [bit](bit) · [datetime](datetime) · [file](file) · [ip](ip) · [json](json) · [number](number) · [str](str) · [uuid](uuid) · [xlo](xlo) · [random](random) · [otp](otp) · [base62](base62) · [hash](hash) · [config](config) · [maxprocs](maxprocs) · [backoff](backoff) · [health](health) · [stress](stress) |

### Sub-modules (own go.mod — heavy deps isolated)

| Module | What | Heavy deps |
|--------|------|------------|
| [log4go](log4go) | async structured logging (console/file/kafka/net/io, sampling, ShardLogger, circuit breaker + spill failover, ~1M qps/core) | sarama, sonic, goccy |
| [kafka](kafka) | producer + consumer (sync/async, group, partition; sarama/franz-go unified) | IBM/sarama |
| [postgres](postgres) | pgx pool wrapper | jackc/pgx/v5 |
| [redis](redis) | Redis client wrapper | redis/go-redis |
| [redislock](redislock) | distributed lock (token-guarded Lua, auto-renew, onLost) | redis/go-redis |
| [rate](rate) | Redis-backed GCRA rate limiter | redis/go-redis |
| [grpcclient](grpcclient) | gRPC client middleware (retry, breaker, metrics) | grpc, protobuf |
| [grpcserver](grpcserver) | gRPC server (interceptors, graceful shutdown) | grpc, protobuf |
| [email](email) | SMTP via go-mail (TLS Mandatory by default) | wneessen/go-mail |
| [metrics](metrics) | Prometheus wrapper | prometheus/client_golang |
| [tracing](tracing) | OpenTelemetry wrapper | go.opentelemetry.io/otel |

Importing `github.com/v8fg/kit4go/log4go` does **not** pull pgx or grpc into your
module graph — each sub-module owns only its own dependencies. Local development
uses a committed `go.work` so `go build`/`go test` resolve all modules together.

## Install

```sh
go get github.com/v8fg/kit4go                     # root utilities (50+ packages)
go get github.com/v8fg/kit4go/log4go              # structured logging (standalone)
go get github.com/v8fg/kit4go/kafka               # kafka producer/consumer (standalone)
go get github.com/v8fg/kit4go/postgres            # pgx pool (standalone)
go get github.com/v8fg/kit4go/redislock           # distributed lock (standalone)
```

## Quality

- **Deep concurrency audit**: 6 rounds, ~23 real bugs fixed (deadlocks, races, leaks, panics). See [QUALITY_RULES.md](QUALITY_RULES.md) for the framework.
- **log4go resilience**: circuit breaker + spill failover, observable degradation, bounded shutdown. See [log4go/RESILIENCE.md](log4go/RESILIENCE.md).
- **Callback-recover policy**: library-owned workers recover panics (`Recovered()` + `SetOnPanic`).
- **CI**: all 11 sub-modules, ubuntu + macOS, `-race`, `-short`.
- **Lint**: golangci-lint v2 with 11 high-signal linters.
- **Coverage**: 90%+ across root-module packages.

## Notes

> If test failed, maybe effected by the inline, you can try: `go test -v -gcflags=all=-l xxx_test.go`.

> Error-path tests use injectable interfaces (e.g. `file.FS`, `otp.RandomReader`,
> `ip.AddrLookup`, `random.CryptoSource`) with [mockery](https://github.com/vektra/mockery)
> mocks instead of runtime monkey-patching. Regenerate mocks with
> `go generate ./...` (mockery v2) after editing those interfaces. Each interface
> has a `//go:generate mockery ...` directive and a committed `mock_*.go`.

## CMD

- **release check**: `make`
- **coverage**: `make cover`
- **format check**: `make fmt-check`
- **format fixed**: `make fmt`
- **misspell check**: `make misspell-check`
- **golang lint**: `make golangci`
- **escape analysis**: `make escape` or `ESCAPE_PATH=ip make escape`
