package hash

import (
	"hash/crc32"
	"hash/crc64"
	"testing"

	"github.com/stretchr/testify/require"
)

// "123456789" is the universal CRC check sequence (its check value is published
// in every CRC spec for each polynomial).
func TestCRC_CheckSequence(t *testing.T) {
	d := []byte("123456789")
	require.Equal(t, uint32(0xcbf43926), CRC32IEEE(d))         // CRC-32/ISO-HDLC check
	require.Equal(t, uint64(0xb90956c775a41001), CRC64ISO(d))  // CRC-64/ISO check
	require.Equal(t, uint64(0x995dc9bbdf1939fa), CRC64ECMA(d)) // Go stdlib ECMA (reflected) check
}

func TestCRC_Empty(t *testing.T) {
	require.Equal(t, uint32(0), CRC32IEEE(nil))
	require.Equal(t, uint32(0), CRC32IEEE([]byte{}))
	require.Equal(t, "00000000", CRC32IEEEHex(nil))
}

// Cross-check against the stdlib tables.
func TestCRC_MatchesStdlib(t *testing.T) {
	inputs := []string{"", "a", "bid", "impression_id=abc", "123456789"}
	for _, s := range inputs {
		d := []byte(s)
		require.Equal(t, crc32.ChecksumIEEE(d), CRC32IEEE(d))
		require.Equal(t, crc64.Checksum(d, crc64.MakeTable(crc64.ISO)), CRC64ISO(d))
		require.Equal(t, crc64.Checksum(d, crc64.MakeTable(crc64.ECMA)), CRC64ECMA(d))
	}
}

func TestCRC32HexFormat(t *testing.T) {
	// Hex is 8 chars, lowercase, big-endian of the uint32.
	h := CRC32IEEEHex([]byte("123456789"))
	require.Len(t, h, 8)
	require.Equal(t, "cbf43926", h)
}

func TestCRC_DistributionSanity(t *testing.T) {
	// Distinct inputs should very likely produce distinct CRCs.
	require.NotEqual(t, CRC32IEEE([]byte("a")), CRC32IEEE([]byte("b")))
	require.NotEqual(t, CRC64ISO([]byte("a")), CRC64ISO([]byte("b")))
}
