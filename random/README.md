# random: pseudo-random & cryptographically secure random

Pseudo-random number and string primitives for non-security workloads, plus a
crypto-strong subset for tokens and verification codes. Pure standard library.

## Sources

The package exposes two distinct sources, picked by use case:

- **Pseudo-random** (the bulk of the API) delegates to the `math/rand/v2`
  package-level functions, which use a concurrent-safe global source. Safe to
  call from many goroutines. Fast; NOT for security.
- **Cryptographically secure** (`Crypto*`) delegates to `crypto/rand`. Slower;
  use for tokens, session IDs, 2FA seeds, and anything an attacker must not
  predict.

## Why math/rand/v2

The earlier implementation held a shared `*rand.Rand`. That type is not safe for
concurrent use and panics (`index out of range [-1]`) under parallel load.
`math/rand/v2` package-level functions are concurrency-safe and auto-seeded, so
`Seed` / `SeedReset` are retained as no-ops for backward compatibility.

## Usage

### numbers (pseudo-random)

- `Int() int` non-negative int.
- `IntBetween(min, max int) int` int in `[min, max)`.
- `Int31() int32`, `Int31Between(min, max int32) int32` 31-bit.
- `Int63() int64`, `Int63Between(min, max int64) int64` 63-bit.
- `Uint32() uint32`, `Uint64() uint64`.
- `Float32() float32`, `Float32Between(min, max float32) float32`.
- `Float64() float64`, `Float64Between(min, max float64) float64`.
- `NormFloat64() float64` standard normal (mean 0, stddev 1).
- `ExpFloat64() float64` exponentially distributed (rate 1).
- `Percent() float64` in `[0, 100.0]`.
- `Perm(n int) []int` permutation of `[0, n)`.
- `PermBetween(min, max int) []int` permutation of `[min, max)`.
- `Shuffle(n int, swap func(i, j int))` Fisher-Yates.
- `Read(p []byte) (n int, err error)` fills `p` from `crypto/rand` (kept under
  the random namespace; the non-security `math/rand` source has no package-level
  `Read` in v2).

### strings (pseudo-random)

- `RandStringWithLetter(n int) string` `[a-zA-Z]`.
- `RandStringWithLetterDigits(n int) string` `[0-9a-zA-Z]`.
- `RandStringInCharset(n int, charset []rune) string` arbitrary runes (1-4 bytes each).
- `RandStringWithKind(n int, kind int) []byte` bitmask of digit/upper/lower.
- `StringByRead(b []byte) string` base64 of `crypto/rand` bytes, length `len(b)`.
- `NumericCode(n int) string` n-digit string, leading zeros allowed (SMS/email codes).
- `RandUniCodeByUID(uid uint64, n int) string` deterministic-looking code from a
  UID (n < 10); see the diffusion/confusion comment in source.
- `RandUniCodeByUIDWithSalt(uid uint64, n int, salt uint64) string` same, with caller salt.

### sampling (pseudo-random)

- `RandIn[T any](slice []T) T` one element (panics on empty).
- `RandNIn[T any](n int, slice []T) []T` n distinct elements.

### cryptographically secure

- `CryptoInt(max int64) (*big.Int, error)` uniform in `[0, max)`.
- `CryptoPrime(bits int) (*big.Int, error)` probable prime of bit length `bits`.
- `CryptoRead(b []byte) (n int, err error)` fills `b`.
- `CryptoReadString(b []byte) string` base64 of `len(b)` crypto bytes.

## Examples

```go
import "github.com/v8fg/kit4go/random"

code := random.NumericCode(6)               // "048213" — SMS OTP
id    := random.RandStringWithLetterDigits(16)
pick  := random.RandIn([]string{"a", "b", "c"})
tok, _ := random.CryptoInt(1 << 62)         // secure range-limited int
```

## Notes

- For 2FA (time/counter based) use package `otp` (TOTP/HOTP), not this package.
- `NumericCode` and the `RandString*` family are pseudo-random; do not use them
  for security tokens. Prefer `CryptoReadString` / `CryptoInt`.
