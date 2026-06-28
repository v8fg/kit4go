package random

import (
	cryptorand "crypto/rand"
	"math/big"
)

// CryptoSource is the crypto/rand subset used by CryptoInt / CryptoPrime /
// CryptoRead / CryptoReadString. Declaring it as an interface lets mockery
// generate a mock so the error paths (gomonkey used to patch rand.Int /
// rand.Prime / rand.Read) are covered deterministically without runtime
// patching.
//
// Public package API signatures are unchanged; injection is through the
// package-level DefaultCryptoSource variable, which tests swap (defer restore).
type CryptoSource interface {
	// Int mirrors crypto/rand.Int: a uniform random value in [0, max).
	Int(max *big.Int) (n *big.Int, err error)
	// Prime mirrors crypto/rand.Prime: a prime of the given bit length.
	Prime(bits int) (p *big.Int, err error)
	// Read mirrors crypto/rand.Read.
	Read(b []byte) (n int, err error)
}

// cryptoRandSource is the default CryptoSource delegating to crypto/rand.
type cryptoRandSource struct{}

// Int implements CryptoSource.
func (cryptoRandSource) Int(max *big.Int) (*big.Int, error) {
	return cryptorand.Int(cryptorand.Reader, max)
}

// Prime implements CryptoSource.
func (cryptoRandSource) Prime(bits int) (*big.Int, error) {
	return cryptorand.Prime(cryptorand.Reader, bits)
}

// Read implements CryptoSource.
func (cryptoRandSource) Read(b []byte) (int, error) { return cryptorand.Read(b) }

// DefaultCryptoSource is the CryptoSource used by the package functions. It
// defaults to crypto/rand; tests may temporarily replace it (defer restore) to
// inject failures.
//
//go:generate mockery --name CryptoSource --inpackage --with-expecter --filename mock_CryptoSource.go
var DefaultCryptoSource CryptoSource = cryptoRandSource{}
