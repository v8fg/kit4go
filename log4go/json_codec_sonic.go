package log4go

import "github.com/bytedance/sonic"

// sonicMarshal delegates to bytedance/sonic. Sonic uses SIMD on amd64 and falls
// back to a pure-Go path on other architectures (arm64, etc.), so it is safe to
// select on any platform — it just may not be faster than goccy on non-amd64.
// It is the fastest option on amd64 for the structured-log shape.
func sonicMarshal(v interface{}) ([]byte, error) {
	return sonic.Marshal(v)
}
