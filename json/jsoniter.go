//go:build jsoniter

package json

import jsoniter "github.com/json-iterator/go"

// PKG package name imported
const PKG = "json_iter"

var (
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
