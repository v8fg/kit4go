# hash

Ergonomic wrappers around the standard library's `crypto/*`, `hash/fnv`, and
`hash/crc` packages. Zero third-party dependencies — pure stdlib.

Returns raw digest bytes for composability, with hex / base64 convenience
helpers for the common "hash a string and format it" case. Every function
allocates a fresh hasher per call, so all are safe for concurrent use.

## Why

Real services reach for the same handful of hashes repeatedly. This package
gives them stable names and the two return shapes people actually want (raw
bytes + hex string), without dragging in a crypto framework.

## API

### Cryptographic digests

```go
hash.MD5(data)     // []byte (16)  — checksums/ETags only; MD5 is broken
hash.SHA1(data)    // []byte (20)  — collision-broken; prefer SHA-256
hash.SHA224(data)  // []byte (28)
hash.SHA256(data)  // []byte (32)  — the workhorse
hash.SHA384(data)  // []byte (48)
hash.SHA512(data)  // []byte (64)

hash.SHA256Hex("auction_id=42")  // lowercase hex string
hash.MD5Hex(""), hash.SHA1Hex(s), hash.SHA512Hex(s)
```

### HMAC (signing)

```go
hash.HMACSHA256(key, data)         // []byte (32) — postbacks, webhooks, MMP callbacks
hash.HMACSHA1(key, data), hash.HMACSHA512(key, data)

hash.HMACSHA256Hex(key, data)      // hex form
hash.HMACSHA256Base64(key, data)   // base64 form (some webhooks expect this)

hash.Equal(a, b)                   // constant-time compare; never use == on MACs
```

### FNV — fast, non-cryptographic, deterministic

For consistent bucketing / sharding of keys where a crypto hash is wasted cost.

```go
hash.FNV1a32(data)   // uint32 — empty input is the offset basis 0x811c9dc5
hash.FNV1a64(data)   // uint64
hash.FNV132(data), hash.FNV164(data)              // FNV-1 (multiply-then-xor) variants
hash.FNV1aString64("user_hash_42")                // string-keyed convenience

bucket := hash.FNV1aString64(userHash) % shardCount
```

### CRC — cheap checksums

For payload validation, ETags, change detection. Never for security.

```go
hash.CRC32IEEE(data)        // uint32 — "123456789" → 0xcbf43926
hash.CRC32IEEEHex(data)     // 8-char lowercase hex
hash.CRC64ISO(data), hash.CRC64ECMA(data)
```

## Ad-tech uses

- **HMAC-SHA256** signs postbacks, MMP callbacks, and SSP webhook payloads.
- **SHA-256** fingerprints auction / bid IDs for dedup and idempotency keys.
- **FNV** buckets a user hash into bidder shards or frequency-cap windows
  cheaply and deterministically.
- **CRC32** produces cheap payload checksums / ETags.

## Testing

100% statement coverage, `-race` clean. Known vectors (NIST FIPS, RFC 1321,
RFC 4231), cross-checks against the stdlib reference, and concurrent-call
invariants for every family.

```bash
go test -race -cover ./hash/...
```
