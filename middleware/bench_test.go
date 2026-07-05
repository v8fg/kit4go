package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkRequestID(b *testing.B) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkCORS(b *testing.B) {
	h := CORS(CORSConfig{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://example.com")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkRateLimit(b *testing.B) {
	h := RateLimit(func() bool { return true }, 0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}
