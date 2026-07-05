# featureflag

In-process feature toggle with deterministic percentage rollout (FNV hash), allowlist, time-gate, runtime Enable/Disable/SetPercentage/Allow/Revoke. Pure standard library.

## Usage

- `New(opts ...Option) *Flag` — default: disabled, 100% when enabled.
- `(*Flag).Enabled(key string) bool` — evaluate for a user/request key.
- `(*Flag).Enable()` / `.Disable()` / `.SetPercentage(p)` / `.Allow(keys...)` / `.Revoke(keys...)`.

## Example

```go
flag := featureflag.New(
    featureflag.WithEnabled(true),
    featureflag.WithPercentage(30),        // 30% rollout
    featureflag.WithAllowlist("vip-user"), // always on
)
flag.Enabled("vip-user")  // true
flag.Enabled("user-42")   // deterministic: ~30% chance, same key always same answer
```
