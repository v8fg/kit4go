# kit4go Package Quality Rules

> Industry-calibrated checklist for every kit4go package. Each rule cites a
> source (Uber/Google/K8s/OTel/golangci-lint) and has a check method,
> threshold, severity, and acceptance level (Universal / Strong-consensus /
> Opinionated). Designed for multi-agent parallel review: each role
> (Architect, SRE, QA, Developer, Security) runs independently.

---

## A. Architecture & Project Layout (Architect)

### A1. Single Responsibility
- **Check**: package godoc states one clear purpose; no catch-all names
  (`util`, `common`, `helpers`, `shared`, `types`).
- **Sources**: Google Decisions, K8s, Effective Go — **Universal**.
- **Pass**: one-sentence purpose fitting ad-tech/finance scope.
- **Fail**: mixes concerns; named `util`/`common`.
- **Severity**: P0.

### A2. Package Layout
- **Check**: `package foo` matches directory name; one package per dir; `internal/`
  for non-public; `cmd/` for entry points.
- **Sources**: go.dev modules-layout, K8s — **Universal**.
- **Pass**: package name == dir name; no `util/` or `misc/`.
- **Severity**: P0.

### A3. Interface Segregation
- **Check**: exported interfaces ≤3 methods; single-method named with `-er`
  suffix (`Reader`, `Formatter`); no "fat interface".
- **Sources**: Go Code Review Comments, Google — **Universal**.
- **Pass**: small interfaces; caller-side definition where possible.
- **Fail**: >5 methods, or forces callers to depend on unused methods.
- **Severity**: P1.

### A4. Accept Interfaces, Return Concretes
- **Check**: exported functions accept interface params, return concrete types.
- **Sources**: Go Tip #49, Google — **Universal**.
- **Pass**: `func New(s Store) *Cache` not `func New(s *Store) *Store`.
- **Severity**: P1.

### A5. Compile-Time Interface Verification
- **Check**: `var _ I = (*T)(nil)` for exported types implementing API contracts.
- **Sources**: Uber, Effective Go — **Universal**.
- **Pass**: every exported type that should implement an interface has the
  compile-time assertion.
- **Severity**: P1.

### A6. Functional Options
- **Check**: constructor uses `opts ...Option`; zero-config works with defaults.
- **Sources**: Uber — **Strong consensus** (widely adopted).
- **Pass**: `New(opts ...Option)`; no positional constructor with >3 params.
- **Severity**: P1.

### A7. No Over-Engineering
- **Check**: every exported type/function is used or demonstrably useful; no
  speculative generics, no "future-proof" interfaces.
- **Sources**: K8s ("avoid package sprawl"), Google — **Universal**.
- **Severity**: P1.

### A8. No init() Side Effects
- **Check**: no `init()` doing I/O, env access, flag registration, or global
  mutation. Libraries must be configured via Go APIs, not CLI flags.
- **Sources**: Uber, Google — **Universal**.
- **Pass**: no `init()` at all, or only for compile-time constants.
- **Severity**: P0.

---

## B. Error Handling (Architect + Security)

### B1. Error is Last Return, Always
- **Check**: functions taking `context.Context` return `error`; error is last.
- **Sources**: Google Decisions, Effective Go — **Universal**.
- **Severity**: P0.

### B2. Sentinel Errors with Err Prefix
- **Check**: exported errors use `Err` prefix (`ErrTimeout`, `ErrInvalidInput`);
  wrap with `fmt.Errorf("context: %w", err)`.
- **Sources**: Uber, Google — **Universal**.
- **Pass**: `errors.Is`/`errors.As` works; wrap chain preserved.
- **Fail**: bare `errors.New` without wrapping; string-matching errors.
- **Severity**: P0.

### B3. No In-Band Errors
- **Check**: no returning `-1`/`nil`/`""` to signal failure. Use `(value, ok bool)`
  or `(value, error)`.
- **Sources**: Google Decisions — **Universal**.
- **Severity**: P0.

