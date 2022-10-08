// Package random, the file refs the crypto/rand, it implements a cryptographically secure
// random number generator.
package random

import (
	"crypto/rand"
	"encoding/base64"
	"math/big"
)

// CryptoInt returns a uniform random value in [0, max). It panics if max <= 0.
// secure random number generator.
func CryptoInt(max int64) (n *big.Int, err error) {
	return rand.Int(rand.Reader, big.NewInt(max))
}

// CryptoPrime returns a number of the given bit length that is prime with high probability.
// Prime will return error for any error returned by rand.Read or if bits < 2.
func CryptoPrime(bits int) (n *big.Int, err error) {
	return rand.Prime(rand.Reader, bits)
}

// CryptoRead is a helper function that calls Reader.Read using io.ReadFull.
// On return, n == len(b) if and only if err == nil.
func CryptoRead(b []byte) (n int, err error) {
	return rand.Read(b)
}

// CryptoReadString returns the random string with the length == len(b).
func CryptoReadString(b []byte) string {
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}
