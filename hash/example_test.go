package hash_test

import (
	"fmt"

	"github.com/v8fg/kit4go/hash"
)

func ExampleSHA256Hex() {
	// SHA256Hex is the workhorse: lowercase hex digest of a string, useful for
	// fingerprinting auction/bid IDs or deriving idempotency keys. The empty
	// string yields the well-known NIST empty-input digest.
	fmt.Println(hash.SHA256Hex(""))
	fmt.Println(hash.SHA256Hex("hello"))
	// Output:
	// e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	// 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
}

func ExampleHMACSHA256Hex() {
	// HMACSHA256Hex signs data under a shared secret — the standard choice for
	// postbacks, MMP callbacks, and webhook payloads. Pair it with Equal when
	// verifying an inbound signature: never compare MACs with ==.
	sig := hash.HMACSHA256Hex([]byte("secret"), []byte("payload"))
	fmt.Println(sig)

	want := hash.HMACSHA256Hex([]byte("secret"), []byte("payload"))
	fmt.Println(hash.Equal([]byte(sig), []byte(want)))
	// Output:
	// b82fcb791acec57859b989b430a826488ce2e479fdf92326bd0a2e8375a42ba4
	// true
}