### B4. Handle Errors Once
- **Check**: don't log + return the same error. Either handle-and-degrade, or
  wrap-and-return.
- **Sources**: Dave Cheney, Uber — **Universal**.
- **Severity**: P1.

### B5. No panic for Normal Errors
- **Check**: `panic` only in `Must*` helpers or truly unrecoverable conditions.
- **Sources**: Uber, Google — **Universal**.
- **Pass**: bad input returns error, not panic (unless `Must*` constructor).
- **Severity**: P0.

### B6. No os.Exit / log.Fatal in Library
- **Check**: `os.Exit`/`log.Fatal` only in `main()`.
- **Sources**: Uber, Google — **Universal**.
- **Severity**: P0.

### B7. No err.Error() String Inspection
- **Check**: never match on `err.Error()` string content.
- **Sources**: Dave Cheney, Google — **Strong consensus**.
- **Pass**: `errors.Is`/`errors.As` for all error matching.
- **Severity**: P1.

---

## C. Naming Conventions (Developer)

### C1. MixedCaps, No Underscores
- **Check**: PascalCase exported, camelCase unexported; no `_` in identifiers
  except `_test.go` function names and `_test` package suffix.
- **Sources**: Google, Uber — **Universal**.
- **Severity**: P0.

### C2. Initialisms Keep Case
- **Check**: `URL`/`ID`/`HTTP`/`API`/`DB`, never `Url`/`Id`/`Http`.
- **Sources**: Google Decisions — **Universal**.
- **Severity**: P0.

### C3. Receiver Names
- **Check**: 1-2 letter abbreviation, consistent across all methods; never
  `this`/`self`/`_`.
- **Sources**: Google Decisions, Uber — **Universal**.
- **Pass**: `func (c *Cache)` consistently.
- **Severity**: P1.

### C4. No Get Prefix
- **Check**: no `GetX()` on getters; use noun directly (`Counts()`, not
  `GetCounts()`).
- **Sources**: Google Decisions — **Universal**.
- **Severity**: P1.

### C5. Constants — No K Prefix, No ALL_CAPS
- **Check**: MixedCaps constants; no `MAX_SIZE`/`kDefaultPort`.
- **Sources**: Google Decisions — **Universal**.
- **Severity**: P1.

### C6. Mutex Named mu/lock, Never Embedded
- **Check**: `mu sync.Mutex` as a named field; never anonymous embed; multiple
  locks get suffix (`stateMu`, `mapMu`).
- **Sources**: K8s, Uber — **Strong consensus**.
- **Severity**: P1.

### C7. Package Name — All Lowercase, Not Plural
- **Check**: `net/url` not `net/urls`; `cache` not `caches`; matches dir name.
- **Sources**: Google, Uber — **Universal**.
- **Severity**: P0.

---

## D. Performance & Resources (SRE)

### D1. Hot Path = 0 Allocations
- **Check**: `go test -bench -benchmem`; hot path (Get/Set/Allow/Push) must be
  0 allocs/op.
- **Sources**: fasthttp (valyala), bigcache — **Strong consensus** for perf libs.
- **Pass**: 0 allocs/op.
- **Acceptable**: ≤2 allocs/op if documented and justified.
- **Fail**: >2 allocs/op without justification.
- **Severity**: P0.

### D2. strconv Over fmt
- **Check**: use `strconv.Itoa`/`strconv.ParseInt`, not `fmt.Sprint`/`fmt.Sprintf`
  for primitive↔string conversion.
- **Sources**: Uber (benchmarked: 64ns/1alloc vs 143ns/2allocs) — **Universal**.
- **Severity**: P1.

### D3. Pre-size Slices and Maps
- **Check**: `make([]T, 0, capacity)` / `make(map[K]V, hint)` when size is known
  or estimable.
- **Sources**: Uber — **Universal**.
- **Severity**: P1.

### D4. Hoist Constant Conversions
- **Check**: `[]byte("constant")` moved out of loops.
- **Sources**: Uber (benchmarked: 3.25ns vs 22.2ns) — **Universal**.
- **Severity**: P1.

