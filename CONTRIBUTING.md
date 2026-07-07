# Contributing to kit4go

kit4go is a pure-Go utility library for ad-tech, finance, and blockchain
infrastructure. It is deliberately split into a **zero-external-dependency root
module** and a set of **sub-modules that each isolate their own heavy deps**.
This document is the short path from "I want to add something" to "it shipped on
`release`".

Read these alongside this file:

- [QUALITY_RULES.md](QUALITY_RULES.md) — the authoritative, language-neutral
  quality framework (dimensions A–L, severities P0–P3). Every rule below cites
  back to it.
- [README.md](README.md) — package list, architecture diagram, install lines.

## 1. Scope: what belongs here

kit4go wraps **infrastructure components** (clients, servers, loggers, rate
limiters, circuit breakers, distributed locks) and provides **generic
primitives** (concurrency, algorithms, finance math, utilities).

It does **not** host domain types. A type that only makes sense inside one
product (a bid request, an order book, a billing ledger entry) does not belong
here, even if it feels reusable. The test: *could a second, unrelated consumer
import this today without inheriting a business model?* If no, keep it in the
product repo.

- YES: `breaker`, `limiter`, `money` (ISO-4217 fixed-point), `consistenthash`,
  `workerpool`, `log4go`, `redislock`.
- NO: an ad-tech `BidRequest`, a trading `Order`, a CRM `Contact`.

When in doubt, open an issue before writing code. (QUALITY_RULES A1 Single
Responsibility, A7 No Over-Engineering.)

## 2. Where it goes: root module vs sub-module

The repo is a **multi-module workspace** (`go.work` lists the root plus 13
sub-modules). The deciding question is *does this need a third-party dependency
the root does not already have?*

### Root module — pure standard library

`github.com/v8fg/kit4go` (root `go.mod`) must stay **zero-external-dep** after
your change (QUALITY_RULES J1 Root Module Purity). The root already depends only
on a small set of vetted libraries (testify, samber/lo, json variants, otp,
uuid libs); do not add a new one to satisfy a root-module package. If you need
`pgx`, `go-redis`, `sarama`, `grpc`, `otel`, `prometheus`, `gopsutil`, etc.,
that work goes in a **sub-module**.

To add a root-module package:

1. Create `<pkgname>/` at the repo root. The directory name **is** the package
   name — all-lowercase, singular, no `util`/`common`/`helpers`
   (A2 Module Layout, C7 Package Name).
2. Add `<pkgname>.go` (or split files by concern). `package <pkgname>` matches
   the dir.
3. Add `doc.go` with a package godoc: one-line summary, a `# Quick start`
   runnable snippet, and (for hot-path packages) a `# Performance` block with
   benchmark numbers. See `breaker/doc.go` as the template.
4. Add `README.md` under the package dir: what / how / API table / example /
   testing notes (H5 README).
5. Add tests (see §5) and an `example_test.go` with `Example*` functions (H6
   Runnable Examples). Add a `bench_test.go` if any function is a hot path (D8
   Benchmark Exists).
6. Do **not** touch the root `go.mod`. Run `go build ./...` and `go vet ./...`
   from the repo root — they resolve through `go.work`.

### Sub-module — heavy deps isolated

If the component pulls in a heavy or opinionated dependency, give it its **own**
`go.mod` so importing `github.com/v8fg/kit4go/<submodule>` does not drag that
dep into every consumer's module graph (J2 Module Isolation). Existing
sub-modules: `log4go`, `kafka`, `postgres`, `clickhouse`, `redis`, `redislock`,
`rate`, `grpcclient`, `grpcserver`, `email`, `metrics`, `tracing`, `adaptive`.

To add a sub-module:

1. Create `<submodule>/` at the repo root with its own `go.mod`:
   ```
   module github.com/v8fg/kit4go/<submodule>

   go 1.26.2
   ```
   Go directive must match the root (`go 1.26.2` today — read it from the root
   `go.mod`, do not guess).
2. List the heavy deps and their transitive deps **only** in this sub-module's
   `go.mod`. Run `go mod tidy` inside the sub-module dir.
