package etcd

// Integration test against a live etcd cluster. Skipped under -short and unless
// ETCD_ENDPOINT is set. Run locally with, e.g.:
//
//	docker run -d -p 2379:2379 -e ALLOW_NONE_AUTHENTICATION=yes bitnami/etcd
//	ETCD_ENDPOINT=http://127.0.0.1:2379 go test -run Integration -v ./etcd/

import (
	"context"
	"os"
	"testing"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestIntegration_KVLeaseRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test under -short")
	}
	endpoint := os.Getenv("ETCD_ENDPOINT")
	if endpoint == "" {
		t.Skip("ETCD_ENDPOINT not set")
	}

	ctx := context.Background()
	c, err := New(ctx, WithEndpoints(endpoint))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	if _, err := c.Put(ctx, "kit4go/it", "v1"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	resp, err := c.Get(ctx, "kit4go/it")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(resp.Kvs) != 1 || string(resp.Kvs[0].Value) != "v1" {
		t.Fatalf("Get Kvs = %+v, want one entry value v1", resp.Kvs)
	}

	// Lease: grant + Put-with-lease.
	lease, err := c.Grant(ctx, 60)
	if err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if _, err := c.Put(ctx, "kit4go/leased", "v2", clientv3.WithLease(lease.ID)); err != nil {
		t.Fatalf("Put with lease: %v", err)
	}

	if _, err := c.Delete(ctx, "kit4go/it"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	m := c.Metrics()
	if m.Puts == 0 || m.Gets == 0 || m.Grants == 0 || m.Errors != 0 {
		t.Fatalf("metrics after round-trip: %+v", m)
	}
}
