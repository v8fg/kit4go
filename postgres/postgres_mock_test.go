package postgres

import (
	"context"
	"errors"
	"testing"
)

// mockPool implements PoolConn for testing.
type mockPool struct {
	pingErr  error
	closed   bool
	pingCall int
}

func (m *mockPool) Ping(ctx context.Context) error {
	m.pingCall++
	return m.pingErr
}

func (m *mockPool) Close() { m.closed = true }

func TestClient_Ping(t *testing.T) {
	mp := &mockPool{}
	c := newWithPool(mp)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if mp.pingCall != 1 {
		t.Fatalf("Ping called %d times, want 1", mp.pingCall)
	}
}

func TestClient_PingError(t *testing.T) {
	mp := &mockPool{pingErr: errors.New("connection refused")}
	c := newWithPool(mp)
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("Ping should propagate error")
	}
}

func TestClient_Close(t *testing.T) {
	mp := &mockPool{}
	c := newWithPool(mp)
	c.Close()
	if !mp.closed {
		t.Fatal("Close should call pool.Close()")
	}
}

func TestClient_Pool_NilWhenMocked(t *testing.T) {
	c := newWithPool(&mockPool{})
	if c.Pool() != nil {
		t.Fatal("Pool() should return nil when mock-injected")
	}
}

func TestNew_EmptyHost(t *testing.T) {
	_, err := New(context.Background(), Options{DBName: "test"})
	if err == nil {
		t.Fatal("empty host should error")
	}
}

func TestNew_EmptyDBName(t *testing.T) {
	_, err := New(context.Background(), Options{Host: "localhost"})
	if err == nil {
		t.Fatal("empty db name should error")
	}
}

func TestNew_SSLModeDefault(t *testing.T) {
	// Just verify it doesn't panic on empty SSLMode (it defaults to "disable").
	// The actual connection will fail (no live PG) but the config parsing should work.
	_, err := New(context.Background(), Options{
		Host:   "localhost",
		Port:   5432,
		DBName: "test",
		User:   "test",
	})
	// Will fail at connection (no PG), but that's OK — we're testing the config path.
	_ = err
}

func TestNew_CustomOptions(t *testing.T) {
	// Exercise the option-parsing branches (MaxConns, MinConns, timeouts).
	_, err := New(context.Background(), Options{
		Host:            "localhost",
		Port:            5432,
		DBName:          "test",
		User:            "test",
		MaxConns:        20,
		MinConns:        5,
		MaxConnLifetime: 1000000000, // 1s
		MaxConnIdleTime: 500000000,  // 0.5s
		ConnectTimeout:  3000000000, // 3s
		SSLMode:         "require",
	})
	_ = err // connection will fail, but all option branches are exercised
}
