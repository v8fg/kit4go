package hash

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// FuzzHMACEqual checks the contract: HMACSHA256(key, a) equals
// HMACSHA256(key, b) if and only if a == b.
//
// The forward implication (a == b -> equal) is trivially true for any hash and
// is asserted to guard against accidental keying/state corruption in the
// wrapper. The reverse implication (equal -> a == b) relies on the collision
// resistance of HMAC-SHA256; a fuzzer-generated collision would constitute a
// real cryptographic break, so the assertion is deterministic and never flaky
// in practice.
//
// The same key is reused for both HMAC computations, exercising keys of
// arbitrary length (shorter than the block size, equal to it, and far longer,
// which forces the key-hashing path inside hmac.New).
func FuzzHMACEqual(f *testing.F) {
	// Seeds: (key, a, b). Include equal, differing, empty, and oversized cases.
	f.Add([]byte("k"), []byte("d"), []byte("d"))
	f.Add([]byte("k"), []byte("d"), []byte("x"))
	f.Add([]byte(""), []byte(""), []byte(""))
	f.Add([]byte("shared-secret"), []byte("conv_id=7"), []byte("conv_id=7"))
	f.Add(make([]byte, 256), []byte("a"), []byte("b")) // block-sized+ key

	f.Fuzz(func(t *testing.T, key, a, b []byte) {
		ma := HMACSHA256(key, a)
		mb := HMACSHA256(key, b)
		gotEqual := Equal(ma, mb)
		inputEqual := bytes.Equal(a, b)

		// Forward: identical inputs must yield identical MACs.
		if inputEqual && !gotEqual {
			t.Fatalf("HMAC differs for identical inputs: key=%x a=%x", key, a)
		}
		// Reverse: distinct inputs must yield distinct MACs (collision resistance).
		if !inputEqual && gotEqual {
			t.Fatalf("HMAC collision for distinct inputs: key=%x a=%x b=%x", key, a, b)
		}

		// Cross-check the raw-byte comparison against the hex form, which must
		// agree (hex encoding is injective, so equal hex iff equal bytes).
		ha := HMACSHA256Hex(key, a)
		hb := HMACSHA256Hex(key, b)
		if (ha == hb) != gotEqual {
			t.Fatalf("hex equality (%v) disagrees with raw equality (%v)", ha == hb, gotEqual)
		}

		// Fixed-length guarantee regardless of input length.
		if len(ma) != 32 || len(mb) != 32 {
			t.Fatalf("HMAC-SHA256 length changed: len(ma)=%d len(mb)=%d", len(ma), len(mb))
		}
	})
}

// FuzzMD5Consistency checks that MD5 is a pure function of its input: the same
// bytes always produce the same digest, in both raw and hex form, and across
// repeated calls. MD5 is deterministic by construction, so this never flakes.
func FuzzMD5Consistency(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("a"))
	f.Add([]byte("auction_id=42"))
	f.Add([]byte("The quick brown fox jumps over the lazy dog"))
	f.Add(make([]byte, 4096)) // large input

	f.Fuzz(func(t *testing.T, data []byte) {
		first := MD5(data)
		second := MD5(data)

		// Repeated calls over identical input must be byte-identical.
		if !bytes.Equal(first, second) {
			t.Fatalf("MD5 is non-deterministic for input %x: %x vs %x", data, first, second)
		}

		// Fixed 16-byte digest length for every input.
		if len(first) != 16 {
			t.Fatalf("MD5 length changed: got %d, want 16 for input %x", len(first), data)
		}

		// Hex helper must match hex encoding of the raw digest.
		wantHex := hex.EncodeToString(first)
		gotHex := MD5Hex(string(data))
		if gotHex != wantHex {
			t.Fatalf("MD5Hex(%x) = %q, want %q", data, gotHex, wantHex)
		}

		// Hex length is always 32 lowercase chars.
		if len(gotHex) != 32 {
			t.Fatalf("MD5Hex length changed: got %d, want 32 for input %x", len(gotHex), data)
		}
	})
}
