package aerospike

// Integration test against a live Aerospike cluster. Skipped under -short and
// unless AEROSPIKE_HOST is set. Run locally with, e.g.:
//
//	docker run -d -p 3000:3000 -p 3001:3001 -p 3002:3002 aerospike/aerospike-server
//	AEROSPIKE_HOST=127.0.0.1 go test -run Integration -v ./aerospike/

import (
	"os"
	"testing"

	as "github.com/aerospike/aerospike-client-go/v8"
)

func TestIntegration_KVRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test under -short")
	}
	host := os.Getenv("AEROSPIKE_HOST")
	if host == "" {
		t.Skip("AEROSPIKE_HOST not set")
	}

	c, err := newClient(host, 3000, nil, defaultOpener)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	defer c.Close()

	key, err := as.NewKey("test", "kit4go", "pk")
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}

	if err := c.Put(nil, key, as.BinMap{"v": "v1"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	rec, err := c.Get(nil, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Bins["v"] != "v1" {
		t.Fatalf("Get Bins = %+v, want v=v1", rec.Bins)
	}
	if _, err := c.Delete(nil, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	m := c.Metrics()
	if m.Puts == 0 || m.Gets == 0 || m.Deletes == 0 || m.Errors != 0 {
		t.Fatalf("metrics after round-trip: %+v", m)
	}
}
