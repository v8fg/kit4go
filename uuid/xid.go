package uuid

import (
	"time"

	"github.com/rs/xid"
)

// NewXID generates a globally unique ID.
func NewXID() xid.ID {
	return xid.New()
}

// NewXIDWithTime generates a globally unique ID with the passed in time.
func NewXIDWithTime(time time.Time) xid.ID {
	return xid.NewWithTime(time)
}

// XIDSort sorts an array of IDs inplace.
// It works by wrapping `[]ID` and use `sort.Sort`.
func XIDSort(ids []xid.ID) {
	xid.Sort(ids)
}

// XIDFromString reads an ID from its string representation
func XIDFromString(id string) (xid.ID, error) {
	return xid.FromString(id)
}

// XIDFromBytes convert the byte array representation of `ID` back to `ID`
func XIDFromBytes(b []byte) (xid.ID, error) {
	return xid.FromBytes(b)
}
