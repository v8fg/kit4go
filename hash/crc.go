package hash

import (
	"encoding/hex"
	"hash/crc32"
	"hash/crc64"
)

// CRC checksums are cheap, non-cryptographic integrity checks — useful for
// quick payload validation, ETags, or change detection where collisions are
// acceptable. Do NOT use for any security purpose.

// CRC32IEEE returns the CRC-32 checksum of data using the IEEE polynomial
// (the most common CRC-32, used by zlib/PNG/Ethernet). The empty-input value is 0.
func CRC32IEEE(data []byte) uint32 { return crc32.ChecksumIEEE(data) }

// CRC32IEEEHex returns the lowercase hex CRC-32 (IEEE) of data.
func CRC32IEEEHex(data []byte) string {
	b := make([]byte, 4)
	v := CRC32IEEE(data)
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
	return hex.EncodeToString(b)
}

// CRC64ISO returns the CRC-64 checksum of data using the ISO polynomial.
func CRC64ISO(data []byte) uint64 { return crc64.Checksum(data, crc64.MakeTable(crc64.ISO)) }

// CRC64ECMA returns the CRC-64 checksum of data using the ECMA polynomial.
func CRC64ECMA(data []byte) uint64 { return crc64.Checksum(data, crc64.MakeTable(crc64.ECMA)) }
