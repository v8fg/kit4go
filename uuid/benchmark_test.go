package uuid_test

import (
	"testing"
	"time"

	"github.com/v8fg/kit4go/uuid"
)

func BenchmarkRequestID(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = uuid.RequestID()
	}
}

func BenchmarkRequestIDCanonicalFormat(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = uuid.RequestIDCanonicalFormat()
	}
}

func BenchmarkNewV4(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = uuid.NewV4()
	}
}

func BenchmarkNewKSUID(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = uuid.NewKSUID()
	}
}

func BenchmarkNewKSUIDRandomWithTime(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_, _ = uuid.NewKSUIDRandomWithTime(time.Now())
	}
}

func BenchmarkNewXID(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = uuid.NewXID()
	}
}

func BenchmarkNewXIDWithTime(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = uuid.NewXIDWithTime(time.Now())
	}
}

func BenchmarkNewV5(b *testing.B) {
	ns := uuid.NewV4()
	b.ReportAllocs()

	for b.Loop() {
		_ = uuid.NewV5(ns, "example.com")
	}
}
