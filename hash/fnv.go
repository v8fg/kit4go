package hash

import (
	"hash/fnv"
)

// FNV-1a is a fast, non-cryptographic hash. It is deterministic across runs and
// platforms, which makes it suitable for consistent bucketing / sharding of keys
// (e.g. assigning a user hash to a bidder shard) where a cryptographic hash would
// be wasted cost. Do NOT use it for any security purpose.

// FNV1a32 returns the 32-bit FNV-1a hash of data. The empty-input value is the
// FNV offset basis 0x811c9dc5 (2166136261).
func FNV1a32(data []byte) uint32 {
	h := fnv.New32a()
	_, _ = h.Write(data)
	return h.Sum32()
}

// FNV1a64 returns the 64-bit FNV-1a hash of data.
func FNV1a64(data []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(data)
	return h.Sum64()
}

// FNV132 returns the 32-bit FNV-1 hash of data (the multiply-then-xor variant).
func FNV132(data []byte) uint32 {
	h := fnv.New32()
	_, _ = h.Write(data)
	return h.Sum32()
}

// FNV164 returns the 64-bit FNV-1 hash of data.
func FNV164(data []byte) uint64 {
	h := fnv.New64()
	_, _ = h.Write(data)
	return h.Sum64()
}

// FNV1aString32 is a string-keyed convenience wrapper around FNV1a32.
func FNV1aString32(s string) uint32 { return FNV1a32([]byte(s)) }

// FNV1aString64 is a string-keyed convenience wrapper around FNV1a64.
func FNV1aString64(s string) uint64 { return FNV1a64([]byte(s)) }
