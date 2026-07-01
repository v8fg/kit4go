# health

Liveness and readiness health checks for container orchestration (Kubernetes
probes). Pure standard library.

## API

```go
h := health.New(
    health.WithChecker(health.CheckerFunc{CheckerName: "db", Fn: pingDB}),
    health.WithChecker(health.CheckerFunc{CheckerName: "redis", Fn: pingRedis}),
    health.WithCacheTTL(5*time.Second),
)
mux.Handle("/healthz", h.LivenessHandler())
mux.Handle("/readyz", h.ReadinessHandler())

// Force liveness failure (triggers pod restart):
h.SetAlive(false)
```

| Symbol | Behavior |
|---|---|
| `New(opts...)` | Build |
| `WithChecker(c)` | Add a readiness dependency check |
| `WithCacheTTL(d)` | Cache readiness results (avoid hammering deps) |
| `SetAlive(bool)` / `Alive()` | Liveness state |
| `IsReady() Report` | Evaluate all checkers, return report |
| `AddChecker(c)` | Add at runtime |
| `LivenessHandler()` | http.HandlerFunc for /healthz |
| `ReadinessHandler()` | http.HandlerFunc for /readyz |

## Testing

100% coverage, `-race` clean.

```bash
go test -race -cover ./health/...
```
