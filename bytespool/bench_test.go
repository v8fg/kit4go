package bytespool_test

import (
	"bytes"
	"testing"

	"github.com/v8fg/kit4go/bytespool"
)

func BenchmarkGetPut(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		buf := bytespool.Get(256)
		bytespool.Put(buf)
	}
}

func BenchmarkWithBuffer(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		bytespool.WithBuffer(256, func(buf *bytes.Buffer) {
			buf.WriteString("x")
		})
	}
}
