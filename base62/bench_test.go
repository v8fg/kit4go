package base62_test

import (
	"testing"

	"github.com/v8fg/kit4go/base62"
)

func BenchmarkEncode(b *testing.B) {
	const id = 1234567890123456
	b.ReportAllocs()
	for b.Loop() {
		_ = base62.Encode(id)
	}
}

func BenchmarkDecode(b *testing.B) {
	s := base62.Encode(1234567890123456)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = base62.Decode(s)
	}
}

// BenchmarkDecodeError measures the error path (invalid input) — no panic, fast
// rejection.
func BenchmarkDecodeInvalid(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = base62.Decode("not!base62$")
	}
}