### D5. Memory Bounded
- **Check**: maps/slices/buffers have explicit caps; no unbounded growth.
- **Sources**: kit4go convention (OOM prevention) — **Universal**.
- **Pass**: `MaxSize`/`MaxKeys`/capacity parameter.
- **Severity**: P0.

### D6. Lock Granularity
- **Check**: RLock for reads, Lock for writes; no I/O/alloc under lock; CAS for
  single-variable hot paths.
- **Sources**: Uber, Go concurrency best practices — **Universal**.
- **Severity**: P1.

### D7. Goroutine Hygiene
- **Check**: every goroutine has a shutdown path (ctx.Done, channel close,
  wg.Wait); no fire-and-forget; `goleak` test in packages that spawn goroutines.
- **Sources**: Uber, Google ("goroutine lifetimes") — **Universal**.
- **Severity**: P0.

### D8. Benchmark Exists
- **Check**: `bench_test.go` with `b.ReportAllocs()` for hot-path operations.
- **Sources**: Go bench docs, TwiN — **Universal**.
- **Pass**: ≥1 Benchmark per hot-path function.
- **Severity**: P1 for perf-critical; P3 for cold path.

### D9. Prefer Synchronous Functions
- **Check**: package provides synchronous API; caller adds concurrency. No
  forced background goroutines.
- **Sources**: Google Decisions — **Strong consensus**.
- **Severity**: P1.

---

## E. Testing & Coverage (QA)

### E1. Coverage Threshold
- **Check**: `go test -cover`.
- **Pass**: ≥90% (team policy — no industry standard minimum exists per
  Google/Uber/K8s).
- **Acceptable**: 80-90% if uncovered code is defensive/unreachable.
- **Fail**: <80%.
- **Severity**: P0.

### E2. Race Detection
- **Check**: `go test -race`.
- **Sources**: Go race detector, Uber — **Universal**.
- **Pass**: clean.
- **Severity**: P0.

### E3. Table-Driven Tests
- **Check**: multi-input functions use table-driven with `t.Run`; named rows;
  no complex branching.
- **Sources**: Uber, Google, K8s — **Universal**.
- **Severity**: P1.

### E4. t.Helper() in Test Helpers
- **Check**: test helpers call `t.Helper()` after context param.
- **Sources**: Google Decisions — **Universal**.
- **Severity**: P2.

### E5. No Flaky Tests
- **Check**: no `time.Sleep` for correctness; use injected clocks / mocks /
  in-process servers (miniredis, bufconn, httptest).
- **Sources**: K8s ("wait-and-retry, not sleep-one-second") — **Strong consensus**.
- **Severity**: P0.

### E6. Edge Cases
- **Check**: nil input, empty input, zero/negative, max values, concurrent,
  resource exhaustion (full buffer).
- **Pass**: ≥1 test per category.
- **Severity**: P0 for nil/concurrent; P1 for others.

### E7. _test Package for Black-Box
- **Check**: external tests use `package foo_test` for API surface testing.
- **Sources**: Google, K8s — **Universal**.
- **Severity**: P2.

### E8. Lint Clean
- **Check**: `golangci-lint run` + `go vet` = 0 issues.
- **Sources**: Uber baseline (errcheck, goimports, revive, govet, staticcheck) —
  **Universal**.
- **Severity**: P0.

### E9. Cross-Platform
- **Check**: tests pass on macOS + Linux; platform-specific uses build tags or
  `t.Skip`.
- **Sources**: K8s — **Strong consensus**.
- **Severity**: P1.

---

## F. Concurrency Safety (QA)

### F1. Thread Safety Documented
- **Check**: godoc states "safe for concurrent use" or "not safe; use shard +
  Merge".
- **Sources**: kit4go convention — **Strong consensus**.
- **Severity**: P1.

### F2. No Race Between Close and Use
- **Check**: Close uses CAS/once/mutex; Use checks under same lock or atomic;
  no send-on-closed-channel.
