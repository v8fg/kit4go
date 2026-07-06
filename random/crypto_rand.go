// Package random, the file refs the crypto/rand, it implements a cryptographically secure
// random number generator.
package random

import (
	"encoding/base64"
	"math/big"
)

// CryptoInt returns a uniform random value in [0, max). It panics if max <= 0.
// secure random number generator.
func CryptoInt(max int64) (n *big.Int, err error) {
	return DefaultCryptoSource.Int(big.NewInt(max))
}

// CryptoPrime returns a number of the given bit length that is prime with high probability.
// Prime will return error for any error returned by rand.Read or if bits < 2.
func CryptoPrime(bits int) (n *big.Int, err error) {
	return DefaultCryptoSource.Prime(bits)
}

// CryptoRead is a helper function that calls Reader.Read using io.ReadFull.
// On return, n == len(b) if and only if err == nil.
func CryptoRead(b []byte) (n int, err error) {
	return DefaultCryptoSource.Read(b)
}

// CryptoReadString fills b with cryptographically secure random bytes (via
// CryptoRead) and returns them base64 (StdEncoding) encoded. n <= 0 or a nil
// buffer returns "".
//
// On a Read error it returns "" rather than encoding potentially-unfilled
// bytes, so callers can treat any non-empty result as fully random. Inspect
// the underlying error via CryptoRead if you need the cause.
func CryptoReadString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if _, err := DefaultCryptoSource.Read(b); err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}
