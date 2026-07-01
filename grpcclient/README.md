# grpcclient: gRPC client middleware

gRPC client middleware (interceptors, not generated stubs): retry with backoff
on retryable status codes, optional circuit-breaker integration, latency
observation, and a metrics surface. Compose the interceptors onto your own dial.

Configuration is a `ClientOptions` struct (json + mapstructure tags).

## Usage

- `NewMiddleware(opts ClientOptions) *Middleware`.
- `(*Middleware).UnaryClientInterceptor() grpc.UnaryClientInterceptor`.
- `(*Middleware).StreamClientInterceptor() grpc.StreamClientInterceptor`.
- `DialConn(opts ClientOptions) (*grpc.ClientConn, error)` ready-to-use conn with
  the interceptors attached.

## ClientOptions

- `Target` the dial target (required for `DialConn`).
- `ConnectTimeout`, `RequestTimeout`.
- `RetryMax`, `RetryCodes []codes.Code` (which codes are retryable),
  `RetryWaitMin`, `RetryWaitMax` exponential backoff.
- `Breaker CircuitBreaker` — non-nil wraps calls; nil disables.
- `Latency LatencyObserver` — non-nil receives durations; nil disables.

The `Client` wrapper exposes `SetOnEvent(func(ClientEvent))` and
`Metrics() ClientMetrics` for observation.

## Example

One-shot dial (interceptors built internally from the same options):

```go
import "github.com/v8fg/kit4go/grpcclient"

conn, err := grpcclient.DialConn(grpcclient.ClientOptions{
    Target:       target,
    RetryMax:     3,
    RetryCodes:   []codes.Code{codes.Unavailable},
    RetryWaitMin: 100 * time.Millisecond,
    RetryWaitMax: 2 * time.Second,
})
```

Or attach the interceptors to your own dial:

```go
mw := grpcclient.NewMiddleware(opts)
grpc.Dial(opts.Target,
    grpc.WithUnaryInterceptor(mw.UnaryClientInterceptor()),
    grpc.WithStreamInterceptor(mw.StreamClientInterceptor()),
)
```
