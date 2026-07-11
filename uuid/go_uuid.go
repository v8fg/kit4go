package uuid

import (
	uid "github.com/gofrs/uuid/v5"
)

// Equal returns true if u1 and u2 equals, otherwise returns false.
func Equal(u1 uid.UUID, u2 uid.UUID) bool {
	return u1 == u2
}

// NewV1 returns UUID based on current timestamp and MAC address.
func NewV1() uid.UUID {
	return uid.Must(uid.NewV1())
}

// NewV3 returns UUID based on MD5 hash of namespace UUID and name.
func NewV3(ns uid.UUID, name string) uid.UUID {
	return uid.NewV3(ns, name)
}

// NewV4 returns random generated UUID.
func NewV4() uid.UUID {
	return uid.Must(uid.NewV4())
}

// NewV5 returns UUID based on SHA-1 hash of namespace UUID and name.
func NewV5(ns uid.UUID, name string) uid.UUID {
	return uid.NewV5(ns, name)
}

// FromBytes returns UUID converted from raw byte slice input.
// It will return error if the slice isn't 16 bytes long.
func FromBytes(input []byte) (u uid.UUID, err error) {
	return uid.FromBytes(input)
}

// FromBytesOrNil returns UUID converted from raw byte slice input.
// Same behavior as FromBytes, but returns a Nil UUID on error.
func FromBytesOrNil(input []byte) uid.UUID {
	return uid.FromBytesOrNil(input)
}

// FromString returns UUID parsed from string input.
// Input is expected in a form accepted by UnmarshalText.
func FromString(input string) (u uid.UUID, err error) {
	return uid.FromString(input)
}

// FromStringOrNil returns UUID parsed from string input.
// Same behavior as FromString, but returns a Nil UUID on error.
func FromStringOrNil(input string) uid.UUID {
	return uid.FromStringOrNil(input)
}
