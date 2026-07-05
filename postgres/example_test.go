package postgres_test

import (
	"context"

	"github.com/v8fg/kit4go/postgres"
)

// ExampleNew shows the construction shape for the pgx-backed pool wrapper and
// the basic Ping/Close surface. The underlying *pgxpool.Pool is reachable via
// Client.Pool() for queries and transactions.
//
// This example has no // Output: comment, so go test does not execute it; it
// is a compile-checked illustration. Running it requires a PostgreSQL at
// 127.0.0.1:5432 (New dials and pings on construction).
func ExampleNew() {
	c, err := postgres.New(context.Background(), postgres.Options{
		Host:   "127.0.0.1",
		Port:   5432,
		User:   "postgres",
		DBName: "app",
	})
	if err != nil {
		return // handle connection error
	}
	defer c.Close()

	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		return // handle ping error
	}
}
