package log4go

import "testing"

// Benchmark_Field_FloatJSON proves the NaN/Inf-safe float path (touched in the
// safety pass) stays fast: a normal float appends without function-call overhead.
func Benchmark_Field_FloatJSON(b *testing.B) {
	f := floatField("rate", 1.5)
	buf := make([]byte, 0, 32)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = appendFieldJSON(buf[:0], f)
	}
}

// Benchmark_Field_IntJSON is the scalar baseline (no NaN check).
func Benchmark_Field_IntJSON(b *testing.B) {
	f := intField("count", 42)
	buf := make([]byte, 0, 32)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = appendFieldJSON(buf[:0], f)
	}
}