- **Sources**: Uber ("data race patterns") — **Universal**.
- **Severity**: P0.

### F3. No Mutable Globals
- **Check**: no package-level `var` that mutates at runtime; use dependency
  injection.
- **Sources**: Uber — **Strong consensus**.
- **Severity**: P1.

### F4. Channel Sizes Justified
- **Check**: channel cap is 0 (unbuffered) or 1 by default; larger requires
  documented justification (bounded, backpressure strategy).
- **Sources**: Uber — **Strong consensus**.
- **Severity**: P1.

### F5. Zero-Value Mutex
- **Check**: `sync.Mutex`/`RWMutex` as a zero-value field; never `new(sync.Mutex)`.
- **Sources**: Uber — **Universal**.
- **Severity**: P1.

---

## G. Observability (SRE)

### G1. Library Uses OTel API Only, Never SDK
- **Check**: instrumentation imports `go.opentelemetry.io/otel` (API), not
  `.../otel/sdk`.
- **Sources**: OpenTelemetry library guide — **Universal**.
- **Pass**: API is no-op without SDK → zero cost when unconfigured.
- **Severity**: P0 for instrumented packages.

### G2. No Direct Logging in Library
- **Check**: no `log.Printf`/`slog.Info` in library code; use callbacks/interfaces
  (`SetOnEvent`, `OnEvent`).
- **Sources**: OTel guide, golangci-lint (`sloglint: no-global`) — **Strong consensus**.
- **Severity**: P0.

### G3. Metrics/Snapshot Exposure
- **Check**: `Metrics()`/`Snapshot()` returning atomic counters; or documented
  "no metrics by design".
- **Sources**: kit4go convention — **P1** for infra; P3 for algorithms.

### G4. Tracer Naming
- **Check**: tracer name = library name + version (`"kit4go/kafka"`).
- **Sources**: OTel guide — **Universal** for instrumented packages.
- **Severity**: P1.

### G5. Span Lifecycle
- **Check**: `span.End()` always in `defer`; `RecordException` + `SetStatus(ERROR)`
  on error paths.
- **Sources**: OTel guide — **Universal**.
- **Severity**: P0 for instrumented packages.

### G6. Check span.IsRecording()
- **Check**: skip expensive attribute computation when span not recording.
- **Sources**: OTel guide — **Strong consensus**.
- **Severity**: P1 for instrumented packages.

---

## H. API & Developer Experience (Developer)

### H1. Zero-Value is Useful
- **Check**: types work without `New*()` where possible (`var c Cache`).
- **Sources**: Uber, Go stdlib convention — **Universal**.
- **Pass**: zero-value is a valid, usable state.
- **Severity**: P1.

### H2. No nil vs Empty Slice Distinction
- **Check**: check `len(s) == 0`, never `s == nil`; return `nil`, not `[]T{}`.
- **Sources**: Google Decisions, Uber — **Universal**.
- **Severity**: P1.

### H3. Copy Slices/Maps at Boundaries
- **Check**: when receiving a slice/map and storing it; when returning internal
  state.
- **Sources**: Uber — **Strong consensus**.
- **Severity**: P1.

### H4. context.Context First Param
- **Check**: ctx is first param; never stored in struct fields.
- **Sources**: Google Decisions — **Universal**.
- **Severity**: P0.

### H5. README
- **Check**: README.md with what/how/API table/example/ad-tech use/testing.
- **Pass**: all sections present.
- **Severity**: P0.

### H6. Godoc Examples
- **Check**: `example_test.go` with runnable `Example` functions.
- **Sources**: Effective Go — **Universal**.
- **Severity**: P1.

---

## I. Security & Robustness (Security)

### I1. crypto/rand for Security-Sensitive Randomness
- **Check**: tokens, nonces, IDs use `crypto/rand`, not `math/rand`.
- **Sources**: Google Decisions — **Universal**.
- **Severity**: P0.

### I2. Input Validation
- **Check**: exported functions validate nil/empty/negative/overflow.
- **Sources**: Go community — **Universal**.
- **Severity**: P0.

