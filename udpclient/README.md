# udpclient: UDP datagram client

A UDP client with timeout, retry with backoff (overflow-guarded), and a metrics
surface. Suited to fire-and-forget and short request/response datagram work
(metrics/statsd-style telemetry, lightweight heartbeats).

Configuration is a `ClientOptions` struct (json + mapstructure tags).

## Features

- `Send` (fire-and-forget) and `SendReceive` (request/response with read deadline).
- Retry with backoff; delay clamped and overflow-guarded; ctx-cancellation aware.
- Optional breaker; `Metrics` for export.

## Usage

- `NewClient(opts ClientOptions) (*Client, error)` (dial may fail).
- `(*Client).Send(ctx, data []byte) error`.
- `(*Client).SendReceive(ctx, data []byte) ([]byte, error)`.
- `(*Client).SetOnEvent(func(ClientEvent))`, `(*Client).Metrics() ClientMetrics`.
- `(*Client).Close() error`.

## ClientOptions

- `Address`, `LocalAddress` (bind source; optional).
- `ReadTimeout`, `WriteTimeout`, `BufferSize` (read buffer).
- `RetryMax`, `RetryWaitMin`, `RetryWaitMax`.
- `Breaker CircuitBreaker` (nil disables).

## Example

```go
import "github.com/v8fg/kit4go/udpclient"

c, err := udpclient.NewClient(udpclient.ClientOptions{
    Address:     "127.0.0.1:8125",
    ReadTimeout: 500 * time.Millisecond,
})
if err != nil { return err }
defer c.Close()
_ = c.Send(ctx, []byte("metric:1|c"))
```
