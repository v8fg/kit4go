package uuid_test

import (
	"bytes"
	"testing"

	"github.com/v8fg/kit4go/uuid"
)

// FuzzFromStringRoundTrip validates the v5 FromString parser for arbitrary
// input: either it errors (invalid format) or the parsed UUID round-trips
// (String() re-parses to the same UUID). This guards the gofrs/uuid v1→v5
// migration (import path, API change) against parsing regressions.
func FuzzFromStringRoundTrip(f *testing.F) {
	f.Add("10da441c-3870-4f06-a78c-4dfef1c9acea") // valid canonical
	f.Add("10da441c38704f06a78c4dfef1c9acea")     // valid hash-like (no dashes)
	f.Add("")                                     // empty
	f.Add("not-a-uuid")                           // invalid
	f.Add("G")                                    // single non-hex char

	f.Fuzz(func(t *testing.T, s string) {
		u, err := uuid.FromString(s)
		if err != nil {
			return // error is fine for invalid input — just ensure no panic
		}
		// Valid parse: round-trip through String() and re-parse.
		s2 := u.String()
		u2, err2 := uuid.FromString(s2)
		if err2 != nil {
			t.Fatalf("round-trip failed: FromString(%q).String()=%q re-parse error: %v", s, s2, err2)
		}
		if !uuid.Equal(u, u2) {
			t.Fatalf("round-trip mismatch: %v != %v (input=%q)", u, u2, s)
		}
	})
}

// FuzzFromBytesRoundTrip validates FromBytes for arbitrary byte slices: either
// it errors (wrong length) or the parsed UUID's Bytes() match the input exactly.
func FuzzFromBytesRoundTrip(f *testing.F) {
	f.Add(make([]byte, 16)) // valid length
	f.Add(make([]byte, 15)) // too short
	f.Add(make([]byte, 0))  // empty
	f.Add([]byte{0xff, 0xee, 0xdd, 0xcc, 0xbb, 0xaa, 0x99, 0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11, 0x00})

	f.Fuzz(func(t *testing.T, b []byte) {
		u, err := uuid.FromBytes(b)
		if err != nil {
			return // wrong length → error is correct
		}
		// Valid (16 bytes): Bytes() must reproduce the input exactly.
		got := u.Bytes()
		if !bytes.Equal(got, b) {
			t.Fatalf("round-trip mismatch: FromBytes(%v).Bytes() = %v", b, got)
		}
	})
}
