//go:build go_json
// +build go_json

// Package json provides a pluggable JSON codec selected via build tags:
// encoding/json (default), goccy/go-json (go_json), jsoniter, or sonic.
// Marshal/Unmarshal/MarshalToString are re-exported so importers share
// one codec across the codebase; Backend() returns the active name for
// monitoring, and PKG records the import path.
package json

import json "github.com/goccy/go-json"

// PKG package name imported
const PKG = "go_json"

// Backend returns the active JSON backend's short name for monitoring.
func Backend() string { return "go_json" }

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
