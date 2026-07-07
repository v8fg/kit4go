package shortlink

import "testing"

func BenchmarkGenerate(b *testing.B) {
	s := New(WithCodeLength(6))
	b.ReportAllocs()
	for b.Loop() {
		_, _ = s.Generate("https://example.com/long/url")
	}
}

func BenchmarkEncodeBaseN(b *testing.B) {
	s := NewIDShortener(Alphabet, 0)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.Encode(uint64(i))
	}
}

func BenchmarkResolve(b *testing.B) {
	s := New(WithCodeLength(6))
	code, _ := s.Generate("https://example.com")

	for b.Loop() {
		_, _ = s.Resolve(code)
	}
}
