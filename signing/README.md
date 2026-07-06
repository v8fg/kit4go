# signing

HMAC-SHA256 request signing for API authentication. The package canonicalizes a
set of request parameters, binds them to a signing timestamp, and signs the
canonical string under a shared secret. `Verify` recomputes the signature and
checks the timestamp is within a max age, defeating replay. Pure standard
library.

Distinct from the `hash` package, which performs content hashing (digesting a
byte payload with a fixed algorithm). This package authenticates a request:
its canonical string is built from sorted, `QueryEscape`'d parameters so a
value containing `&b=2` cannot masquerade as a second parameter
(parameter-injection resistance).

## Canonical string

Parameters sorted by key (excluding the signature and timestamp keys), joined
as `k1=v1&k2=v2&...&<TimestampKey>=<unix-seconds>`, then
`HMAC-SHA256(secret, string)`, hex-encoded. Keys and values are
`url.QueryEscape`'d so the separators `&` and `=` are unambiguous. The
timestamp is carried in the parameter set under `TimestampKey = "_ts"` so the
receiver can verify it.

## API

| Symbol | Behavior |
|---|---|
| `Sign(params, secret, opts...) (string, error)` | Compute hex HMAC-SHA256; does not mutate `params` |
| `Verify(params, secret, signature, opts...) bool` | Recompute, constant-time compare, freshness check |
| `WithTimestamp(ts)` | Sign: signing instant (default `time.Now`); for deterministic tests |
| `WithMaxAge(d)` | Verify: freshness window (default `DefaultMaxAge` = 5m; 0 disables the check) |
| `WithNow(fn)` | Verify: injected clock for deterministic expiry tests |
| `SignatureKey = "_sig"` | Parameter name carrying the signature; excluded from the canonical string |
| `TimestampKey = "_ts"` | Parameter name carrying the signing timestamp; included in the canonical string |

`Verify` returns `false` on a missing `TimestampKey`, a malformed timestamp,
an out-of-window timestamp (too old or in the future), or any HMAC mismatch.

## Example

```go
params := map[string]string{
    "auction_id": "42",
    "bidder":     "acme",
    "price":      "1.25",
}
sig, _ := signing.Sign(params, "shared-secret",
    signing.WithTimestamp(time.Unix(1_700_000_000, 0)))

// Receiver side: rebuild params with the embedded _ts, recompute, check
// freshness. WithNow makes the check deterministic.
recv := map[string]string{
    "auction_id":         "42",
    "bidder":             "acme",
    "price":              "1.25",
    signing.TimestampKey: "1700000000",
}
ok := signing.Verify(recv, "shared-secret", sig,
    signing.WithNow(func() time.Time { return time.Unix(1_700_000_010, 0) }))
fmt.Println(ok) // true
```

## Testing

Pure standard library; no external services. Deterministic signing vectors,
constant-time verify, freshness window (stale, future, boundary), missing or
malformed timestamp, parameter-injection resistance, and option seams
(`WithTimestamp`, `WithNow`, `WithMaxAge`).

```bash
go test -race -cover ./signing/...
```
