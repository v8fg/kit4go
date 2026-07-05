package redis

import (
	"testing"

	goredis "github.com/redis/go-redis/v9"
)

// closelessStub is a Cmdable that satisfies the interface via an embedded nil
// Cmdable and does NOT implement `Close() error`. Used only to drive the final
// `return nil` branch of Client.Close when own==true but the cmdable lacks Close.
type closelessStub struct {
	goredis.Cmdable
}

// TestClose_OwnedButCloseless covers the trailing `return nil` in Close: the
// client owns the cmdable (own==true), but the cmdable does not implement
// `Close() error`, so the type assertion fails and Close falls through to nil.
// This branch is unreachable through the public New (which always wires a real
// go-redis client exposing Close), so we construct the Client via the package's
// unexported fields directly (white-box test in package redis).
func TestClose_OwnedButCloseless(t *testing.T) {
	c := &Client{cmd: closelessStub{}, own: true}
	if err := c.Close(); err != nil {
		t.Fatalf("Close on owned closeless cmdable must return nil, got %v", err)
	}
}
