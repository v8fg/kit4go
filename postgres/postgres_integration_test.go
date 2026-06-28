package postgres

import (
	"os"
	"testing"
)

// Test_Integration_NewPingClose exercises the real success path against a live
// Postgres. Skipped under -short and when PG_HOST is unset. Run locally with:
//   PG_HOST=localhost PG_USER=postgres PG_PASS=postgres PG_DB=test go test ./postgres/
func Test_Integration_NewPingClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	host := os.Getenv("PG_HOST")
	if host == "" {
		t.Skip("PG_HOST not set (set PG_HOST/PG_USER/PG_PASS/PG_DB to run)")
	}
	c, err := New(t.Context(), Options{
		Host:     host,
		Port:     5432,
		User:     os.Getenv("PG_USER"),
		Password: os.Getenv("PG_PASS"),
		DBName:   os.Getenv("PG_DB"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}
}
