# shutdown

Orchestrates component lifecycle: start in dependency order, stop in reverse
order, each step bounded by a timeout. Pure standard library.

## Why

Every long-running service owns several components — a consumer pool, an HTTP/gRPC
server, a background worker, a DB pool — and on SIGTERM they must come down in
the right order (workers before the server, server before the DB) within the
pod's termination grace period. Doing this ad hoc per service is where shutdown
deadlocks, leaked goroutines, and dropped requests live. This package is the one
place that gets it right.

## API

```go
m := shutdown.New(
    shutdown.WithSignal(),           // SIGINT + SIGTERM (unix) trigger shutdown
    shutdown.WithStopTimeout(10*time.Second),
)
m.Add("db",   startDB,   stopDB)
m.Add("api",  startAPI,  stopAPI,  "db")     // depends on db
m.Add("worker", startWorker, stopWorker, "api") // depends on api

if err := m.Run(ctx); err != nil { log.Fatal(err) }
```

| Method | Behavior |
|---|---|
| `Add(name, start, stop, dependsOn...)` | Register a component (nil hooks = no-op); ErrDuplicate on repeat |
| `Start(ctx)` | Topological start (deps first); rolls back on failure |
| `Stop(ctx)` | Reverse-topological stop; per-component timeout; aggregates errors |
| `Run(ctx)` | Start, block until ctx/signal, then Stop |
| `Components()` | Resolved start-order names (resolve error if cyclic) |

| Option | Default | Effect |
|---|---|---|
| `WithSignal(sigs...)` | none | Cancel Run's context on a signal (no-arg default = SIGINT + SIGTERM on unix) |
| `WithStopTimeout(d)` | 10s | Per-component stop budget |
| `WithStartTimeout(d)` | 30s | Per-component start budget |

## Semantics

- **Topological order**: deps start first; dependents stop first (reverse).
  Cycles → `ErrCycle`; a dep on an unknown name → `ErrMissingDep`.
- **Rollback**: a failed Start stops the already-started components in reverse.
- **Failure isolation**: a Stop error or timeout in one component does not stop
  the rest; all errors aggregate into `*ErrShutdown` (one or more
  `ComponentError`s).
- **Timeouts**: each Stop gets its own budget; an over-budget component's context
  cancels and the shutdown of the rest continues.

## Cross-cutting use

Any service — ad-tech bidder, fin payment service, chain node, IoT gateway,
stream/push backend — that wants a single, correct graceful-shutdown path instead
of hand-rolled signal handling per component.

## Testing

95% statement coverage, `-race` clean. Covers dependency-respecting start order,
reverse stop order, cycle detection, missing-dep and duplicate errors,
start-failure rollback, stop error aggregation, stop-timeout abandonment,
Run-on-cancel, Run-on-real-SIGINT (the process signals itself), nil hooks, the
`WithSignal()` default set, and stop-without-prior-start (lazy resolve).

```bash
go test -race -cover ./shutdown/...
```
