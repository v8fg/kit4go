//go:build !jsoniter && !go_json && !(sonic && avx && (linux || windows || darwin) && amd64)
// +build !jsoniter
// +build !go_json
// +build !sonic !avx !linux,!windows,!darwin !amd64

package json

import "encoding/json"

// PKG package name imported
const PKG = "encoding/json"

// Backend returns the active JSON backend's short name for monitoring — the
// build tag selects which file (and thus which value) compiles: "stdlib"
// (default), "goccy" (-tags go_json), "jsoniter" (-tags jsoniter), or "sonic"
// (-tags sonic).
func Backend() string { return "stdlib" }

var (
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
