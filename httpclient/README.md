# httpclient: HTTP client

An HTTP client over `net/http` with retry, optional circuit-breaker integration,
per-request latency observation, redirect control, HTTP/2 toggle, and split
connect/request timeouts. Pure standard library.

Configuration is a `ClientOptions` struct (json + mapstructure tags) with zero
values replaced by defaults, so it loads from JSON / Viper or is built by hand.

## Usage

- `NewClient(opts ClientOptions) *Client`.
- `(*Client).Get(ctx, url, ...)`, `Post`, `Put`, `Delete`, and `Do(ctx, *http.Request)`.
- `(*Client).SetOnEvent(func(ClientEvent))` observe success / retry / failure.
- `(*Client).Metrics() ClientMetrics` accumulated counts / latency.
- `WithRedirect(opts) ClientOptions` / `WithNoRedirect(opts) ClientOptions` set
  redirect handling explicitly.

## ClientOptions

- `ConnectTimeout` (default 5s), `RequestTimeout` (default 30s).
- `MaxIdleConns` (100), `IdleConnTimeout` (90s), `MaxIdlePerHost` (10).
- `RetryMax` (3), `RetryWaitMin` (100ms), `RetryWaitMax` (2s) exponential backoff.
- `FollowRedirect` (tri-state via `FollowRedirectSet`; default true).
- `EnableHTTP2` (default false; HTTP/1.1 only).
- `Breaker CircuitBreaker` — non-nil wraps every call; nil disables it. Pass a
  `*breaker.Breaker[error]` (satisfies the interface) or any implementation.
- `Latency LatencyObserver` — non-nil receives end-to-end duration; nil disables
  it (the disabled path is free). Pass a `*latency.Histogram`.

## Example

```go
import "github.com/v8fg/kit4go/httpclient"

opts := httpclient.ClientOptions{
    RetryMax:      3,
    RequestTimeout: 2 * time.Second,
}
c := httpclient.NewClient(opts)
resp, err := c.Get(ctx, "https://example.com/health")
```
