package clickhouse_test

import (
	"context"

	"github.com/v8fg/kit4go/clickhouse"
)

// ExampleNew shows the construction shape. The native protocol (default) uses
// port 9000; the address carries the protocol's port.
//
// This example has no // Output: comment, so go test does not execute it; it
// is a compile-checked illustration. Running it requires a ClickHouse at
// 127.0.0.1:9000 (New dials and pings on construction).
func ExampleNew() {
	c, err := clickhouse.New(context.Background(),
		clickhouse.WithAddrs("127.0.0.1:9000"),
		clickhouse.WithDatabase("default"),
		clickhouse.WithMaxOpenConns(10),
	)
	if err != nil {
		return // handle connection error
	}
	defer c.Close()

	ctx := context.Background()
	_ = c.Exec(ctx, "CREATE TABLE events (id UInt64, ts DateTime) Engine=Memory")

	batch, _ := c.PrepareBatch(ctx, "INSERT INTO events (id, ts)")
	_ = batch.Append(uint64(1))
	_ = batch.Send()
}
