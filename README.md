# Kit4go

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/v8fg/kit4go?color=red)
![Top Languages](https://img.shields.io/github/languages/top/v8fg/kit4go)
[![Go Report Card](https://goreportcard.com/badge/github.com/v8fg/kit4go)](https://goreportcard.com/report/github.com/v8fg/kit4go)
[![License](https://img.shields.io/:license-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/release/v8fg/kit4go.svg?style=flat-square)](https://github.com/v8fg/kit4go/releases)
[![Build Status](https://img.shields.io/github/workflow/status/v8fg/kit4go/CI/release?label=CI)](https://github.com/v8fg/kit4go/actions?query=branch%3Arelease)
[![Last Commit](https://img.shields.io/github/last-commit/v8fg/kit4go?label=last%20commit)](https://github.com/v8fg/kit4go)
[![Workflow for CI Action](https://img.shields.io/github/workflow/status/v8fg/kit4go/CI?label=tests)](https://github.com/v8fg/kit4go/actions/workflows/pr.yml?query=event%3Apush)
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
- [x] [number](number) round, bytes convert.
- [x] [otp](otp) `TOTP`, `HOTP`.
- [x] [random](random) rand, random.
- [x] [str](str) common string utils.
- [x] [uuid](uuid) requestID, go.uuid, ksuid, xid.
- [x] [xlo](xlo) some utils ref *lo*, more pls use [lo](https://github.com/samber/lo) directly.

## Install

`go get -u github.com/v8fg/kit4go`

## Notes

> If test failed, maybe effected by the inline, you can try: `go test -v -gcflags=all=-l xxx_test.go`.

## CMD

- **release check**: `make`
- **coverage**: `make cover`
- **format check**: `make fmt-check`
- **format fixed**: `make fmt`
- **misspell check**: `make misspell-check`
- **golang lint**: `make golangci`
- **escape analysis**: `make escape` or `ESCAPE_PATH=ip make escape`
