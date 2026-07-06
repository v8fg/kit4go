package signing_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/signing"
)

// ExampleSign shows signing a request's parameters with a shared secret. The
// timestamp is injected via WithTimestamp so the example is deterministic.
func ExampleSign() {
	params := map[string]string{
		"auction_id": "42",
		"bidder":     "acme",
		"price":      "1.25",
	}
	sig, _ := signing.Sign(params, "shared-secret",
		signing.WithTimestamp(time.Unix(1_700_000_000, 0)))
	fmt.Println(sig)
	// Output: f10a32b28703e3afba7449726bc66f42de2592280ce5fc5ef30bdebe59d02be1
}

// ExampleVerify shows the receiver side: rebuild the params with the embedded
// _ts, recompute, and check freshness. WithNow makes the example deterministic.
func ExampleVerify() {
	// Params as received (carrier added the _ts entry from the signed payload).
	params := map[string]string{
		"auction_id":         "42",
		"bidder":             "acme",
		"price":              "1.25",
		signing.TimestampKey: "1700000000",
	}
	sig := "f10a32b28703e3afba7449726bc66f42de2592280ce5fc5ef30bdebe59d02be1"
	ok := signing.Verify(params, "shared-secret", sig,
		signing.WithNow(func() time.Time { return time.Unix(1_700_000_010, 0) }))
	fmt.Println(ok)
	// Output: true
}