3. Add the sub-module to **three** places that enumerate them (search the repo
   for the existing list `adaptive clickhouse email ...` and append):
   - `go.work` `use (...)` block.
   - `Makefile` `SUBMODULES :=`.
   - `.github/workflows/pr.yml` — the build, vet, test, and lint loops all
     iterate the same list. CI will not build or lint your sub-module if you
     miss this.
4. Add a `.golangci.yml` is **not** needed per sub-module: the root v2 config is
   auto-discovered from each sub-module dir. Just run `golangci-lint run` from
   inside the sub-module to confirm.
5. Same `doc.go` / `README.md` / tests / examples as a root package.

If your sub-module has a build-tag-selected alternate implementation (like
`kafka`'s franz-go backend under `-tags franzgo`), also wire it into the
`kafka franz-go backend` step of `pr.yml`.

## 3. The options-based config pattern

Construction is configurable, but the **zero-config default is
production-usable** (QUALITY_RULES A6). Two equivalent forms live in the kit;
pick whichever reads best for the package and stay consistent within it.

### Form A — functional options (variadic `opts ...Option`)

The dominant style for generic primitives and most components. Constructor
takes `opts ...Option`, applies defaults first, then the caller's overrides.

```go
// options.go
type Option[T any] func(*config[T])

type config[T any] struct {
    capacity int
    onEvent  func(Event)
}

func WithCapacity[T any](n int) Option[T] {
    return func(c *config[T]) { c.capacity = n }
}

// backoff.go / lru.go / pipeline.go / loadbalance.go / rate.go all use this.
func New[K comparable, V any](opts ...Option[K, V]) *Cache[K, V] {
    c := config[K, V]{capacity: defaultCapacity} // sensible default
    for _, o := range opts { o(&c) }
    // ...build with c
}
```

Reference implementations: `backoff`, `lru`, `pipeline`, `loadbalance`, `rate`,
`clickhouse`, `tracing`, `httpclient`, `signing`.

### Form B — exported options struct

Used when the config surface is large and benefits from named fields and
struct tags for JSON/mapstructure loading. The struct's zero value must be
usable (defaults filled by a `withDefaults` step at construction).

```go
type BreakerOptions struct {
    Name         string        `json:"name"          mapstructure:"name"`
    MaxRequests  uint32        `json:"max_requests"  mapstructure:"max_requests"`
    Interval     time.Duration `json:"interval"      mapstructure:"interval"`
    OpenDuration time.Duration `json:"open_duration" mapstructure:"open_duration"`
    FailRate     float64       `json:"fail_rate"      mapstructure:"fail_rate"`
    // ...
}

func NewBreaker[T any](opts BreakerOptions) *Breaker[T] {
    opts = opts.withDefaults() // zero value yields a working breaker
    // ...
}
```

Reference: `breaker/options.go`.

### Conventions either form shares

- **Zero config works.** `New()` / `NewBreaker[T](BreakerOptions{})` with no
  fields set must produce a correctly-behaving instance (H1 Zero-Value Useful).
- **Validate at construction**, not per-call. Clamp/validate inputs in the
  constructor (or `withDefaults`); return errors from `New` when construction
  can fail (e.g. `clickhouse.New`, `tracing.New`); panic only from an explicit
  `Must*` variant (B5, B6 — no `os.Exit`/`log.Fatal` in library code).
- **No positional constructors with many params.** If you're adding a 4th
  positional arg, switch to options (A6).
- **Accept abstractions, return concretes.** `func New(s Store) *Cache`, not
  `func New(s *Store) *Store` (A4).
- **Assert API contracts at compile time** for exported types implementing an
  interface: `var _ I = (*T)(nil)` (A5).

## 4. Concurrency, resources, and the hot path

These come up on almost every contribution, so they're stated up front (full
text in QUALITY_RULES D, F, L):

- **`context.Context` is the first parameter** of any blocking operation, never
  stored in a struct (H4).
- **Every background goroutine has a shutdown path** — `ctx.Done`, channel
  close, or `wg.Wait`. No fire-and-forget. Packages that spawn goroutines use
  `goleak` in tests (D7 Async-Unit Hygiene).
