# kit4go Package Quality Rules

> Checklist for every kit4go package — each rule has a check method, threshold,
> and severity. Designed for multi-agent parallel review: each role (Architect,
> SRE, Developer, QA, Security) runs independently and reports findings.

## Dimensions

| # | Dimension | Role | Key Question |
|---|---|---|---|
| A | Architecture | Architect | Does the design follow SOLID/DRY/KISS? Is the API surface minimal? |
| B | Performance & Resources | SRE | What are the CPU/memory/disk/goroutine costs? Is the hot path zero-alloc? |
| C | Testing & Coverage | QA | Are tests deep enough? ≥90% coverage? Race-clean? Edge cases? |
| D | Concurrency Safety | QA | Is it safe for concurrent use? No data races? No deadlocks? |
| E | Observability | SRE | Can ops see what's happening? Metrics, logs, events, health? |
| F | API & DX | Developer | Is the API ergonomic? Zero-config usable? Well-documented? |
| G | Security & Robustness | Security | Input validation? Error handling? No panics on bad input? |
| H | Dependency Hygiene | Architect | Zero external deps for root module? Isolated deps for modules? |

---

## A. Architecture (Architect)

### A1. Single Responsibility
- **Check**: package godoc — does it state one clear purpose?
- **Pass**: one-sentence purpose that fits the ad-tech/finance scope.
- **Fail**: package mixes concerns (e.g., "rate limiting + email sending").
- **Severity**: P0 (must fix).

### A2. Interface Segregation
- **Check**: count exported interfaces and their method count.
- **Pass**: each interface ≤5 methods; no "fat interface".
- **Fail**: interface with >5 methods or one that forces callers to depend on
  methods they don't use.
- **Severity**: P1.

### A3. Functional Options
- **Check**: does the constructor use `opts ...Option`?
- **Pass**: `New(opts ...Option)` pattern; zero-config works with sensible defaults.
- **Fail**: positional constructor with >3 params; required config not defaulted.
- **Severity**: P1.

### A4. Error Handling
- **Check**: exported error sentinels? Wrapped errors? No panic on bad input?
- **Pass**: `ErrXxx` sentinels for caller-facing errors; `fmt.Errorf("%w", err)`
  wrapping; `recover` only for marshalling edge cases.
- **Fail**: panics on nil/bad input; bare `errors.New` without wrapping.
- **Severity**: P0.

### A5. Dependency Direction
- **Check**: imports — does the package import its siblings circularly? Does a
  root-module package import a heavy external dep?
- **Pass**: root-module packages import only stdlib + sibling root packages; own
  modules import only their dep + root.
- **Fail**: circular import; root package imports go-redis/grpc/etc.
- **Severity**: P0.

### A6. No Over-Engineering
- **Check**: is every exported type/function actually used or useful?
- **Pass**: no speculative generics, no "future-proofing" interfaces, no unused
  options.
- **Fail**: exported "just in case" API surface that nobody calls.
- **Severity**: P1.

---

## B. Performance & Resources (SRE)

### B1. Hot Path Allocations
- **Check**: `go test -bench -benchmem` on the core operation.
- **Pass**: 0 allocs/op on the hot path (Get/Set/Allow/Push/etc.).
- **Acceptable**: ≤2 allocs/op if documented and justified.
- **Fail**: >2 allocs/op on the hot path without justification.
- **Severity**: P0 for hot path; P2 for cold path.

### B2. Memory Bound
- **Check**: does the package use bounded data structures?
- **Pass**: maps/slices/buffers have explicit caps (MaxSize, MaxKeys, capacity);
  no unbounded growth on the hot path.
- **Fail**: `make(map[...])` without eviction; unbounded slice append on hot path.
- **Severity**: P0.

### B3. Lock Granularity
- **Check**: mutex usage — RWMutex vs Mutex; lock hold time.
- **Pass**: RLock for reads; Lock only for writes; no I/O/allocation under lock;
  CAS for single-variable hot paths (atomics > mutex).
