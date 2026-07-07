package base62

import (
	"testing"
)

// FuzzEncodeDecodeRoundtrip asserts the core round-trip invariant of the
// package: for every uint64, Encode followed by Decode must reproduce the
// original value with no error. This covers the default-alphabet fast path
// (Decode uses the package-init defaultDecodeTable).
//
// Seeds span the interesting boundaries: zero, single-digit values, the
// base-62 radix boundary, large powers of two, and both 64-bit extremes.
func FuzzEncodeDecodeRoundtrip(f *testing.F) {
	seeds := []uint64{
		0,
		1,
		2,
		61, // last single-digit index
		62, // first two-digit value
		63,
		123,
		999,
		1000,
		1 << 10,
		1<<31 - 1,
		1 << 32,
		123456789012345,
		1<<63 - 1,
		1 << 63,
		^uint64(0), // max uint64
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, id uint64) {
		enc := Encode(id)
		if len(enc) == 0 {
			t.Fatalf("Encode(%d) returned empty string", id)
		}
		// A base-62 encoding of a uint64 never exceeds 11 characters.
		if len(enc) > 11 {
			t.Fatalf("Encode(%d) = %q: len %d exceeds 11", id, enc, len(enc))
		}
		got, err := Decode(enc)
		if err != nil {
			t.Fatalf("Decode(%q) for id %d: unexpected error %v", enc, id, err)
		}
		if got != id {
			t.Fatalf("round-trip mismatch: Encode(%d) = %q, Decode -> %d", id, enc, got)
		}
	})
}

// FuzzEncodeDecodeWithAlphabetRoundtrip asserts the same invariant through the
// custom-alphabet path (DecodeWithAlphabet), confirming that an arbitrary
// uint64 survives EncodeWithAlphabet + DecodeWithAlphabet for the default
// alphabet. The alphabet is fixed (the package Alphabet constant) so the
// assertions stay deterministic; only the id varies.
func FuzzEncodeDecodeWithAlphabetRoundtrip(f *testing.F) {
	seeds := []uint64{
		0, 1, 61, 62, 63, 999, 1000,
		1 << 32, 1<<63 - 1, 1 << 63, ^uint64(0),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, id uint64) {
		enc := EncodeWithAlphabet(id, Alphabet)
		if len(enc) == 0 {
			t.Fatalf("EncodeWithAlphabet(%d) returned empty string", id)
		}
		got, err := DecodeWithAlphabet(enc, Alphabet)
		if err != nil {
			t.Fatalf("DecodeWithAlphabet(%q) for id %d: unexpected error %v", enc, id, err)
		}
		if got != id {
			t.Fatalf("round-trip mismatch: EncodeWithAlphabet(%d) = %q, DecodeWithAlphabet -> %d", id, enc, got)
		}
	})
}

// FuzzDecodeArbitraryInput feeds arbitrary strings to Decode and asserts that
// it never panics and obeys its contract: a result is returned only when the
// entire string is composed of default-alphabet bytes, and any other input
// (empty, non-ASCII, symbols) yields ErrInvalid. This guards the byte->index
// table walker against out-of-range bytes and malformed input.
func FuzzDecodeArbitraryInput(f *testing.F) {
	seeds := []string{
		"",      // empty -> ErrInvalid
		"0",     // valid single char
		"10",    // valid multi char
		"z",     // last alphabet byte
		"abc!",  // trailing invalid byte
		" ",     // space, not in alphabet
		"-",     // symbol, not in alphabet
		"0Z9a",  // valid mixed-case
		"\x00",  // NUL byte
		"\xff",  // high byte
		"こんにちは", // multibyte UTF-8
		Encode(999999),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		// Decode must never panic on any input.
		got, err := Decode(s)

		allValid := len(s) > 0
		if allValid {
			for i := 0; i < len(s); i++ {
				if defaultDecodeTable[s[i]] < 0 {
					allValid = false
					break
				}
			}
		}

		switch {
		case allValid:
			if err != nil {
				t.Fatalf("Decode(%q): expected success, got error %v", s, err)
			}
			// Decode tolerates leading zeros (e.g. "0Z9a" == "Z9a"), so the
			// input need not be canonical. The invariant we can assert: the
			// canonical re-encoding decodes back to the same value.
			redec, err := Decode(Encode(got))
			if err != nil {
				t.Fatalf("Decode(Encode(%d)): unexpected error %v", got, err)
			}
			if redec != got {
				t.Fatalf("Decode(%q) = %d, but Decode(Encode(%d)) = %d",
					s, got, got, redec)
			}
		default:
			if err == nil {
				t.Fatalf("Decode(%q): expected error for invalid input, got %d", s, got)
			}
			if err != ErrInvalid {
				t.Fatalf("Decode(%q): expected ErrInvalid, got %v", s, err)
			}
		}
	})
}
