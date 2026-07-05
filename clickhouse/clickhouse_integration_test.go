package clickhouse_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/clickhouse"
)

// TestIntegration_LiveRoundTrip exercises the real native protocol end-to-end.
// Skipped under -short and when CLICKHOUSE_HOST is unset.
//
//	docker run -d -p 9000:9000 --name ch clickhouse/clickhouse-server
//	CLICKHOUSE_HOST=127.0.0.1 CLICKHOUSE_DB=default go test -run Integration -v ./clickhouse/
func TestIntegration_LiveRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	host := os.Getenv("CLICKHOUSE_HOST")
	if host == "" {
		t.Skip("CLICKHOUSE_HOST not set (set CLICKHOUSE_HOST[/PORT/DB/USER/PASS] to run)")
	}
	port := os.Getenv("CLICKHOUSE_PORT")
	if port == "" {
		port = "9000"
	}
	db := os.Getenv("CLICKHOUSE_DB")
	if db == "" {
		db = "default"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := clickhouse.New(ctx,
		clickhouse.WithAddrs(host+":"+port),
		clickhouse.WithDatabase(db),
		clickhouse.WithUsername(os.Getenv("CLICKHOUSE_USER")),
		clickhouse.WithPassword(os.Getenv("CLICKHOUSE_PASS")),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	require.NoError(t, c.Ping(ctx))

	require.NoError(t, c.Exec(ctx, "DROP TABLE IF EXISTS kit4go_integration"))
	require.NoError(t, c.Exec(ctx, "CREATE TABLE kit4go_integration (id UInt64) Engine=Memory"))

	batch, err := c.PrepareBatch(ctx, "INSERT INTO kit4go_integration (id)")
	require.NoError(t, err)
	require.NoError(t, batch.Append(uint64(1)))
	require.NoError(t, batch.Append(uint64(2)))
	require.NoError(t, batch.Send())

	var n uint64
	require.NoError(t, c.QueryRow(ctx, "SELECT count() FROM kit4go_integration").Scan(&n))
	require.Equal(t, uint64(2), n)
}
