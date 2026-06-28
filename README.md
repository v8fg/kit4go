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

> The common tools for go.

## Support list

- [x] [bit](bit) hacks for bit.
- [x] [datetime](datetime) parse, format, others.
- [x] [file](file) base file ops.
- [x] [ip](ip) parse, match, convert, info.
- [x] [json](json) support multi json packages.
- [x] [log4go](log4go) async structured logging: console/file/kafka/net/io writers, structured fields (With/WithField/WithFields), JSON format (FormatJSON), sampling, context binding (zerolog-style), request-id middleware, generic overflow (ring→file→drop), crash resume, metrics + webhook alerts, multi-core ShardLogger, switchable JSON codec (goccy/std/sonic), ~1M qps/core (no-caller). See [log4go/PERFORMANCE.md](log4go/PERFORMANCE.md).
- [x] [number](number) round, bytes convert.
- [x] [otp](otp) `TOTP`, `HOTP`.
- [x] [postgres](postgres) pgx pool wrapper (pure Go, cross-platform).
- [x] [random](random) rand, random.
- [x] [str](str) common string utils.
- [x] [uuid](uuid) requestID, go.uuid, ksuid, xid.
- [x] [xlo](xlo) some utils ref *lo*, more pls use [lo](https://github.com/samber/lo) directly.

## Modules

kit4go is a **multi-module** repository, so you only pull the dependencies you actually use:

| Module path | Packages | Heavy deps isolated |
|-------------|----------|---------------------|
| `github.com/v8fg/kit4go` (root) | bit, datetime, file, ip, json, number, otp, random, str, uuid, xlo, maxprocs, breaker, limiter, latency, httpclient, tcpclient, udpclient, stress | — |
| `github.com/v8fg/kit4go/log4go` | log4go — structured logger | sarama, sonic, goccy/go-json |
| `github.com/v8fg/kit4go/postgres` | postgres — pgx pool | jackc/pgx/v5 |
| `github.com/v8fg/kit4go/grpcclient` | grpcclient — gRPC client | grpc, protobuf |
| `github.com/v8fg/kit4go/kafka` | kafka — producer + consumer (sync/async, group, partition) | IBM/sarama, protobuf |

Importing `github.com/v8fg/kit4go/log4go` does **not** pull pgx or grpc into your
module graph — each sub-module owns only its own dependencies. Local development
uses a committed `go.work` so `go build`/`go test` resolve all modules together.

## Install

```sh
go get -u github.com/v8fg/kit4go            # root utilities
go get -u github.com/v8fg/kit4go/log4go     # structured logging (standalone)
go get -u github.com/v8fg/kit4go/postgres   # pgx pool (standalone)
go get -u github.com/v8fg/kit4go/grpcclient # gRPC client (standalone)
```

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
