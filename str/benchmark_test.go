package str_test

import (
	"testing"

	"github.com/v8fg/kit4go/str"
)

func BenchmarkStringToBytes(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		str.StringToBytes("Go")
	}
}

func stringToBytesNormal(s string) []byte {
	return []byte(s)
}

func BenchmarkStringToBytesNormal(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		stringToBytesNormal("Go")
	}
}

func BenchmarkBytesToString(b *testing.B) {
	data := []byte{71, 111}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		str.BytesToString(data)
	}
}

func byteSliceToStringNormal(bytes []byte) string {
	return string(bytes)
}
func BenchmarkBytesToStringNormal(b *testing.B) {
	data := []byte{71, 111}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = byteSliceToStringNormal(data)
	}
}