- **Library-owned workers recover panics** and expose `Recovered()` /
  `SetOnPanic` so a panic inside library code never crashes the host. The
  synchronous caller stays raw (no blanket recover).
- **Hot path = zero allocation.** `Get`/`Set`/`Allow`/`Execute`/`Observe` type
  operations must report `0 allocs/op` in `go test -bench -benchmem` (D1). This
  is what the `bench_test.go` is for.
- **Everything bounded.** Every buffer, queue, cache, and pool has an explicit
  cap (D5, I3, L3).
- **Locks are zero-value named fields** (`mu sync.Mutex`), never
  `new(sync.Mutex)`, never anonymous embed (C6, F5).

## 5. Tests

### Coverage: ≥95% for new packages

The README states the project bar as "90%+ across root-module packages"; the
CONTRIBUTING target for **new** code is **≥95%** (statement coverage,
`go test -cover`). Existing packages sit well above this; do not regress a
package below 90%. Defensive/unreachable branches below the bar need a one-line
`// unreachanble: ...` comment explaining why (QUALITY_RULES E1, E6).

### White-box for internals, black-box for the API

Both live in the same package directory, distinguished by the package clause:

- **Internal mechanics** — state transitions, private fields, race-prone
  invariants, unexported helpers — are tested **white-box** in `package <pkg>`
  (e.g. `lru/lru_test.go`, `ringbuffer/ringbuffer_test.go`,
  `decimal/decimal_coverage_test.go`). Use this when you must read unexported
  state or call unexported funcs to assert an invariant.
- **Public API** — the surface a consumer sees — is tested **black-box** in
  `package <pkg>_test` (e.g. `breaker/breaker_test.go`,
  `redislock/lock_test.go`). This is the form QUALITY_RULES E7 calls for; it
  proves the package is usable through its exported API alone and catches
  accidental reliance on internals. Prefer black-box; reach for white-box only
  when there is no other way to assert the invariant.

### Mock injection, not live deps

Tests never spin up real PostgreSQL / Redis / Kafka / SMTP brokers and never
rely on `time.Sleep` to gate an assertion (E5 No Flaky Tests). Instead:

- **Injectable interfaces** with committed mockery mocks. The kit defines small
  seam interfaces and generates mocks:
  - `file.FS` (filesystem) — `mock_FS.go`
  - `otp.RandomReader` — `mock_RandomReader.go`
  - `random.CryptoSource` — `mock_CryptoSource.go`
  - `ip.AddrLookup` — `mock_AddrLookup.go`

  Each interface carries a `//go:generate mockery --name <I> --inpackage
  --with-expecter --filename mock_<I>.go` directive. **Regenerate mocks with
  `go generate ./...` (mockery v2) after editing those interfaces.** No runtime
  monkey-patching.
- **In-process fakes**: `miniredis` for Redis (`redislock`, `rate`),
  `nettest`/`httptest`/`bufconn` for servers, an injected clock (`func()
  time.Time`) for time-windowed logic (`breaker` substitutes a fake clock so
  state transitions are deterministic).
- **Live integration tests self-skip** under `-short` or when the broker env
  var is unset (see `postgres` reading `PG_HOST`). They run in CI only when
  wired; the default `go test -short -race` gate does not require them.

### Other test rules

- **Table-driven** for any multi-input logic: named rows, `t.Run`, no complex
  branching inside (E3).
- **`-race` is mandatory.** `go test -race` must be clean on every run (E2).
  CI runs `-race` across linux + macOS.
- **Edge cases** each get ≥1 test: nil/empty, zero/negative, max, concurrency,
  resource exhaustion / full buffer (E6).
- **`t.Helper()`** in test helpers so failures point at the call site (E4).
- If a test looks flaky due to inlining, try `go test -v -gcflags=all=-l`
  (noted in the README).

## 6. Lint and format

### golangci-lint v2 (required)

The repo uses **golangci-lint v2** — the v1 binary (v1.49/1.48) panicked on
modern Go and is gone. Install v2 from the official script:

```sh
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
  | sh -s -- -b "$(go env GOPATH)/bin" v2.11.4
```

