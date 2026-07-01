# postgres: pgx pool wrapper

A thin pool wrapper around pgx (pure Go, cross-platform, no cgo). `New` validates
options, builds the pool, and pings before returning. Returns typed errors for
misconfiguration (missing host / database).

## Usage

- `New(ctx context.Context, opts Options) (*Client, error)` build + health-check.
- `(*Client).Pool() *pgxpool.Pool` the underlying pool for direct queries.
- `(*Client).Close()` drain and close.

## Options

- `Host`, `Port`, `User`, `Password`, `DBName`.
- `SSLMode` (`disable`|`require`|`verify-ca`|`verify-full`; empty -> disable).
- `MaxConns` (default 10), `MinConns` (2).
- `MaxConnLifetime` (30m), `MaxConnIdleTime` (5m), `ConnectTimeout` (5s).

## Example

```go
import (
    "context"
    "github.com/v8fg/kit4go/postgres"
)

cli, err := postgres.New(ctx, postgres.Options{
    Host: "127.0.0.1", Port: 5432,
    User: "app", Password: pw, DBName: "ads",
    MaxConns: 20,
})
if err != nil { return err }
defer cli.Close()
var n int
_ = cli.Pool().QueryRow(ctx, "SELECT 1").Scan(&n)
```

Integration tests require a live Postgres; unit tests cover option validation.
