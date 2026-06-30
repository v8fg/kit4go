package hash_test

import (
	"fmt"

	"github.com/v8fg/kit4go/hash"
)

func ExampleSHA256Hex() {
	// Fingerprint an auction ID for dedup / logging.
	fmt.Println(len(hash.SHA256Hex("auction_id=42")))
	// Output: 64
}

func ExampleHMACSHA256Hex() {
	// Sign a postback payload with a shared secret (hex form).
	sig := hash.HMACSHA256Hex([]byte("shared-secret"), []byte("conv_id=7"))
	fmt.Println(len(sig))
	// Output: 64
}

func ExampleHMACSHA256Base64() {
	// Some webhook APIs expect the HMAC-SHA256 signature in base64.
	sig := hash.HMACSHA256Base64([]byte("shared-secret"), []byte("payload"))
	fmt.Println(len(sig) > 0)
	// Output: true
}

func ExampleEqual() {
	// Constant-time signature comparison — never use == on MACs.
	a := hash.HMACSHA256([]byte("k"), []byte("d"))
	b := hash.HMACSHA256([]byte("k"), []byte("d"))
	fmt.Println(hash.Equal(a, b))
	// Output: true
}

func ExampleFNV1aString64() {
	// Consistent, cheap bucketing of a user hash into shards.
	bucket := hash.FNV1aString64("user_hash_42") % 16
	fmt.Println(bucket < 16)
	// Output: true
}

func ExampleCRC32IEEEHex() {
	// Cheap payload checksum / ETag.
	fmt.Println(len(hash.CRC32IEEEHex([]byte("payload"))))
	// Output: 8
}
