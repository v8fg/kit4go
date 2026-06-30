# config

Typed configuration from layered, read-only sources: environment variables, flat
JSON files, and programmatic maps. Pure standard library.

## Why

A 12-factor service reads its config from env vars; a local dev run reads a
small JSON file; tests inject values directly. This package unifies all three
behind one `Store` with a clear priority order and typed getters that never
panic — they fall back to a caller-supplied default.

## Sources

| Source | Reads |
|---|---|
| `config.Env(prefix)` | process env; `"redis.addr"` + prefix `"app"` → `APP_REDIS_ADDR` |
| `config.FromFile(path)` | flat JSON `{"key.path": "value", ...}` |
| `config.MapSource{...}` | in-memory map (defaults, test fixtures) |

## Priority

`New(sources...)` — the **first source that has the key wins**. So list them
highest-priority first:

```go
store := config.New(
	config.Env("app"),                  // highest: prod / 12-factor
	fileSource,                         // file: local dev override
	config.MapSource{"log.level": "info"}, // lowest: defaults
)
```

## Typed getters

All getters return a default when the key is missing or fails to parse.

```go
store.String(key, def)               // string
store.Int(key, def), store.Int64(key, def)
store.Bool(key, def)                 // 1/0, t/f, true/false, yes/no, on/off (case-insensitive)
store.Float64(key, def)
store.Duration(key, def)             // "250ms", "2s", "1m30s"
store.StringSlice(key, sep, def)     // empty fields dropped
store.IntSlice(key, sep, def)        // any field unparseable -> def
store.Unmarshal(key, &dst)           // JSON value into a struct; ErrMissing if absent
store.Has(key)                       // present in any source?
```

## Ad-tech uses

- SSP endpoints, bidder timeouts, pacing limits, feature flags — values that
  differ per environment.
- A `MapSource` of defaults means a missing env var degrades safely instead of
  crashing the bidder on startup.
- `Unmarshal` loads a structured sub-config (e.g. a per-SSP tuning block) from a
  single JSON env value.

## Testing

92% statement coverage, `-race` clean. Covers env normalization (dots/dashes/
case), file happy/bad-path/bad-JSON, priority overlay (first source wins),
every typed getter, bool variants, default-on-missing/parse-error, slices,
`Unmarshal` (ok / `ErrMissing` / bad JSON), and an empty store.

```bash
go test -race -cover ./config/...
```