- **Fail**: Mutex where RWMutex suffices; channel send under lock; expensive
  computation while holding lock.
- **Severity**: P1.

### B4. Goroutine Hygiene
- **Check**: does the package spawn goroutines? Are they bounded + joinable?
- **Pass**: every spawned goroutine has a shutdown path (ctx.Done, channel close,
  wg.Wait); no goroutine leak on Close.
- **Fail**: `go func()` without a stop signal; goroutine that outlives Close.
- **Severity**: P0.

### B5. Benchmark Exists
- **Check**: is there a `bench_test.go`?
- **Pass**: at least one Benchmark for the hot-path operation with `-benchmem`.
- **Acceptable**: no bench for cold/utility packages (config, health, shutdown).
- **Fail**: no benchmark on a performance-critical package.
- **Severity**: P1 for perf-critical; P3 for others.

### B6. Disk Footprint (modules)
- **Check**: `go list -deps -m` for the module.
- **Pass**: own module has ≤5 direct deps; root module adds zero new deps.
- **Fail**: module with >10 deps; root module modified go.mod.
- **Severity**: P1.

---

## C. Testing & Coverage (QA)

### C1. Coverage Threshold
- **Check**: `go test -cover`.
- **Pass**: ≥90% statement coverage.
- **Acceptable**: 80-90% if the uncovered code is defensive/unreachable (e.g.,
  `math.MaxUint64` overflow guards).
- **Fail**: <80%.
- **Severity**: P0.

### C2. Race Detection
- **Check**: `go test -race`.
- **Pass**: clean (no warnings, no failures).
- **Fail**: any data race detected.
- **Severity**: P0.

### C3. Edge Cases
- **Check**: test file covers: nil input, empty input, zero/negative values,
  max values, concurrent access, resource exhaustion (full buffer/queue).
- **Pass**: ≥1 test per edge case category.
- **Fail**: no nil/empty/concurrent tests.
- **Severity**: P0 for nil/concurrent; P1 for others.

### C4. Table-Driven
- **Check**: test structure — table-driven where applicable?
- **Pass**: complex functions use table-driven tests with named cases.
- **Severity**: P2 (style).

### C5. No flaky tests
- **Check**: tests don't use `time.Sleep` for timing (use injected clocks);
  tests don't depend on external services (use mocks/in-process servers).
- **Pass**: deterministic; uses injected clock / mock / in-process server
  (miniredis, bufconn, httptest).
- **Fail**: `time.Sleep` for correctness assertion; depends on external service
  without env-gated skip.
- **Severity**: P0.

### C6. Lint Clean
- **Check**: `golangci-lint run` + `go vet`.
- **Pass**: 0 issues.
- **Severity**: P0.

---

## D. Concurrency Safety (QA)

### D1. Thread Safety Documented
- **Check**: godoc states whether the type is safe for concurrent use.
- **Pass**: explicit "safe for concurrent use" or "not safe; use per-shard + Merge".
- **Fail**: no mention of concurrency safety.
- **Severity**: P1.

### D2. No Race Between Close and Use
- **Check**: Close + concurrent Use (Publish/Allow/Send/Get).
- **Pass**: Close uses CAS/once/mutex to atomically claim shutdown; Use checks
  closed flag under the same lock or via atomic; no "send on closed channel".
- **Fail**: check-then-act race on Close; panic possible under concurrent Close+Use.
- **Severity**: P0.

### D3. Concurrency Model Documented
- **Check**: if the package is NOT internally synchronized (e.g., hyperloglog),
  does the godoc explain the intended model (shard + Merge)?
- **Pass**: "Add is not internally synchronized; use per-shard + Merge" or similar.
- **Severity**: P1.

---

## E. Observability (SRE)

### E1. Metrics/Snapshot
- **Check**: does the package expose counters/snapshots?
- **Pass**: `Metrics()` or `Snapshot()` method returning a struct of atomic
  counters; or explicit "no metrics by design" with rationale.
- **Acceptable**: pure algorithm primitives (hash, base62) may omit metrics.
- **Severity**: P1 for infra packages; P3 for pure algorithms.

