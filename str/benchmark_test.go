package str_test

import (
	"testing"

	"github.com/v8fg/kit4go/str"
)

func BenchmarkStringToBytes(b *testing.B) {

	b.ReportAllocs()
	for b.Loop() {
		str.StringToBytes("Go")
	}
}

func stringToBytesNormal(s string) []byte {
	return []byte(s)
}

func BenchmarkStringToBytesNormal(b *testing.B) {

	b.ReportAllocs()
	for b.Loop() {
		stringToBytesNormal("Go")
	}
}

func BenchmarkBytesToString(b *testing.B) {
	data := []byte{71, 111}

	b.ReportAllocs()
	for b.Loop() {
		str.BytesToString(data)
	}
}

func byteSliceToStringNormal(bytes []byte) string {
	return string(bytes)
}
func BenchmarkBytesToStringNormal(b *testing.B) {
	data := []byte{71, 111}

	b.ReportAllocs()
	for b.Loop() {
		_ = byteSliceToStringNormal(data)
	}
}
