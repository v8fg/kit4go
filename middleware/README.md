# middleware

Composable HTTP middleware for `net/http` / `httpserver`. Each is a `func(http.Handler) http.Handler`. Pure standard library.

## Usage

- `RequestID(http.Handler)` — generate (CSPRNG hex) or propagate `X-Request-ID`; inject into context.
- `RateLimit(allow AllowFunc, retryAfter int) func(http.Handler) http.Handler` — 429 on reject.
- `CORS(cfg CORSConfig) func(http.Handler) http.Handler` — preflight + headers; spec-compliant credentials handling.
- `FromContext(ctx) string` — extract request ID set by RequestID.

## Example

```go
handler := middleware.RequestID(
    middleware.RateLimit(limiter.Allow, 5)(
        middleware.CORS(middleware.CORSConfig{
            AllowOrigins: []string{"https://app.example.com"},
        })(myHandler)))
http.ListenAndServe(":8080", handler)
```
