package etcd_test

import (
	"context"
	"log"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/v8fg/kit4go/etcd"
)

// ExampleNew shows the service-registration + discovery flow. It is a
// compile-checked illustration (an Example without an // Output: comment is
// compiled but not executed); wire your own endpoints to run it against a live
// etcd.
func ExampleNew() {
	ctx := context.Background()
	c, err := etcd.New(ctx, etcd.WithEndpoints("http://localhost:2379"))
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// Register a service instance with a lease: the key auto-expires if the
	// process stops keep-aliving (crash → key vanishes → discovery drops it).
	lease, err := c.Grant(ctx, 30) // 30s TTL
	if err != nil {
		log.Fatal(err)
	}
	if _, err := c.Put(ctx, "/services/bidder/inst-1", "10.0.0.1:8080", clientv3.WithLease(lease.ID)); err != nil {
		log.Fatal(err)
	}

	// Discover: list all bidder instances.
	resp, err := c.Get(ctx, "/services/bidder/", clientv3.WithPrefix())
	if err != nil {
		log.Fatal(err)
	}
	for _, kv := range resp.Kvs {
		_ = kv // key = instance path, value = address
	}
}
