package base62

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeKnownSmall(t *testing.T) {
	// Alphabet index: 0-9=0..9, A-Z=10..35, a-z=36..61.
	require.Equal(t, "0", Encode(0))
	require.Equal(t, "1", Encode(1))
	require.Equal(t, "z", Encode(61))   // index 61 = 'z'
	require.Equal(t, "10", Encode(62))  // 1*62 + 0
	require.Equal(t, "1z", Encode(123)) // 1*62 + 61
}

func TestRoundTrip(t *testing.T) {
	values := []uint64{
		0, 1, 2, 61, 62, 63, 123, 999, 1000,
		1 << 10, 1<<31 - 1, 1 << 32,
		123456789012345, 1<<63 - 1, 1<<63 + 1,
		^uint64(0), // max uint64
	}
	for _, v := range values {
		enc := Encode(v)
		dec, err := Decode(enc)
		require.NoError(t, err, "decode %d -> %q", v, enc)
		require.Equal(t, v, dec, "round-trip %d", v)
		require.Greater(t, len(enc), 0)
	}
}

func TestEncodeIsCompact(t *testing.T) {
	// base-62 of a uint64 never exceeds 11 chars.
	require.LessOrEqual(t, len(Encode(^uint64(0))), 11)
	// A small id is short.
	require.Equal(t, 1, len(Encode(5)))
}

func TestDecodeInvalid(t *testing.T) {
	_, err := Decode("")
	require.ErrorIs(t, err, ErrInvalid)
	_, err = Decode("abc!") // '!' not in alphabet
	require.ErrorIs(t, err, ErrInvalid)
	_, err = Decode(" ")
	require.ErrorIs(t, err, ErrInvalid)
}

func TestCustomAlphabetRoundTrip(t *testing.T) {
	// Reversed-style custom alphabet: still 62 unique bytes.
	custom := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ9876543210"
	for _, v := range []uint64{0, 1, 61, 62, 999999, ^uint64(0)} {
		enc := EncodeWithAlphabet(v, custom)
		dec, err := DecodeWithAlphabet(enc, custom)
		require.NoError(t, err)
		require.Equal(t, v, dec)
	}
}

func TestAlphabetValidation(t *testing.T) {
	_, err := DecodeWithAlphabet("0", "0123456789abcdefghijklmnopqrstuvwxyz") // 36 chars
	require.ErrorIs(t, err, ErrAlphabet)
	// Duplicate byte in an otherwise 62-byte alphabet -> ErrAlphabet. The
	// string must be exactly 62 bytes so it passes the length guard and reaches
	// the duplicate-detection branch of buildDecodeTableErr (replacing 'Z' with
	// '0' yields two '0's and no 'Z', length unchanged).
	dup := Alphabet[:35] + "0" + Alphabet[36:]
	require.Len(t, dup, 62)
	_, err = DecodeWithAlphabet("0", dup)
	require.ErrorIs(t, err, ErrAlphabet)
}

func TestDecodeRejectsOutOfRangeChar(t *testing.T) {
	// A byte > 127 (non-ASCII) is outside the 256 table but lands in a slot;
	// here a valid-alphabet char is fine, a non-alphabet ASCII char errors.
	_, err := Decode("0Z9a")
	require.NoError(t, err)
	_, err = Decode("0Z9-") // '-' invalid
	require.ErrorIs(t, err, ErrInvalid)
}

// TestDecodeMatchesCustomPath guards the precomputed default-alphabet fast
// path: Decode must return exactly what DecodeWithAlphabet(default) returns,
// for valid strings and for the same error sentinel on invalid ones.
func TestDecodeMatchesCustomPath(t *testing.T) {
	// Valid encodings across the whole uint64 range plus boundary values.
	inputs := []string{
		"0", "1", "z", "10", "1z", "aB3", "0Z9a",
		Encode(0), Encode(61), Encode(62), Encode(999999), Encode(^uint64(0)),
	}
	for _, s := range inputs {
		want, errW := DecodeWithAlphabet(s, Alphabet)
		got, errG := Decode(s)
		require.Equal(t, errW, errG, "error mismatch for %q", s)
		require.Equal(t, want, got, "value mismatch for %q", s)
	}
	// Invalid inputs must yield the same sentinel from both paths.
	for _, s := range []string{"", "abc!", "-", " ", "0Z9-"} {
		_, errW := DecodeWithAlphabet(s, Alphabet)
		_, errG := Decode(s)
		require.Equal(t, errW, errG, "error mismatch for invalid %q", s)
		require.ErrorIs(t, errG, ErrInvalid, "invalid %q", s)
	}
}

// TestDefaultDecodeTable checks the package-init precomputed table directly.
func TestDefaultDecodeTable(t *testing.T) {
	// Every default-alphabet byte maps to its own index.
	for i := 0; i < 62; i++ {
		require.Equal(t, int8(i), defaultDecodeTable[Alphabet[i]],
			"index of byte %q", Alphabet[i])
	}
	// Bytes not in the alphabet (a sample across ASCII) must be -1.
	for _, c := range []byte{'!', '-', ' ', '#', '\x00', '\xff', 0x80} {
		require.Equal(t, int8(-1), defaultDecodeTable[c], "non-alphabet byte %d", c)
	}
}

// TestDecodeWithTableHelper covers the unexported walker used by both Decode
// and DecodeWithAlphabet.
func TestDecodeWithTableHelper(t *testing.T) {
	idx := buildDecodeTable(Alphabet)
	// Empty -> ErrInvalid, regardless of table.
	_, err := decodeWithTable("", &idx)
	require.ErrorIs(t, err, ErrInvalid)
	// Valid -> expected value.
	v, err := decodeWithTable("10", &idx) // 1*62 + 0
	require.NoError(t, err)
	require.Equal(t, uint64(62), v)
	// A slot set to -1 in the table is rejected as invalid.
	idx['1'] = -1
	_, err = decodeWithTable("10", &idx)
	require.ErrorIs(t, err, ErrInvalid)
}

// TestBuildDecodeTableErrDuplicate covers the duplicate-byte rejection in the
// custom-alphabet table builder (mirrors DecodeWithAlphabet validation).
func TestBuildDecodeTableErrDuplicate(t *testing.T) {
	// A valid alphabet yields ok=true with correct indices.
	idx, ok := buildDecodeTableErr(Alphabet)
	require.True(t, ok)
	require.Equal(t, int8(0), idx[Alphabet[0]])
	require.Equal(t, int8(61), idx[Alphabet[61]])
	// A duplicate byte yields ok=false.
	dup := "00123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	_, ok = buildDecodeTableErr(dup)
	require.False(t, ok)
}
