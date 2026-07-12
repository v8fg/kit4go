//go:build jsoniter
// +build jsoniter

package json

import jsoniter "github.com/json-iterator/go"

// PKG package name imported
const PKG = "jsoniter"

// Backend returns the active JSON backend's short name for monitoring.
func Backend() string { return "jsoniter" }

var (
	// json is the jsoniter backend configured for stdlib compatibility. NOTE:
	// jsoniter is NOT a true drop-in for cyclic values — unlike encoding/json and
	// goccy (which return a "cycle" error), jsoniter has no cycle guard and
	// recurses until the goroutine stack overflows (a fatal, unrecoverable
	// crash). Do not Marshal self-referential / graph structures under this tag.
	json = jsoniter.ConfigCompatibleWithStandardLibrary

	// Marshal is exported by kit4go/json package.
	Marshal = json.Marshal

	// Unmarshal is exported by kit4go/json package.
	Unmarshal = json.Unmarshal

	// MarshalIndent is exported by kit4go/json package.
	MarshalIndent = json.MarshalIndent

	// NewEncoder is exported by kit4go/json package.
	NewEncoder = json.NewEncoder

	// NewDecoder is exported by kit4go/json package.
	NewDecoder = json.NewDecoder

	// Valid is exported by kit4go/json package.
	Valid = json.Valid
)
