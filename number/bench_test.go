package number_test

import (
	"testing"

	"github.com/v8fg/kit4go/number"
)

// BenchmarkRound measures the rounding hot path (widely used in money/metrics).
func BenchmarkRound(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = number.Round(3.14159265, 4)
	}
}

// BenchmarkBytesToUint measures the byte-slice → uint conversion (packet/ID
// decoding).
func BenchmarkBytesToUint(b *testing.B) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	b.ReportAllocs()
	for b.Loop() {
		_ = number.BytesToUint(data)
	}
}
