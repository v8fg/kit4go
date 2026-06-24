package otp

import (
	cryptorand "crypto/rand"
)

// RandomReader is the minimal reader contract used by RandomSecret and the
// default secret generator. It mirrors io.Reader but is declared as an
// interface so mockery can generate a mock for deterministic error-path
// testing (gomonkey used to patch crypto/rand.Read).
//
// Read fills b with random bytes and returns the number of bytes written.
// It never returns fewer than len(b) bytes unless err != nil.
type RandomReader interface {
	Read(b []byte) (n int, err error)
}

// cryptoRandReader adapts crypto/rand.Reader to the RandomReader interface.
type cryptoRandReader struct{}

// Read implements RandomReader by delegating to crypto/rand.Read.
func (cryptoRandReader) Read(b []byte) (int, error) { return cryptorand.Read(b) }

// DefaultRandomReader is the RandomReader used by the package functions.
// It defaults to crypto/rand.Reader; tests may temporarily replace it
// (defer restore) to inject read failures.
//
//go:generate mockery --name RandomReader --inpackage --with-expecter --filename mock_RandomReader.go
var DefaultRandomReader RandomReader = cryptoRandReader{}