Config lives at the repo root (`.golangci.yml`, `version: "2"`) and is
auto-discovered from each sub-module dir — no per-module config needed. The
enabled set is deliberately **high-signal, low-noise** (QUALITY_RULES K0):
`errcheck`, `govet`, `ineffassign`, `misspell`, `staticcheck`, `unused`, plus
`errorlint`, `nilerr`, `nilnesserr`, `wastedassign`, `reassign`. Formatters:
`gofmt`, `goimports` (with `local-prefixes: github.com/v8fg/kit4go`).

Opinionated style families are **off by design** — staticcheck `ST1*`/`QF1*`/
`S1*` are suppressed because the codebase has its own consistent style, and the
`SA*` correctness checks carry the value. Do not re-enable them to "clean up"
without measuring repo-wide first (K0).

Run lint everywhere (root + every sub-module, plus the kafka franz-go backend):

```sh
make golangci
# equivalent to:
#   golangci-lint run --timeout=5m ./...
#   for m in <submodules>; do (cd $m && golangci-lint run --timeout=5m ./...); done
#   (cd kafka && golangci-lint run --build-tags franzgo --timeout=5m ./...)
```

### Format and spell

```sh
make fmt          # gofmt -s -w
make fmt-check    # CI gate: fails if any file is unformatted
make misspell-check
```

`make check` runs fmt-check + misspell-check + golangci + cover together — run
it before pushing.

## 7. The per-package squash-to-release workflow

`release` is the **primary branch** (`origin/HEAD -> release`); `main` mirrors
it. PRs target `release`. The development-to-release model is **per-package
squash with a build gate on every push**:

1. **Branch per package/theme** off `release`. One package (or one coherent
   theme) per branch — do not mix unrelated packages in one branch. Name it
   after the package or theme.
2. **Open a PR targeting `release`.** CI (`.github/workflows/pr.yml`) runs on
   every push and is the build gate: it builds, vets, gofmt-checks, tests
   (`-short -race`, linux + macOS), and lints **every module** plus the kafka
   franz-go backend. A red gate blocks merge.
3. **Squash-merge per package.** The branch's commits collapse to one commit on
   `release`, one per package/theme. This keeps `release` history readable as a
   sequence of package-level changes. Commit messages stay in English.
4. **`main` mirrors `release`.** Do not commit to `main` directly; it tracks
   `release`.
5. **Releases are thematic.** Version tags (`v*`) cut from `release`; the
   squash-per-package history makes the release notes a clean list of package
   changes.

Local pre-push gate, mirroring CI:

```sh
make check        # fmt-check + misspell-check + golangci + cover
# or step by step:
go build ./... && for m in adaptive clickhouse email grpcclient grpcserver kafka log4go metrics postgres rate redis redislock tracing; do (cd $m && go build ./...); done
go test -short -race -count=1 ./...
make golangci
```

## 8. Quick checklist before you open a PR

- [ ] Right home: root module (pure stdlib, no new `go.mod` dep) or sub-module
      (own `go.mod`, added to `go.work` + `Makefile` + `pr.yml`).
- [ ] In scope: wraps a component or is a generic primitive — no domain types.
- [ ] `doc.go` + package `README.md` + `example_test.go`; `bench_test.go` for
      any hot path.
- [ ] Options-based construction; zero-config default works; no positional
      constructor bloat.
- [ ] Tests: white-box for internals, black-box (`package <pkg>_test`) for the
      API; mocks via injectable interfaces (`go generate ./...`), no live deps,
      no `time.Sleep` assertions.
- [ ] Coverage ≥95% for new code; `go test -race` clean.
- [ ] `make check` green (gofmt, misspell, golangci-lint v2, cover).
- [ ] One package/theme per PR, targeting `release`.

## 9. Adding a new language (future)

kit4go is Go today. QUALITY_RULES is written principle-first so the same
framework governs future kits (Rust, JVM, TypeScript). When a second language
lands, follow the "When the kit adds a second language" steps at the end of
QUALITY_RULES — mirror each principle under that language's tooling via the
cross-language mechanism map, do **not** fork a separate rulebook.
