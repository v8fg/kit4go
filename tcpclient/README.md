# tcpclient: TCP & Unix-socket client

A connection-pooled TCP / Unix-socket client with deadline-based I/O, retry with
backoff on retriable transport errors, latency observation, and a metrics
surface. Built for high-throughput request/response protocols (bidding,
real-time enrichment) where connection reuse matters.

Configuration is a `ClientOptions` struct (json + mapstructure tags).

## Features

- Connection pool with size cap and idle-expiry; safe under concurrency.
- `Send` (fire) and `SendReceive` (request/response) with read/write deadlines;
  `SendReceiveLine` reads up to a line delimiter.
- Retry with backoff; ctx-cancellation aware during backoff.
- Optional breaker / latency observer; `Metrics` for export.

## Usage

- `NewClient(opts ClientOptions) *Client`.
- `(*Client).Send(ctx, data []byte) error`.
- `(*Client).SendReceive(ctx, data []byte) ([]byte, error)`.
- `(*Client).SendReceiveLine(ctx, data []byte) (string, error)`.
- `(*Client).DoWithRetry(ctx, func(ctx) error) error` retry a custom op.
- `(*Client).SetOnEvent(func(ClientEvent))`, `(*Client).Metrics() ClientMetrics`.
- `(*Client).Close() error`.

## ClientOptions

- `Network` (`tcp` / `unix` / ...), `Address`.
- `ConnectTimeout`, `ReadTimeout`, `WriteTimeout`.
- `PoolSize`, `IdleTimeout`.
- `RetryMax`, `RetryWaitMin`, `RetryWaitMax`.
- `Breaker CircuitBreaker`, `Latency LatencyObserver` (nil disables).

## Example

```go
import "github.com/v8fg/kit4go/tcpclient"

c := tcpclient.NewClient(tcpclient.ClientOptions{
    Network: "tcp", Address: "127.0.0.1:9000",
    PoolSize: 32, ReadTimeout: time.Second,
})
defer c.Close()
resp, err := c.SendReceive(ctx, req)
```
