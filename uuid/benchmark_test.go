package uuid_test

import (
	"testing"
	"time"

	"github.com/v8fg/kit4go/uuid"
)

func BenchmarkRequestID(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uuid.RequestID()
	}
}

func BenchmarkRequestIDCanonicalFormat(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uuid.RequestIDCanonicalFormat()
	}
}

func BenchmarkNewV4(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uuid.NewV4()
	}
}

func BenchmarkNewKSUID(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uuid.NewKSUID()
	}
}

func BenchmarkNewKSUIDRandomWithTime(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = uuid.NewKSUIDRandomWithTime(time.Now())
	}
}

func BenchmarkNewXID(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uuid.NewXID()
	}
}

func BenchmarkNewXIDWithTime(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uuid.NewXIDWithTime(time.Now())
	}
}
