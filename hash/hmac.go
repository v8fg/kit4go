package hash

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"hash"
)

// hmacSum computes HMAC with the given hasher factory over data, keyed by key.
func hmacSum(newH func() hash.Hash, key, data []byte) []byte {
	mac := hmac.New(newH, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}

// HMACSHA1 returns the HMAC-SHA1 of data under key (20 bytes).
func HMACSHA1(key, data []byte) []byte { return hmacSum(sha1.New, key, data) }

// HMACSHA256 returns the HMAC-SHA256 of data under key (32 bytes). This is the
// standard choice for signing postbacks, MMP callbacks, and webhook payloads.
func HMACSHA256(key, data []byte) []byte { return hmacSum(sha256.New, key, data) }

// HMACSHA512 returns the HMAC-SHA512 of data under key (64 bytes).
func HMACSHA512(key, data []byte) []byte { return hmacSum(sha512.New, key, data) }

// HMACSHA256Hex returns the lowercase hex HMAC-SHA256 of data under key.
func HMACSHA256Hex(key, data []byte) string { return hexOf(HMACSHA256(key, data)) }

// HMACSHA256Base64 returns the standard base64 HMAC-SHA256 of data under key.
// Many webhook APIs (and some MMPs) expect the signature in base64 rather than hex.
func HMACSHA256Base64(key, data []byte) string {
	return base64.StdEncoding.EncodeToString(HMACSHA256(key, data))
}

// Equal reports whether two MACs or digests are equal in constant time, avoiding
// timing side-channels. Use it instead of bytes.Equal when comparing signatures.
func Equal(a, b []byte) bool { return hmac.Equal(a, b) }
