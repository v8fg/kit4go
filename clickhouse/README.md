# clickhouse

A thin, option-configured wrapper around
[`github.com/ClickHouse/clickhouse-go/v2`](https://github.com/ClickHouse/clickhouse-go),
the official ClickHouse driver. It provides ergonomic construction, a health
check, pass-through query execution, an escape hatch to the full driver, light
metrics, and a graceful Close — and nothing more (no query builder, no domain
types).

## Why

ClickHouse is the column-store OLAP workhorse for event logs, aggregations, and
time-series. Like the `redis`/`postgres` wrappers, this package keeps the boring
parts (options, defaults, ping, lifecycle, observability) in one place so call
sites stay tidy, while leaving the driver API untouched for anything advanced.

## Usage

```go
import (
    "context"

    "github.com/v8fg/kit4go/clickhouse"
)

func main() {
    ctx := context.Background()

    // Native protocol (default) — port 9000 (HTTP: 8123). WithAddrs carries
    // the protocol's port.
    c, err := clickhouse.New(ctx,
        clickhouse.WithAddrs("127.0.0.1:9000"),
        clickhouse.WithDatabase("default"),
    )
    if err != nil {
        panic(err)
    }
    defer c.Close()

    if err := c.Exec(ctx, "CREATE TABLE events (id UInt64, ts DateTime) Engine=Memory"); err != nil {
        panic(err)
    }

    // Bulk insert — the #1 ClickHouse operation.
    batch, err := c.PrepareBatch(ctx, "INSERT INTO events (id, ts)")
    if err != nil {
        panic(err)
    }
    _ = batch.Append(uint64(1))
    _ = batch.Send()

    var n uint64
    _ = c.QueryRow(ctx, "SELECT count() FROM events").Scan(&n)

    // Pool stats + counters.
    _ = c.Stats()
    _ = c.Metrics()
}
```

## Options

| Option | Default | Notes |
|--------|---------|-------|
| `WithAddrs` | required | `host:9000` (native) or `host:8123` (HTTP) |
| `WithProtocol` | `ProtocolNative` | `ProtocolHTTP` behind a proxy/LB |
| `WithDatabase` | `"default"` | |
| `WithUsername` / `WithPassword` | empty | valid for a no-auth server |
| `WithTLSConfig` | nil (plaintext) | |
| `WithDialTimeout` / `WithMaxOpenConns` / `WithMaxIdleConns` / `WithConnMaxLifetime` | driver defaults | 30s / `idle+5` / 5 / 1h |
| `WithSettings` | nil | pass-through query settings |
| `WithCompression` | nil (none) | recommended in production: `&clickhouse.Compression{Method: clickhouse.CompressionLZ4}` |
| `WithConnOpenStrategy` | `ConnOpenInOrder` | `ConnOpenRoundRobin` for multi-node |
| `WithDebug` | false | deprecated upstream (prefer the driver's slog `Logger`) |

Zero-valued tuning fields defer to clickhouse-go's own defaults — the wrapper
never overrides them.

## Surface

- Lifecycle: `New`, `Wrap` (adopt a conn; Close is a no-op), `Close`, `Ping`.
- Pass-through: `Exec`, `Query`, `QueryRow`, `PrepareBatch` (returns the driver's
  `Batch`; call `Append`/`Send`/`Close` exactly as upstream documents).
- Escape hatch: `Conn()` — the raw `driver.Conn` for `Select`, `AsyncInsert`,
  `ServerVersion`, etc. (nil when the client is mock-injected for tests).
- Observability: `Metrics()` (per-op atomic counters), `SetOnEvent` (nil hook =
  one atomic load, ~zero overhead), `Stats()` (driver pool counters).

## Protocol & ports

The wrapper does **not** remap ports. Native uses the columnar TCP protocol on
`:9000` (TLS `:9440`); HTTP uses `:8123` (TLS `:8443`). Pass the matching port in
`WithAddrs` for the chosen `Protocol`.

## Testing

No live server is required for unit tests — they inject a mock `Conn` through
the package's internal interface. A real end-to-end test is env-gated:

```sh
docker run -d -p 9000:9000 --name ch clickhouse/clickhouse-server
CLICKHOUSE_HOST=127.0.0.1 CLICKHOUSE_DB=default \
    go test -run Integration -v ./clickhouse/
```