### E2. Event Hooks
- **Check**: `SetOnEvent` / `OnEvent` / callback mechanism?
- **Pass**: optional callback for critical events (overflow, error, close).
- **Severity**: P2.

### E3. Health Integration
- **Check**: can the package's readiness be probed?
- **Pass**: `Ping()` / `Healthy()` / implements health.Checker; or N/A for
  pure-data packages.
- **Severity**: P2 for connection-based packages; N/A for algorithms.

---

## F. API & Developer Experience (Developer)

### F1. README
- **Check**: `README.md` exists with: what it does, API table, example, ad-tech
  use, testing instructions.
- **Pass**: all 5 sections present.
- **Fail**: missing README or <3 sections.
- **Severity**: P0.

### F2. Godoc Examples
- **Check**: `example_test.go` with runnable examples.
- **Pass**: ≥1 `Example` function per public type/function.
- **Acceptable**: simple packages may skip if README has code blocks.
- **Severity**: P1.

### F3. Zero-Config Usability
- **Check**: can a new user use the package with zero configuration?
- **Pass**: `New()` works with no options; sensible defaults.
- **Fail**: must pass 3+ required params to construct.
- **Severity**: P0.

### F4. Naming Convention
- **Check**: exported names follow Go convention (CamelCase, no ALL_CAPS, no
  stuttering `cache.Cache`).
- **Pass**: `cache.Store`, `lru.Cache`, not `Cache.Cache`.
- **Severity**: P1.

---

## G. Security & Robustness (Security)

### G1. Input Validation
- **Check**: exported functions validate inputs (nil, empty, negative, overflow).
- **Pass**: bad input returns error or panics with clear message; no silent
  corruption.
- **Severity**: P0.

### G2. No Secrets in Code
- **Check**: no hardcoded passwords, API keys, connection strings.
- **Pass**: all secrets come from config/env.
- **Severity**: P0.

### G3. Resource Exhaustion Resistance
- **Check**: can a malicious/large input cause OOM or goroutine explosion?
- **Pass**: bounded buffers, max sizes, backpressure; documented limits.
- **Fail**: unbounded map/slice growth from user input.
- **Severity**: P0.

---

## H. Dependency Hygiene (Architect)

### H1. Root Module Purity
- **Check**: `git diff` on root `go.mod` after adding a new root-module package.
- **Pass**: zero changes to root `go.mod`.
- **Fail**: new external dep added to root.
- **Severity**: P0.

### H2. Module Isolation
- **Check**: each own-module package has its own `go.mod`; deps listed there only.
- **Pass**: `go.mod` in the package dir; deps NOT in root.
- **Severity**: P0.

### H3. go.work Sync
- **Check**: `go.work` lists all modules; `go.work.sum` is current.
- **Pass**: `go build ./...` works from root.
- **Severity**: P1.

---

## Multi-Agent Review Design (Future Implementation)

Each role agent receives:
1. The package source code
2. The rules for its dimension
3. The check tools available (go test, golangci-lint, bench, etc.)

Agents run in parallel and return:
```
{
  "role": "SRE",
  "dimension": "Performance & Resources",
  "findings": [
    {"rule": "B1", "severity": "P0", "status": "PASS", "detail": "0 allocs/op"},
    {"rule": "B2", "severity": "P0", "status": "FAIL", "detail": "unbounded map in Parse()"},
  ],
  "summary": "1 FAIL, 4 PASS"
}
```

Final synthesis collects all findings, sorts by severity, and outputs a
go/no-go decision.

### Agent Assignments

| Agent | Dimensions | Tools |
|---|---|---|
| **Architect** | A (architecture), H (dependency hygiene) | `go list`, import analysis, godoc review |
| **SRE** | B (performance), E (observability) | `go test -bench -benchmem`, `go vet`, code review |
| **QA** | C (testing), D (concurrency) | `go test -race -cover`, test file review |
| **Developer** | F (API & DX) | README check, godoc, example review |
| **Security** | G (security) | Input validation audit, resource exhaustion review |
