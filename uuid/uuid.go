package uuid

import (
	"encoding/hex"

	uid "github.com/satori/go.uuid"
)

// RequestID returns the request ID with UUID V4, in hash-like format, 32 chars.
func RequestID() string {
	u := NewV4()
	buf := make([]byte, 32)
	hex.Encode(buf[0:], u[0:])
	return string(buf)
}

// RequestIDCanonicalFormat returns the request ID with UUID V4, in canonical format, 36 chars.
func RequestIDCanonicalFormat() string {
	return NewV4().String()
}

// ToRequestID converts the uuid with UUID V4 to a request ID, in hash-like format, 32 chars.
func ToRequestID(u uid.UUID) string {
	buf := make([]byte, 32)
	hex.Encode(buf[0:], u[0:])
	return string(buf)
}
