# kit Package Quality Rules

> A **language-neutral** quality framework for every kit package. Each rule is
> stated as a universal principle; the concrete check is given for **Go** (the
> kit's current language) and instantiated under other languages via the
> [Cross-language mechanism map](#cross-language-mechanism-map) when the kit
> expands. Every rule cites a source, a severity (P0-P3), and an acceptance
> level (Universal / Strong-consensus / Opinionated). Designed for multi-agent
> parallel review: each role (Architect, SRE, QA, Developer, Security) runs
> independently.

## Guiding Principles (simplicity-first)

All rules below serve a small set of foundational tenets. When a rule and these
tenets conflict, prefer the **smaller correct thing**. The ethos is
simplicity-first engineering (the discipline most associated with Karpathy's
writing): bias to deletion, to the obvious, and to the present requirement.

1. **Delete-first.** The best code is the code not written, or the code removed.
   Fewer lines, fewer abstractions, fewer config knobs. When in doubt, cut.
2. **Flat and boring.** Code is communication for the next human. Obvious,
   linear, unsurprising. Cleverness is a liability, not a virtue.
3. **YAGNI.** Build for the present requirement, not an imagined future. No
   speculative generics, options, or seams — add them when a real second use
   arrives (see A7). "The fastest code is the code that doesn't run."
4. **Work → right → fast.** Tight feedback loops over working increments:
   correctness first, then clarity, then performance. Performance work (D) is
   earned by a measured need, never guessed.
5. **Comments explain WHY, not WHAT.** The code already shows what it does; a
   comment captures intent, invariant, or a non-obvious trade-off — never a
   restatement of the code.
6. **Compose, don't configure.** Prefer small things you combine over big things
   you parameterize. One obvious way to do something; two ways is a smell.

These six are the umbrella; dimensions A-L are their concrete, checkable
expression. A rule that cannot be tied back to one of them is probably
over-engineering.

## Multi-language intent

kit4go is Go today. These rules are written **principle-first** so the same
framework governs future kits in other languages (Rust, Java/JVM, TypeScript),
and so the engineering principles transfer verbatim. A rule's *Principle* line
is language-neutral; its *Go* line is the current instantiation. When a second
language lands, mirror each principle under that language's tooling using the
mechanism map below — do **not** fork a separate rulebook.

## How to read a rule

```
### D1. Hot path = zero allocation  [P0]
- Principle:  <language-neutral statement + why>
- Go:         <concrete check/threshold for Go today>
- Sources:    <industry calibration + acceptance level>
```

Severity: **P0** blocks release; **P1** should-fix; **P2/P3** nice-to-have.
"Universal" = a real industry consensus; "Strong consensus" = widely adopted;
"Opinionated" = this kit's stance.

## Cross-language mechanism map

The same principle is checked with different tooling per language. Reference this
table instead of restating the tool on every rule.

| Mechanism | Go (today) | Java / JVM | Python | Rust | Frontend (React / Vue) |
|---|---|---|---|---|---|
| Data-race freedom | `go test -race` | Thread Sanitizer, JFR | GIL mitigates; `threading` locks; pytest | `Send`/`Sync` + Miri | single-threaded; pure render + effect cleanup |
| Hot-path allocations | `-benchmem` (0 allocs/op) | JMH `-prof gc` | avoid in tight loops (GC'd) | verify no `alloc`/`Box`/`String` | avoid re-render (`useMemo`/`computed`); no allocs in render |
| Lint baseline | golangci-lint | checkstyle/errorprone/spotbugs | ruff + mypy + black | clippy + rustfmt | eslint + tsc + plugin-react/-vue |
| CSPRNG | `crypto/rand` | `SecureRandom` | `secrets` | `getrandom` / `rand` | Web Crypto / `crypto.getRandomValues` |
| Cancellation / timeout | `context.Context` | deadline / `CompletableFuture` | `asyncio` cancel / `threading.Event` | `CancellationToken` (tokio) | `AbortSignal`; React cleanup return; Vue `onScopeDispose` |
| Error identity / chain | `errors.Is/As` + `%w` | typed exceptions | exception types / groups (3.11) | `thiserror`/`anyhow` `Downcast` | typed `Result` / `Error` subclass / `ErrorBoundary` |
| Async unit | goroutine + `wg`/goleak | `Executor` / `Thread` | `asyncio.Task` / thread | tokio task / thread | `Promise`/`async`; effect; `watchEffect` |
| Naming style | MixedCaps | camelCase / PascalCase types | PEP 8 snake / Pascal classes | snake_case / PascalCase types | camelCase; components PascalCase |
| Module-load side effects | `init()` | `static` initializer | module top-level (guard `__main__`) | (none) | ESM top-level |
| Forced process exit | `os.Exit` / `log.Fatal` | `System.exit` | `sys.exit` | `process::exit` | n/a browser; Node `process.exit` |
| Coverage | `go test -cover` | JaCoCo | pytest-cov / coverage.py | tarpaulin / llvm-cov | vitest / jest + c8 |
| Render purity / effects | n/a | n/a | n/a | n/a | pure render; no side-effects in render; cleanup effects; stable `key`s |
| Bundle / tree-shake | n/a | n/a | n/a | (binary size) | tree-shakeable; size budget; code-split |
| Accessibility (a11y) | n/a | n/a | n/a | n/a | semantic HTML; ARIA; keyboard; color contrast |

> **Frontend notes.** React and Vue share the TypeScript/JS substrate; the
> column shows the framework idiom. Their "concurrency" is not thread-safety
> but **render purity** (no side-effects during render, effects cleaned up,
> stable keys, memoised expensive values) and **reactivity correctness** (no
> stale closures, no infinite re-render). Bundle size, tree-shakeability, and
> accessibility are first-class performance/quality concerns with no backend
> analogue (rows 12-14).
>
> **Python notes.** The GIL means true data races are rare; concurrency rules
> (F) map to lock discipline and asyncio task isolation, not a race detector.
> Python is interpreted/GC'd, so D1 (zero allocation) is not enforced — D
> applies as "no quadratic loops / no accidental O(n²) in hot paths."

---

## A. Architecture & Project Layout (Architect)

### A1. Single Responsibility [P0]
- **Principle**: each package/module has one clear purpose; no catch-all names
  (`util`, `common`, `helpers`, `shared`, `types`).
- **Go**: package godoc states one purpose; no `util`/`common`.
- **Sources**: Google, K8s, Effective Go — Universal.

### A2. Module Layout [P0]
- **Principle**: module/package path matches its directory; one unit per dir;
  private code under an internal/private path; entry points isolated.
- **Go**: `package foo` == dir name; `internal/` for non-public; `cmd/` for mains.
- **Sources**: go.dev modules-layout, K8s — Universal.

### A3. Interface Segregation [P1]
- **Principle**: abstractions are small and role-focused; consumers never depend
  on members they don't use.
- **Go**: exported interfaces ≤3 methods; single-method named with `-er`
  suffix; define at consumer side where possible.
- **Sources**: Go Code Review Comments, Google — Universal.

### A4. Accept Abstractions, Return Concretes [P1]
- **Principle**: APIs take the narrowest abstraction and return concrete types
  (so callers get value/identity, not a hidden contract).
- **Go**: `func New(s Store) *Cache`, not `func New(s *Store) *Store`.
- **Sources**: Go Tip #49, Google — Universal.

### A5. Compile-Time Contract Verification [P1]
- **Principle**: any type claimed to satisfy a contract is asserted at compile
  time, not by a runtime test.
- **Go**: `var _ I = (*T)(nil)` for exported types implementing API contracts.
- **Sources**: Uber, Effective Go — Universal.

### A6. Configuration via Options, Zero-Config Works [P1]
- **Principle**: construction is configurable but the zero-config default is
  production-usable; no positional constructor with many params.
- **Go**: `New(opts ...Option)` functional options; defaults applied.
- **Sources**: Uber — Strong consensus.

### A7. No Over-Engineering [P1]
- **Principle**: every exported symbol is used or demonstrably useful; no
  speculative generics, no "future-proof" abstractions, no unused config knobs.
- **Sources**: K8s ("avoid package sprawl"), Google — Universal.

### A8. No Module-Load Side Effects [P0]
- **Principle**: importing/-loading the module performs no I/O, env access, flag
  registration, or global mutation. Configure via API, not load-time magic.
- **Go**: no `init()` doing work (compile-time constants only).
- **Sources**: Uber, Google — Universal.

---

## B. Error Handling (Architect + Security)

### B1. Failure is a Value, Propagated Explicitly [P0]
- **Principle**: operations that can fail expose failure in the type system; the
  caller cannot ignore it.
- **Go**: functions return `error` as the last return value.
- **Sources**: Google, Effective Go — Universal.

### B2. Sentinels / Typed Errors, Chain Preserved [P0]
- **Principle**: callers match errors by identity/type, never by message text;
  context is added without losing the original.
- **Go**: exported `Err*` sentinels; wrap with `fmt.Errorf("ctx: %w", err)`;
  `errors.Is`/`errors.As` works end-to-end. No bare `errors.New` callers must
  string-match.
- **Sources**: Uber, Google — Universal.

### B3. No In-Band Sentinel Values [P0]
- **Principle**: no returning `-1`/`nil`/`""` to mean "failure". Use `(value, ok)`
  or `(value, error)` / a `Result` type.
- **Sources**: Google — Universal.

### B4. Handle Errors Once [P1]
- **Principle**: don't both log and return the same error. Either degrade, or
  wrap-and-propagate.
- **Sources**: Dave Cheney, Uber — Universal.

### B5. No Exception/Panic for Normal Failures [P0]
- **Principle**: bad input or expected runtime failure is a value, not an
  exception/panic. Non-local control flow is reserved for truly unrecoverable
  programmer error or an explicit `Must*` variant.
- **Go**: `panic` only in `Must*` helpers or unrecoverable conditions.
- **Sources**: Uber, Google — Universal.

### B6. No Forced Process Exit in Library Code [P0]
- **Principle**: a library never terminates the process; only the application
  entry point may.
- **Go**: `os.Exit`/`log.Fatal` only in `main()`.
- **Sources**: Uber, Google — Universal.

### B7. No Error-Message String Matching [P1]
- **Principle**: never branch on the text of an error/exception message.
- **Go**: use `errors.Is`/`errors.As`, never `err.Error()` content.
- **Sources**: Dave Cheney, Google — Strong consensus.

---

## C. Naming & Style — Language-Convention Compliance (Developer)

> Naming is **not** universal — it follows each language's canonical style guide.
> The rules below are the Go instantiation; other languages comply with theirs
> (Rust API Guidelines, Google Java Style, etc.).

### C1. Canonical Identifier Style [P0]
- **Principle**: identifiers follow the language's official style exactly.
- **Go**: MixedCaps — PascalCase exported, camelCase unexported; no `_` in
  identifiers (except `_test.go` funcs / `_test` package suffix).
- **Sources**: Google, Uber — Universal (the rule); Go-specific (the form).

### C2. Initialisms Keep Case [P0]
- **Go**: `URL`/`ID`/`HTTP`/`API`/`DB`, never `Url`/`Id`/`Http`.
- **Sources**: Google — Universal within Go.

### C3. Receiver / Self Names [P1]
- **Go**: 1-2 letter abbreviation, consistent across methods; never
  `this`/`self`/`_` (e.g. `func (c *Cache)`).
- **Sources**: Google, Uber — Universal.

### C4. No `Get` Prefix on Accessors [P1]
- **Go**: `Counts()`, not `GetCounts()`.
- **Sources**: Google — Universal.

### C5. Constants — Language-Canonical Case [P1]
- **Go**: MixedCaps constants; no `MAX_SIZE`/`kDefaultPort`.
- **Sources**: Google — Universal.

### C6. Locks Named, Never Embedded [P1]
- **Go**: `mu sync.Mutex` as a named field; never anonymous embed; multiple locks
  get a suffix (`stateMu`, `mapMu`).
- **Sources**: K8s, Uber — Strong consensus.

### C7. Module/Package Name — Canonical Form [P0]
- **Go**: all-lowercase, singular, matches dir (`net/url`, `cache`).
- **Sources**: Google, Uber — Universal.

---

## D. Performance & Resources (SRE)

### D1. Hot Path = Zero Allocation [P0]
- **Principle**: the hottest operations (Get/Set/Allow/Push/Observ­e) allocate no
  heap memory; garbage is the enemy of tail latency.
- **Go**: `go test -bench -benchmem`; 0 allocs/op. ≤2 allocs/op acceptable only
  if documented and justified.
- **Sources**: fasthttp, bigcache — Strong consensus for perf libs.

### D2. Prefer Zero-Reflection Primitive Conversion [P1]
- **Principle**: convert primitives without the reflection-based formatter.
- **Go**: `strconv.Itoa`/`ParseInt`, not `fmt.Sprint`/`Sprintf`.
- **Sources**: Uber — Universal.

### D3. Pre-size Collections [P1]
- **Principle**: size collections at construction when size is known/estimable.
- **Go**: `make([]T, 0, cap)`, `make(map[K]V, hint)`.
- **Sources**: Uber — Universal.

### D4. Hoist Constant Work Out of Loops [P1]
- **Principle**: don't recompute/convert a constant inside a hot loop.
- **Go**: `[]byte("constant")` hoisted out.
- **Sources**: Uber — Universal.

### D5. Memory Bounded [P0]
- **Principle**: every collection/buffer/cache has an explicit cap; no unbounded
  growth from any input.
- **Go**: `MaxSize`/`MaxKeys`/capacity parameter; see also I3.
- **Sources**: kit convention (OOM prevention) — Universal.

### D6. Lock Granularity [P1]
- **Principle**: shared read lock vs exclusive write; no I/O/allocation under a
  lock; single-variable hot paths use CAS.
- **Go**: `RLock`/`Lock`; `atomic` CAS where it fits.
- **Sources**: Uber — Universal.

### D7. Async-Unit Hygiene [P0]
- **Principle**: every background task/goroutine/thread has a shutdown path
  (cancellation, channel close, or join). No fire-and-forget without ownership.
  Leak-detected in tests.
- **Go**: every goroutine exits on `ctx.Done`/channel close/`wg.Wait`; `goleak`
  in packages that spawn goroutines.
- **Sources**: Uber, Google ("goroutine lifetimes") — Universal.

### D8. Benchmark Exists [P1 perf / P3 cold]
- **Principle**: hot-path operations have a microbenchmark that reports
  allocations, checked in CI.
- **Go**: `bench_test.go` with `b.ReportAllocs()`; ≥1 Benchmark per hot-path fn.
- **Sources**: Go bench docs — Universal.

### D9. Prefer Synchronous APIs [P1]
- **Principle**: the package provides a synchronous API; the caller adds
  concurrency. No forced background tasks baked into the primitive.
- **Sources**: Google — Strong consensus.

---

## E. Testing & Coverage (QA)

### E1. Coverage Threshold [P0]
- **Principle**: high statement/branch coverage with meaningful assertions, not
  line-padding.
- **Go**: `go test -cover` ≥90% (team policy; no industry minimum). 80-90%
  acceptable for defensive/unreachable code; <80% fails.
- **Sources**: Google/Uber/K8s — Opinionated threshold.

### E2. Race Detection [P0]
- **Principle**: the data-race detector is clean on every test run.
- **Go**: `go test -race`.
- **Sources**: Go race detector, Uber — Universal.

### E3. Table-Driven / Parameterised Tests [P1]
- **Principle**: multi-input logic is exercised via a named, parameterised table.
- **Go**: table-driven with `t.Run`, named rows, no complex branching.
- **Sources**: Uber, Google, K8s — Universal.

### E4. Test Helpers Marked [P2]
- **Principle**: test helpers report failures at the call site, not inside the
  helper.
- **Go**: `t.Helper()` after the context param.
- **Sources**: Google — Universal.

### E5. No Flaky Tests [P0]
- **Principle**: tests never rely on wall-clock sleeps for correctness; use
  injected clocks, fakes, or in-process servers.
- **Go**: injected clocks/mocks; miniredis, bufconn, httptest; no `time.Sleep`
  gating assertions.
- **Sources**: K8s ("wait-and-retry, not sleep-one-second") — Strong consensus.

### E6. Edge Cases [P0 nil/concurrent, P1 others]
- **Principle**: nil/empty, zero/negative, max values, concurrency, and resource
  exhaustion (full buffer) each have ≥1 test.
- **Sources**: Universal.

### E7. Black-Box Test Package [P2]
- **Principle**: a portion of tests exercise only the public API.
- **Go**: external `package foo_test`.
- **Sources**: Google, K8s — Universal.

### E8. Lint Clean [P0]
- **Principle**: the language linter + formatter are zero-issue.
- **Go**: `golangci-lint run` + `go vet` = 0 issues.
- **Sources**: Uber baseline — Universal.

### E9. Cross-Platform [P1]
- **Principle**: tests pass on the target platforms; platform-specific code is
  tagged or conditionally skipped.
- **Go**: macOS + Linux; build tags or `t.Skip`.
- **Sources**: K8s — Strong consensus.

### E10. Fuzz Testing [P1]
- **Principle**: packages with non-trivial logic (algorithms, parsers, numeric,
  concurrent state machines) carry at least one `Fuzz*` target that encodes a
  **real invariant** — not merely "does not panic". The fuzzer explores edge
  cases (float-drift, buffer overflow, charset edge, off-by-one) that unit
  tests and coverage metrics systematically miss. Empirically, fuzzing finds
  the highest-severity bugs per minute invested.
- **Go**: `func FuzzXxx(f *testing.F)` with `f.Add(seed)` corpus + `f.Fuzz(func(t, ...))`
  asserting a postcondition. At least one target per package whose invariant is
  non-trivially guaranteed (interpolation monotonicity, round-trip identity,
  bounded output, distinct-element count, etc.). Packages whose invariants are
  mathematically provable (e.g. a pure CMS over-approximation) are exempt.
- **Before release**: run high-value targets for ≥5 min each (`go test -fuzz -fuzztime=5m`).
  The fuzzer writes failing inputs to `testdata/fuzz/`, which `go test` then
  replays — an unfixed corpus breaks the regular test suite until the bug is
  fixed or the corpus is `git rm`'d.
- **Gotcha**: a `for i := range N` loop whose body **modifies `i`** (e.g.
  `i += skip`) silently breaks under range — `range` reassigns `i` each
  iteration. Use the classic `for i := 0; i < N; i++` when the body mutates
  the loop variable.
- **Sources**: Go blog "Fuzzing is testing" (2024); redis-cell float-overflow
  pattern; kit4go 2026-07 fuzz campaign (16 targets × 5 min, 100M+ execs, found
  3 real bugs that 100% coverage missed).

---

## F. Concurrency Safety (QA)

### F1. Concurrency Safety Documented [P1]
- **Principle**: every public type states its concurrency contract ("safe for
  concurrent use" / "not safe; shard + merge").
- **Go**: stated in godoc.
- **Sources**: kit convention — Strong consensus.

### F2. No Use-After-Close / Close-Use Race [P0]
- **Principle**: close/shutdown is race-free with ongoing use; no operation can
  touch a resource after it is closed.
- **Go**: Close via CAS/`Once`/mutex; Use checks under the same guard; no
  send-on-closed-channel.
- **Sources**: Uber — Universal.

### F3. No Mutable Globals [P1]
- **Principle**: no package/module-level mutable state; use dependency injection.
- **Go**: no runtime-mutating package-level `var`.
- **Sources**: Uber — Strong consensus.

### F4. Queue/Channel Sizes Justified [P1]
- **Principle**: buffering is intentional and bounded, with a documented
  backpressure strategy.
- **Go**: channel cap 0 (unbuffered) or 1 by default; larger needs a documented
  bound + overflow policy.
- **Sources**: Uber — Strong consensus.

### F5. Locks are Zero-Value [P1]
- **Go**: `sync.Mutex`/`RWMutex` as a zero-value field; never `new(sync.Mutex)`.
- **Sources**: Uber — Universal.

---

## G. Observability (SRE)

### G1. Library Uses the Telemetry API, Never the SDK [P0 instrumented]
- **Principle**: instrumentation depends only on the telemetry **API**, so it is
  a no-op when the application hasn't wired the SDK.
- **Go**: import `go.opentelemetry.io/otel` (API), not `.../otel/sdk`.
- **Sources**: OpenTelemetry library guide — Universal.

### G2. No Direct Logging in Library Code [P0]
- **Principle**: a library emits no logs itself; it exposes callbacks/interfaces
  (`SetOnEvent`, `OnEvent`) for the host to observe.
- **Go**: no `log.Printf`/`slog.Info`; callbacks instead.
- **Sources**: OTel guide — Strong consensus.

### G3. Metrics / Snapshot Exposure [P1 infra / P3 algo]
- **Principle**: long-lived components expose counters/snapshots, or document
  "no metrics by design".
- **Go**: `Metrics()`/`Snapshot()` returning atomic counters.
- **Sources**: kit convention.

### G4. Telemetry Naming [P1 instrumented]
- **Principle**: tracer/meter names follow `library-name/version` form.
- **Go**: `"kit4go/kafka"`.
- **Sources**: OTel guide — Universal.

### G5. Span Lifecycle [P0 instrumented]
- **Principle**: spans always end (deferred); errors are recorded + status set.
- **Go**: `span.End()` in `defer`; `RecordError` + `SetStatus(ERROR)` on errors.
- **Sources**: OTel guide — Universal.

### G6. Cheap When Inactive [P1 instrumented]
- **Principle**: skip expensive attribute building when telemetry isn't recording.
- **Go**: guard on `span.IsRecording()`.
- **Sources**: OTel guide — Strong consensus.

---

## H. API & Developer Experience (Developer)

### H1. Zero-Value / Default is Useful [P1]
- **Principle**: a type is usable from its default/zero state without a
  constructor where feasible.
- **Go**: `var c Cache` works without `New*()`.
- **Sources**: Uber, stdlib — Universal.

### H2. No nil-vs-empty Ambiguity [P1]
- **Go**: check `len(s) == 0`, never `s == nil`; return `nil`, not `[]T{}`.
- **Sources**: Google, Uber — Universal.

### H3. Copy Collections at Boundaries [P1]
- **Principle**: when receiving or returning internal collections, copy to avoid
  aliasing.
- **Go**: copy slices/maps when storing caller input or exposing internal state.
- **Sources**: Uber — Strong consensus.

### H4. Cancellation is the First Concern [P0]
- **Principle**: every blocking operation accepts cancellation/timeout via the
  language's idiom; it is never stashed in a struct for later.
- **Go**: `context.Context` first param; never stored in a struct field.
- **Sources**: Google — Universal.

### H5. README [P0]
- **Principle**: every package has a README with what/how/API table/example/domain
  use/testing.
- **Sources**: kit convention — Universal.

### H6. Runnable Examples [P1]
- **Principle**: the public API has executable, tested examples.
- **Go**: `example_test.go` with `Example` functions.
- **Sources**: Effective Go — Universal.

---

## I. Security & Robustness (Security)

### I1. CSPRNG for Security-Sensitive Randomness [P0]
- **Principle**: tokens, nonces, IDs, secrets use a CSPRNG, never a fast PRNG.
- **Go**: `crypto/rand`, not `math/rand`.
- **Sources**: Google — Universal.

### I2. Input Validation [P0]
- **Principle**: exported functions validate nil/empty/negative/overflow.
- **Sources**: Universal.

### I3. Resource-Exhaustion Resistance [P0]
- **Principle**: bounded buffers, max sizes; no unbounded growth from untrusted
  input (connects D5, F4, L3).
- **Sources**: Universal.

### I4. Security Linter Enabled [P1]
- **Go**: `gosec` in the linter config.
- **Sources**: golangci-lint golden config — Strong consensus.

### I5. Resource-Cleanup Checks [P1 I/O packages]
- **Principle**: resources (HTTP bodies, DB rows, sockets, files) are always
  closed; requests carry cancellation.
- **Go**: `bodyclose`, `noctx`, `rowserrcheck`, `sqlclosecheck`.
- **Sources**: golangci-lint golden config — Strong consensus.

### I6. No Hardcoded Secrets [P0]
- **Principle**: no passwords, keys, or connection strings in source.
- **Sources**: Universal.

---

## J. Dependency Hygiene (Architect)

### J1. Root Module Purity [P0]
- **Go**: root `go.mod` unchanged after adding a root-module package.
- **Sources**: kit convention.

### J2. Module Isolation [P0]
- **Principle**: heavy-dependency components live in their own module so the
  dependency is opt-in.
- **Go**: own-module packages have `go.mod`; deps listed there only.
- **Sources**: go.dev modules-layout — Universal.

### J3. No Side-Effect Imports [P1]
- **Go**: no `import _ "..."` in library code (only in main/tests).
- **Sources**: Google — Universal.

### J4. No Dot Imports [P0]
- **Go**: no `import . "..."` except in `_test` packages.
- **Sources**: Google, Uber — Universal.

### J5. Dependency Deny-List [P1]
- **Principle**: block known-deprecated/insecure dependencies.
- **Go**: depguard denies `satori/go.uuid`, `golang/protobuf`, `math/rand` for
  security use.
- **Sources**: golangci-lint golden config — Strong consensus.

---

## K. Lint & Style Baseline (All Roles)

> The baseline is **language-neutral in intent** (zero lint issues, formatted,
> tuned complexity). The concrete config below is Go today; each new language
> adopts its equivalent (Rust: clippy/rustfmt; JVM: checkstyle/errorprone;
> TS: eslint/tsc).

### K0. Trim by Codebase Stance, Don't Enable Blindly
K1/K2 is an **upper-bound menu**, not a mandate. Enable the subset that catches
real bugs at low noise for THIS codebase; leave linters whose value is mostly
style or false-positives off, with a one-line rationale recorded in the config.

- **Enable eagerly**: bug-catching linters with few false positives (`errorlint`,
  `nilerr`, `nilnesserr`, `wastedassign`, `reassign`).
- **Measure before enabling anything style-flavored** (`revive`, `gocritic`,
  `gosimple`, `predeclared`, `errname`, `exhaustive`, `testifylint`, `bodyclose`):
  run the candidate config repo-wide first. A 0-finding linter is a free
  regression net; a flood of low-signal findings means the linter fights the
  codebase's own style — keep it off and say why. A blanket enable that generates
  `//nolint` noise is a net loss.

kit4go stance (`.golangci.yml`): high-signal correctness set only; opinionated
style families (staticcheck `ST*`/`QF*`/`S*`, `revive`, `gocritic`,
`predeclared`) are deliberately off because the codebase has its own consistent
style, and `bodyclose`/`predeclared` were measured as mostly false-positives
(already-closed bodies it can't trace; readable `min`/`max` param names).

### K1. Always-On (Universal Baseline)
**Go**: `errcheck`, `govet`, `staticcheck`, `unused`, `gosimple`, `ineffassign`,
`revive`, `gocritic`, `goimports`, `gosec`.

### K2. Production-Grade Additional (Strong Consensus)
**Go**: `bodyclose`, `noctx`, `errorlint`, `errname`, `misspell`, `unconvert`,
`predeclared`, `reassign`, `usestdlibvars`, `exhaustive`, `nilerr`,
`nilnesserr`, `wastedassign`, `unparam`, `spancheck`, `testifylint`, `sloglint`.

### K3. Tuned Thresholds
| Metric | Go threshold |
|---|---|
| function length | 100 lines / 50 statements |
| line length | 120 |
| cyclo complexity | 30 (avg 10) |
| cognitive complexity | 20 |
| naked return | banned (0) |
| `govet enable-all` | true (except fieldalignment) |
| `staticcheck` | all (except ST1000/ST1016/QF1008) |
| errcheck type-assertions | true |
| `sloglint.no-global` | all |
| `nolintlint.require-explanation` | true |

---

## L. Hot-Path Infrastructure — Do-No-Harm (SRE + Security)

> Applies to components that sit on **every caller's hot path** and carry a
> "do-no-harm" contract: loggers, metrics exporters, tracers, async pipelines,
> circuit breakers. Such infrastructure must not become the cause of a host crash
> or business impact — not from its own bugs, and not from a downstream
> (broker/sink/network) failure. Isolation is achieved **by design**, not by
> blanket exception/panic swallowing.

### L1. No-Throw / No-Panic Hot Path [P0]
- **Principle**: a diagnostic or infra call must never crash the host on
  pathological input (a user value whose encoder throws/panics, a typed-nil
  receiver). Encode/marshal failures degrade to a null/placeholder, never
  propagate. This is the **only** sanctioned internal recovery — and it must be
  observable (see L5).
- **Go**: recover around `MarshalJSON`/encode; see also B5 (no panic for normal
  errors elsewhere).
- **Sources**: zap, zerolog consensus — Strong consensus.

### L2. Non-Blocking Ingress [P0]
- **Principle**: the caller's submit/log/observe call **never blocks** on a slow
  or stuck sink. Backpressure resolves to drop (counted) or bounded buffer, never
  to stalling the business path.
- **Go**: non-blocking channel send with `default` drop (e.g. `OverflowDrop`),
  never a blocking send on an unbounded queue.
- **Sources**: kit convention, async-logger consensus — Universal.

### L3. Bounded Resources [P0]
- **Principle**: every buffer, queue, spill file, and connection pool is capped.
  The component cannot OOM the host or fill its disk.
- **Go**: bounded channels, `SpillMaxBytes`, max-conn options.
- **Sources**: Universal (connects D5, I3).

### L4. Downstream Isolation [P1]
- **Principle**: a dead/slow/erroring downstream (broker, sink) is contained: a
  circuit breaker or bounded retry stops hammering it, and an optional fail-open
  fallback (local file/stderr) keeps data flowing. Downstream failure never
  propagates to the caller.
- **Go**: error counters + optional breaker + fallback sink.
- **Sources**: resilience-pattern consensus (Release-It!, istio) — Strong consensus.

### L5. Observable Degradation [P0]
- **Principle**: **every** recovered throw, dropped record, tripped breaker, and
  dead background worker is counted and surfaced via metrics/event hook. Silent
  degradation is a bug — if the component fails, the host must be able to see it.
- **Go**: `errored`/`dropped`/`recovered` counters in `Metrics()`, daemon-death
  via `SetOnEvent`.
- **Sources**: kit convention — Universal.

### L6. Safe Lifecycle / Bounded Shutdown [P0]
- **Principle**: Close/flush/drain is bounded by a deadline; it cannot deadlock
  the process on shutdown. The flush guarantee (best-effort vs durable) is
  documented and honored.
- **Go**: shutdown timeout; no unbounded `wg.Wait`; document flush semantics.
- **Sources**: Universal.

### L7. Business-Data Protection [P1 critical paths]
- **Principle**: for business-critical records (audit, billing, transactions),
  loss is bounded and visible, critical paths have a guaranteed flush, and the
  drop rate / latency / breaker-open duration are exposed as metrics so business
  owners can see impact. Extends L1-L6 with a data-integrity stance.
- **Go**: critical-level bypass of sampling + drop; durable flush on
  Panic/Fatal; loss/failover metrics.
- **Sources**: kit convention — Opinionated.

---

## Multi-Agent Review Design

Each role agent receives the package source + its dimension's rules + available
tools. Agents run in parallel. Tooling is language-neutral in role; the Go
instantiation is shown.

| Agent | Dimensions | Key tools (Go today) |
|---|---|---|
| **Architect** | A, B, J | import analysis, godoc review, `go.mod` diff |
| **SRE** | D, G, L | `go test -bench -benchmem`, `go vet`, resilience review |
| **QA** | E, F | `go test -race -cover`, `go test -fuzz`, test-file review |
| **Developer** | C, H | README check, godoc, example review |
| **Security** | I | security linter, input-validation & resource-exhaustion audit |

Each agent emits, per rule:
```
{rule, severity, PASS/FAIL/WARN, detail, source}
```

Final synthesis: collect, sort by severity (P0 first), produce a go/no-go.

### Go/No-Go Decision
- **GO**: 0 P0 failures.
- **GO with conditions**: P0 failures are on defensive/unreachable paths and
  documented.
- **NO-GO**: any P0 failure on a live code path.

### When the kit adds a second language
1. Add the language to the [mechanism map](#cross-language-mechanism-map).
2. Under each rule, add a `<Lang>:` line mirroring the principle with that
   language's tooling — do **not** duplicate the principle or fork the rulebook.
3. Add a lint-baseline section under K for that language.
4. Keep severities and acceptance levels identical across languages.
