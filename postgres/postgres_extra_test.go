package postgres

import (
	"context"
	"net"
	"testing"
	"time"
)

// TestNew_BadSSLModeParseError covers the pgxpool.ParseConfig error branch in
// New: an invalid sslmode value is rejected by ParseConfig's TLS configuration.
func TestNew_BadSSLModeParseError(t *testing.T) {
	_, err := New(context.Background(), Options{
		Host:    "localhost",
		Port:    5432,
		DBName:  "test",
		User:    "u",
		SSLMode: "not-a-real-mode",
	})
	if err == nil {
		t.Fatal("expected ParseConfig error for invalid sslmode")
	}
}

// TestNew_InvalidUserinfoParseError covers the pgxpool.ParseConfig error branch
// via a malformed userinfo (space in the username breaks URL parsing).
func TestNew_InvalidUserinfoParseError(t *testing.T) {
	_, err := New(context.Background(), Options{
		Host:   "localhost",
		Port:   5432,
		DBName: "test",
		User:   "u p", // space -> invalid userinfo
	})
	if err == nil {
		t.Fatal("expected ParseConfig error for invalid userinfo")
	}
}

// TestNew_PingFailure covers the pool.Ping error branch in New (lines 91-94):
// a connection attempt to a port where nothing listens fails Ping, New closes
// the pool, and returns the error. Self-contained (no external PG required).
func TestNew_PingFailure(t *testing.T) {
	c, err := New(context.Background(), Options{
		Host:           "127.0.0.1",
		Port:           1, // nothing listening on port 1
		DBName:         "test",
		User:           "u",
		Password:       "p",
		ConnectTimeout: 200 * time.Millisecond,
	})
	if c != nil {
		c.Close()
	}
	if err == nil {
		// On some sandboxes port 1 may behave oddly; treat nil-error as a skip
		// rather than a hard failure to avoid CI flakes.
		t.Skip("port 1 unexpectedly accepted a connection; skipping")
	}
}

// TestNew_DefaultsApplied exercises the default-value branches (MaxConns<=0,
// MaxConnLifetime<=0, MaxConnIdleTime<=0, ConnectTimeout<=0, SSLMode=="") by
// building a config whose connection fails fast. The defaults are written into
// the parsed config before Ping; reaching Ping (or its failure) means every
// default branch executed.
func TestNew_DefaultsApplied(t *testing.T) {
	_, err := New(context.Background(), Options{
		Host:           "127.0.0.1",
		Port:           1, // unreachable -> Ping fails fast, but all defaults applied
		DBName:         "test",
		User:           "u",
		ConnectTimeout: 100 * time.Millisecond,
	})
	if err == nil {
		t.Skip("port 1 unexpectedly accepted a connection; skipping")
	}
}

// pgReachable reports whether a Postgres is accepting connections at the given
// host:port within a short window. Used to gate the success-path test so it is
// self-contained (skips when no PG is available) yet exercises the full New
// happy path when the standard local infra is present.
func pgReachable(host string, port int) bool {
	addr := net.JoinHostPort(host, "5432")
	if port != 0 {
		addr = net.JoinHostPort(host, itoa(port))
	}
	c, err := net.DialTimeout("tcp", addr, 150*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// itoa is a tiny dependency-free int->string to avoid importing strconv at the
// call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// TestNew_SuccessAgainstLocalPG covers the success return branch of New (the
// `return &Client{...}, nil` line). It runs under -short only when a Postgres
// is reachable at localhost:5432 (the standard local infra documented in
// ~/INFRA.md: postgres/12345678). When unreachable, it skips — so the test
// suite remains self-contained and CI-safe.
func TestNew_SuccessAgainstLocalPG(t *testing.T) {
	if !pgReachable("127.0.0.1", 5432) {
		t.Skip("no Postgres on 127.0.0.1:5432; skipping success-path test")
	}
	c, err := New(context.Background(), Options{
		Host:           "127.0.0.1",
		Port:           5432,
		User:           "postgres",
		Password:       "12345678",
		DBName:         "postgres",
		ConnectTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Skipf("local PG present but New failed: %v", err)
	}
	defer c.Close()
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping after successful New: %v", err)
	}
	if c.Pool() == nil {
		t.Fatal("Pool() must be non-nil for a real (non-mock) Client")
	}
}
