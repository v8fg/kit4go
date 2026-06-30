// Package hash provides ergonomic wrappers around the standard library's
// cryptographic hashes (MD5, SHA-1, SHA-2 family), HMAC, FNV, and CRC.
//
// Each function returns the raw digest bytes for composability, with hex
// convenience variants for the common "hash a string and format it" case.
// All functions allocate a fresh hasher per call, so they are safe for
// concurrent use without locking.
//
// Typical ad-tech uses: signing postbacks and webhooks (HMAC-SHA256),
// fingerprinting auction or bid IDs (SHA-256), consistent bucketing of a
// user hash (FNV), and cheap payload checksums (CRC32) — none of which need
// a third-party dependency.
package hash

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
)

// sumBytes runs a fresh hasher over data and returns the raw digest. crypto
// hashers never return an error from Write, so it is intentionally ignored.
func sumBytes(newH func() hash.Hash, data []byte) []byte {
	h := newH()
	_, _ = h.Write(data)
	return h.Sum(nil)
}

// hexOf returns the lowercase hex encoding of b.
func hexOf(b []byte) string { return hex.EncodeToString(b) }

// MD5 returns the MD5 checksum of data (16 bytes). MD5 is cryptographically
// broken; use it only for non-security checksums, ETags, or legacy interop.
func MD5(data []byte) []byte { return sumBytes(md5.New, data) }

// SHA1 returns the SHA-1 checksum of data (20 bytes). SHA-1 is collision-broken;
// prefer SHA-256 for any security-sensitive use.
func SHA1(data []byte) []byte { return sumBytes(sha1.New, data) }

// SHA224 returns the SHA-224 checksum of data (28 bytes).
func SHA224(data []byte) []byte { return sumBytes(sha256.New224, data) }

// SHA256 returns the SHA-256 checksum of data (32 bytes).
func SHA256(data []byte) []byte { return sumBytes(sha256.New, data) }

// SHA384 returns the SHA-384 checksum of data (48 bytes).
func SHA384(data []byte) []byte { return sumBytes(sha512.New384, data) }

// SHA512 returns the SHA-512 checksum of data (64 bytes).
func SHA512(data []byte) []byte { return sumBytes(sha512.New, data) }

// MD5Hex returns the lowercase hex MD5 of s. Empty input yields the well-known
// empty-string digest, not "".
func MD5Hex(s string) string { return hexOf(MD5([]byte(s))) }

// SHA1Hex returns the lowercase hex SHA-1 of s.
func SHA1Hex(s string) string { return hexOf(SHA1([]byte(s))) }

// SHA256Hex returns the lowercase hex SHA-256 of s.
func SHA256Hex(s string) string { return hexOf(SHA256([]byte(s))) }

// SHA512Hex returns the lowercase hex SHA-512 of s.
func SHA512Hex(s string) string { return hexOf(SHA512([]byte(s))) }
