package shortlink

import (
	"testing"
)

// FuzzGenerateUnique fuzzes the random-code Shortener's Generate method. For
// each fuzz input it generates up to N codes into a fresh MemoryStore and
// asserts that every successfully returned code is unique. The collision-retry
// loop in Generate (plus the store's ErrCollision guard) is the contract that
// makes this assertion deterministic: a successful Generate must never return a
// code already present in the store.
//
// Inputs:
//   - n: number of codes to generate (clamped to [0, 5000] to bound runtime)
//   - codeLen: per-code length (clamped to [1, 16])
//
// The default alphabet is used so codes draw from the full 62-symbol space.
// Determinism note: Generate is non-deterministic in its random draw, but the
// uniqueness assertion is deterministic given the store's collision contract —
// a duplicate return value is always a bug, never a flake.
func FuzzGenerateUnique(f *testing.F) {
	// Seeds: (count, codeLen). Mix small/large counts and short/long codes.
	f.Add(1, 6)
	f.Add(10, 4)
	f.Add(100, 6)
	f.Add(500, 2) // tiny code space (62^2 = 3844) — heavy on retries
	f.Add(2000, 8)

	f.Fuzz(func(t *testing.T, n, codeLen int) {
		// Clamp inputs to keep the corpus bounded and meaningful.
		if n < 0 || n > 5000 {
			t.Skip("n out of bounded range")
		}
		if n == 0 {
			t.Skip("nothing to assert for zero codes")
		}
		if codeLen < 1 || codeLen > 16 {
			t.Skip("codeLen out of bounded range")
		}

		s := New(WithCodeLength(codeLen))
		seen := make(map[string]struct{}, n)

		for i := 0; i < n; i++ {
			code, err := s.Generate("https://example.com/u")
			if err != nil {
				// A non-recoverable store error is a real failure; the default
				// MemoryStore never returns one, so any error here is a bug.
				t.Fatalf("generate %d: unexpected error: %v", i, err)
			}
			if len(code) != codeLen {
				t.Fatalf("generate %d: code length = %d, want %d", i, len(code), codeLen)
			}
			if _, dup := seen[code]; dup {
				// The store rejects collisions and Generate retries; a returned
				// duplicate means the uniqueness contract is broken.
				t.Fatalf("generate %d: duplicate code %q (collision-retry failed)", i, code)
			}
			seen[code] = struct{}{}
		}
	})
}

// FuzzIDShortener fuzzes the deterministic ID-based shortener. For a fuzzed
// start ID and count it encodes a sequential range and asserts two properties:
//  1. Determinism: encoding the same ID twice yields the same code.
//  2. Uniqueness: every code in the sequential range is distinct.
//
// A round-trip Decode check is also applied: each code must decode back to the
// original ID, proving the base62 encode/decode pair is consistent. All
// assertions are pure functions of the input, so the test is fully deterministic.
//
// Inputs:
//   - startID: first ID in the range
//   - count:   number of sequential IDs to encode (clamped to [0, 1<<16])
func FuzzIDShortener(f *testing.F) {
	// Seeds: (startID, count). Include 0, boundary values, and large spans.
	f.Add(uint64(0), uint64(1))
	f.Add(uint64(0), uint64(100))
	f.Add(uint64(1), uint64(1))
	f.Add(uint64(61), uint64(2)) // base62 boundary
	f.Add(uint64(62), uint64(2)) // base62 carry
	f.Add(uint64(1<<32-5), uint64(10))
	f.Add(uint64(1<<63), uint64(100))

	f.Fuzz(func(t *testing.T, startID, count uint64) {
		// Clamp count to bound runtime and avoid uint64 overflow near the top.
		if count == 0 {
			t.Skip("nothing to assert for zero-length range")
		}
		const maxCount uint64 = 1 << 16
		if count > maxCount {
			t.Skip("count exceeds bounded range")
		}
		// Guard against startID+count overflowing uint64.
		if startID > ^uint64(0)-count {
			t.Skip("startID+count overflows uint64")
		}

		s := NewIDShortener(Alphabet, startID)

		seen := make(map[string]uint64, count)
		for i := uint64(0); i < count; i++ {
			id := startID + i

			// 1. Determinism: re-encoding the same ID must be stable.
			c1 := s.Encode(id)
			c2 := s.Encode(id)
			if c1 != c2 {
				t.Fatalf("id %d: non-deterministic encode %q vs %q", id, c1, c2)
			}

			// 2. Round-trip: code must decode back to the source ID.
			decoded, err := s.Decode(c1)
			if err != nil {
				t.Fatalf("id %d: decode %q: %v", id, c1, err)
			}
			if decoded != id {
				t.Fatalf("id %d: round-trip decode = %d", id, decoded)
			}

			// 3. Uniqueness: each code must map to exactly one ID in the range.
			if prev, dup := seen[c1]; dup {
				t.Fatalf("duplicate code %q for ids %d and %d", c1, prev, id)
			}
			seen[c1] = id
		}
	})
}