### I3. Resource Exhaustion Resistance
- **Check**: bounded buffers, max sizes; no unbounded growth from user input.
- **Sources**: Go community — **Universal**.
- **Severity**: P0.

### I4. gosec Linter Enabled
- **Check**: `golangci-lint` config includes `gosec`.
- **Sources**: golangci-lint golden config — **Strong consensus**.
- **Severity**: P1.

### I5. HTTP Body Close + No-ctx Checks
- **Check**: `bodyclose`, `noctx`, `rowserrcheck`, `sqlclosecheck` linters.
- **Sources**: golangci-lint golden config — **Strong consensus**.
- **Severity**: P1 for packages doing I/O.

### I6. No Hardcoded Secrets
- **Check**: no passwords, API keys, connection strings in code.
- **Sources**: Universal — **P0**.

---

## J. Dependency Hygiene (Architect)

### J1. Root Module Purity
- **Check**: root `go.mod` unchanged after adding a root-module package.
- **Sources**: kit4go convention — **P0**.

### J2. Module Isolation
- **Check**: own-module packages have `go.mod`; deps listed there only.
- **Sources**: go.dev modules-layout — **Universal**.
- **Severity**: P0.

### J3. No Side-Effect Imports
- **Check**: no `import _ "..."` in library code (only in main/tests).
- **Sources**: Google Decisions — **Universal**.
- **Severity**: P1.

### J4. No Dot Imports
- **Check**: no `import . "..."` except in `_test` packages.
- **Sources**: Google Decisions, Uber — **Universal**.
- **Severity**: P0.

### J5. Depguard Deny-List
- **Check**: block deprecated deps (`satori/go.uuid`, `golang/protobuf`,
  `math/rand` for security).
- **Sources**: golangci-lint golden config — **Strong consensus**.
- **Severity**: P1.

---

## K. golangci-lint Configuration (All Roles)

### K1. Always-On Linters (Universal Baseline)
`errcheck`, `govet`, `staticcheck`, `unused`, `gosimple`, `ineffassign`,
`revive`, `gocritic`, `goimports`, `gosec`.

### K2. Production-Grade Additional (Strong Consensus)
`bodyclose`, `noctx`, `errorlint`, `errname`, `misspell`, `unconvert`,
`predeclared`, `reassign`, `usestdlibvars`, `exhaustive`, `nilerr`,
`nilnesserr`, `wastedassign`, `unparam`, `spancheck`, `testifylint`,
`sloglint`.

### K3. Tuned Thresholds (from golden config)
| Metric | Threshold |
|---|---|
| `funlen` | 100 lines / 50 statements |
| `golines` max-len | 120 |
| `cyclop` max-complexity | 30 (avg 10) |
| `gocognit` min-complexity | 20 |
| `nakedret` | 0 (ban naked returns) |
| `govet` enable-all | true (except fieldalignment) |
| `staticcheck` | all (except ST1000/ST1016/QF1008) |
| `errcheck.check-type-assertions` | true |
| `sloglint.no-global` | all |
| `nolintlint.require-explanation` | true |

---

## Multi-Agent Review Design

Each role agent receives the package source + its dimension's rules + available
tools. Agents run in parallel:

| Agent | Dimensions | Key Tools |
|---|---|---|
| **Architect** | A, B, J | import analysis, godoc review, go.mod diff |
| **SRE** | D, G | `go test -bench -benchmem`, go vet, code review |
| **QA** | E, F | `go test -race -cover`, test file review |
| **Developer** | C, H | README check, godoc, example review |
| **Security** | I | gosec, input validation audit, resource exhaustion review |

Each agent outputs:
```
{rule, severity, PASS/FAIL, detail, source}
```

Final synthesis: collect, sort by severity (P0 first), produce go/no-go.

### Go/No-Go Decision
- **GO**: 0 P0 failures.
- **GO with conditions**: P0 failures are defensive/unreachable (documented).
- **NO-GO**: any P0 failure on a live code path.
