# base62

Encodes and decodes unsigned integers as base-62 strings (`0-9 A-Z a-z`) — the
standard "short code" encoding for URL shorteners, tracking/click URLs, and
shareable IDs. Pure standard library, zero dependencies.

## Why

Every short-link service does the same thing: turn an auto-increment integer ID
into a compact, URL-safe slug, and turn it back. That round-trip is the one
reusable primitive; the long↔short mapping storage, redirect, and counter are
application-level and live elsewhere. base62 is the codec.

## API

```go
slug := base62.Encode(123456789)      // "8M0kX"
id, err := base62.Decode("8M0kX")     // 123456789, nil

// Custom alphabet (must be 62 unique bytes):
enc := base62.EncodeWithAlphabet(id, custom)
id, _ = base62.DecodeWithAlphabet(enc, custom)
```

| Symbol | Behavior |
|---|---|
| `Encode(id) string` | uint64 → base-62 slug (default `Alphabet`); `0` → `"0"` |
| `Decode(s) (uint64, error)` | slug → uint64; `ErrInvalid` for empty/unknown chars |
| `EncodeWithAlphabet` / `DecodeWithAlphabet` | Same with a custom 62-byte alphabet |
| `Alphabet` | `0123...XYZ...abc...z` |

A uint64 encodes to at most 11 chars — short and URL-safe (no `+/=`).

## Ad-tech use

- **Tracking / click URLs** — encode a placement or click ID as a short slug
  instead of a long numeric param.
- **Short links** — auto-increment ID → slug; store long↔slug in your DB/Redis
  (`redislock` + `rate` from this repo pair well for a short-link service).
- **Shareable IDs** — compact, non-obvious sequential IDs.

## Testing

97% statement coverage, `-race` clean. Known small vectors, round-trips over the
full uint64 range (incl. 0 and max), compactness, invalid-character and
empty-string rejection, custom-alphabet round-trip, and alphabet validation
(wrong length / duplicate bytes). Also paired with `random.NumericCode` for
verification-code generation.

```bash
go test -race -cover ./base62/...
```
