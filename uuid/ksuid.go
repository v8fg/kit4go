package uuid

import (
	"time"

	"github.com/segmentio/ksuid"
)

// NewKSUID generates a new KSUID.
func NewKSUID() ksuid.KSUID {
	return ksuid.New()
}

// NewKSUIDRandom generates a new KSUID with now timestamp.
func NewKSUIDRandom() (uid ksuid.KSUID, err error) {
	return ksuid.NewRandomWithTime(time.Now())
}

// NewKSUIDRandomWithTime generates a new KSUID with the given timestamp.
func NewKSUIDRandomWithTime(t time.Time) (uid ksuid.KSUID, err error) {
	return ksuid.NewRandomWithTime(t)
}

// KSUIDParse decodes a string-encoded representation of a KSUID object.
func KSUIDParse(s string) (ksuid.KSUID, error) {
	return ksuid.Parse(s)
}

// KSUIDFromParts constructs a KSUID from constituent parts.
func KSUIDFromParts(t time.Time, payload []byte) (ksuid.KSUID, error) {
	return ksuid.FromParts(t, payload)
}

// KSUIDFromBytes constructs a KSUID from a 20-byte binary representation.
func KSUIDFromBytes(b []byte) (ksuid.KSUID, error) {
	return ksuid.FromBytes(b)
}

// KSUIDCompare implements comparison for KSUID type.
func KSUIDCompare(a, b ksuid.KSUID) int {
	return ksuid.Compare(a, b)
}

// KSUIDSort the given slice of KSUIDs.
func KSUIDSort(ids []ksuid.KSUID) {
	ksuid.Sort(ids)
}

// KSUIDIsSorted checks whether a slice of KSUIDs is sorted
func KSUIDIsSorted(ids []ksuid.KSUID) bool {
	return ksuid.IsSorted(ids)
}
