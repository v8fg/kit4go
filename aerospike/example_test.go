package aerospike_test

import (
	"log"

	as "github.com/aerospike/aerospike-client-go/v8"

	"github.com/v8fg/kit4go/aerospike"
)

// ExampleNew shows connect + Put/Get. It is a compile-checked illustration (an
// Example without an // Output: comment is compiled but not executed); wire your
// own host/port to run it against a live cluster.
func ExampleNew() {
	c, err := aerospike.New("localhost", 3000)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	key, err := as.NewKey("profiles", "user", "u-42")
	if err != nil {
		log.Fatal(err)
	}

	// Write a record (BinMap = map[string]any of bins).
	if err := c.Put(nil, key, as.BinMap{"segment": "auto", "freq": 3}); err != nil {
		log.Fatal(err)
	}

	// Read it back (nil policy = defaults; no binNames = all bins).
	rec, err := c.Get(nil, key)
	if err != nil {
		log.Fatal(err)
	}
	_ = rec // rec.Bins is a map[string]any
}
