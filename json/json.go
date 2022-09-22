//go:build !jsoniter && !go_json && !(sonic && avx && (linux || windows || darwin) && amd64)
// +build !jsoniter
// +build !go_json
// +build !sonic !avx !linux,!windows,!darwin !amd64

package json

import "encoding/json"

// PKG package name imported
const PKG = "encoding/json"

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
