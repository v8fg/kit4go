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
	// Duplicate byte -> ErrAlphabet.
	_, err = DecodeWithAlphabet("0", "00123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
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
