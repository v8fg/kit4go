package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestRequestID_Generates(t *testing.T) {
	var captured string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if captured == "" {
		t.Fatal("request ID not in context")
	}
	if rec.Header().Get(HeaderRequestID) == "" {
		t.Fatal("X-Request-ID not in response header")
	}
}

func TestRequestID_PreservesIncoming(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if FromContext(r.Context()) != "trace-123" {
			t.Fatal("incoming request ID not preserved")
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderRequestID, "trace-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Header().Get(HeaderRequestID) != "trace-123" {
		t.Fatal("response should echo incoming request ID")
	}
}

func TestRateLimit_AllowsThenBlocks(t *testing.T) {
	var count atomic.Int32
	allow := func() bool { return count.Add(1) <= 1 } // allow first, block rest
	h := RateLimit(allow, 0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	// First request: allowed
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, httptest.NewRequest("GET", "/", nil))
	if rec1.Code != 200 {
		t.Fatalf("first request: %d, want 200", rec1.Code)
	}
	// Second request: blocked
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest("GET", "/", nil))
	if rec2.Code != 429 {
		t.Fatalf("second request: %d, want 429", rec2.Code)
	}
}

func TestRateLimit_RetryAfter(t *testing.T) {
	h := RateLimit(func() bool { return false }, 60)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("Retry-After = %q, want 60", rec.Header().Get("Retry-After"))
	}
}

func TestCORS_Preflight(t *testing.T) {
	h := CORS(CORSConfig{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight should not reach handler")
	}))
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("preflight: %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("Allow-Origin = %q, want *", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORS_SpecificOrigin(t *testing.T) {
	h := CORS(CORSConfig{
		AllowOrigins:     []string{"https://ads.example.com"},
		AllowCredentials: true,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://ads.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://ads.example.com" {
		t.Fatalf("Allow-Origin = %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("Allow-Credentials missing")
	}
}

func TestCORS_RejectedOrigin(t *testing.T) {
	h := CORS(CORSConfig{AllowOrigins: []string{"https://trusted.com"}})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("untrusted origin should get empty Allow-Origin, got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}
