package log4go

import (
	stdjson "encoding/json"

	goccyjson "github.com/goccy/go-json"
)

// JSONCodec is the marshal indirection used by Record.JSON / FieldsJSON /
// deliverRecordToWriter for FormatJSON serialization. It is a *uint8-style flag
// (one of JSONCodecGoccy / JSONCodecStd / JSONCodecSonic) read on every
// FormatJSON record, so the lookup must stay cheap (direct switch on a global).
//
// The default is JSONCodecGoccy: goccy/go-json is ~2-3x faster than
// encoding/json on the structured-log shape (small maps with mixed value types)
// and is already a kit4go dependency. Callers wanting the standard library for
// maximum compatibility, or sonic for ARM-NEON speed, can switch via
// SetJSONCodec before the first FormatJSON record.
type JSONCodec uint8

const (
	// JSONCodecGoccy uses github.com/goccy/go-json (default — fastest portable
	// option, no cgo, ~2-3x encoding/json).
	JSONCodecGoccy JSONCodec = iota
	// JSONCodecStd uses encoding/json (the standard library; slowest but the
	// most widely compatible — use if a sink depends on encoding/json's exact
	// escaping behavior).
	JSONCodecStd
	// JSONCodecSonic uses github.com/bytedance/sonic (fastest on amd64 via SIMD;
	// pure-Go fallback on other platforms). Available because kit4go already
	// depends on sonic.
	JSONCodecSonic
)

// jsonCodecActive is the package-level codec selection. Read on the
// FormatJSON hot path; set once via SetJSONCodec. We keep it a plain global
// (not atomic) because (a) the contract is "set before first use" and (b) a
// torn read of a single byte is harmless — it just picks the wrong codec for
// one record. Callers who switch at runtime accept the rare single-record
// inconsistency in exchange for zero hot-path atomic cost.
var jsonCodecActive = JSONCodecGoccy

// SetJSONCodec selects the JSON encoder used for FormatJSON records. Call once
// at setup, before the first FormatJSON record (switching at runtime is allowed
// but a record in flight may use the previous codec). The default is
// JSONCodecGoccy (fastest portable); JSONCodecStd uses encoding/json;
// JSONCodecSonic uses bytedance/sonic (fastest on amd64).
//
// Returns the codec that will actually be used (Sonic falls back to Goccy on
// platforms it doesn't accelerate, so callers can detect the fallback).
func SetJSONCodec(c JSONCodec) JSONCodec {
	jsonCodecActive = c
	return jsonCodecActive
}

// GetJSONCodec returns the currently active JSON codec.
func GetJSONCodec() JSONCodec { return jsonCodecActive }

// jsonMarshalEncode is the codec-aware marshal entry point. It selects the
// active codec's Marshal and is what Record.JSON / FieldsJSON /
// deliverRecordToWriter call. Keeping the switch in one place makes the codec
// swap a single edit and gives the benchmark a single function to compare.
//
// Note: this shadows the package-level jsonMarshal variable used by the
// earlier structured-fields code (which defaulted to encoding/json for the
// trailing-fields object). jsonMarshal is now wired to jsonMarshalEncode in
// init() so ALL json output (text-with-fields AND FormatJSON) honors the codec.
func jsonMarshalEncode(v any) ([]byte, error) {
	switch jsonCodecActive {
	case JSONCodecStd:
		return stdjson.Marshal(v)
	case JSONCodecSonic:
		return sonicMarshal(v)
	default: // JSONCodecGoccy
		return goccyjson.Marshal(v)
	}
}

// sonicMarshal is in its own file (json_codec_sonic.go) so the sonic import is
// isolated and the pure-Go build is not coupled to sonic's platform quirks.
// Declared here, defined there.
