# httpserver

A production-grade HTTP server with middleware chaining, configurable timeouts,
and context-driven graceful shutdown. Pure standard library (`net/http`).

## Why

Every service's HTTP server needs the same three things: a middleware chain
(logging, recovery, rate-limiting, metrics), sensible timeouts (to prevent
slowloris and resource exhaustion), and graceful shutdown (finish in-flight
requests before the pod dies). This package wraps `net/http` with all three in a
clean builder API — one `Start(ctx)` call that blocks, serves, and shuts down
gracefully when the context is cancelled.

## API

```go
srv := httpserver.New(":8080", myHandler,
    httpserver.WithMiddleware(recoveryMW, loggingMW, ratelimitMW),
    httpserver.WithReadHeaderTimeout(5*time.Second),
    httpserver.WithWriteTimeout(30*time.Second),
    httpserver.WithShutdownTimeout(10*time.Second),
)
// In your main, with shutdown.New():
if err := srv.Start(ctx); err != nil { log.Fatal(err) }
```

| Symbol | Behavior |
|---|---|
| `New(addr, handler, opts...)` | Build with middleware chain + timeouts |
| `WithMiddleware(mw...)` | Append middleware (outer-to-inner order) |
| `WithReadHeaderTimeout/ReadTimeout/WriteTimeout/IdleTimeout` | Configurable net/http timeouts |
| `WithShutdownTimeout(d)` | Graceful-shutdown budget (default 10s) |
| `Start(ctx)` | Blocks; serves; shuts down on ctx.Done() |
| `ListenAndServe()` | Standard net/http entry (no graceful shutdown) |
| `Shutdown(ctx)` / `Close()` | Graceful / immediate stop |
| `HTTPServer()` | Underlying *http.Server (TLS, custom listeners) |

Defaults: ReadHeaderTimeout 10s, ReadTimeout 30s, WriteTimeout 30s, IdleTimeout
120s — protection against slowloris and resource leaks without being too
aggressive for normal ad-tech request latency.

## Ad-tech stack

Pair with the rest of kit4go for a complete request-processing pipeline:
- **middleware**: recovery, request-ID, logging, tracing (kit4go/tracing)
- **rate limiting**: per-route limiter (kit4go/limiter)
- **metrics**: request counters + latency (kit4go/metrics)
- **shutdown**: graceful lifecycle (kit4go/shutdown)
- **hot-key**: detect hot endpoints mid-request (kit4go/hotkey)

## Testing

100% statement coverage, `-race` clean. Covers start+serve round-trip, middleware
chain ordering (before/after), graceful shutdown with in-flight request, addr-
required guard, custom timeout config, HTTPServer exposure, Shutdown method, Close
no-op, and body passthrough.

```bash
go test -race -cover ./httpserver/...
```
