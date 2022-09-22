//go:build go_json
// +build go_json

package json

import json "github.com/goccy/go-json"

// PKG package name imported
const PKG = "go-json"

var (
	// Marshal is exported by gin/json package.
	Marshal = json.Marshal

	// Unmarshal is exported by gin/json package.
	Unmarshal = json.Unmarshal

	// MarshalIndent is exported by gin/json package.
	MarshalIndent = json.MarshalIndent

	// NewDecoder is exported by gin/json package.
	NewDecoder = json.NewDecoder

	// NewEncoder is exported by gin/json package.
	NewEncoder = json.NewEncoder

	// Valid is exported by kit4go/json package.
	Valid = json.Valid
)
