package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestCORS_CredentialsWithWildcardOrigin covers the spec-compliance fallback
// (middleware.go ~line 118): when AllowCredentials is true AND the allowlist is
// "*", the middleware must NOT echo "*" (browsers reject "*"+credentials) but
// instead return the request's specific Origin header.
func TestCORS_CredentialsWithWildcardOrigin(t *testing.T) {
	h := CORS(CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	const want = "https://ads.example.com"
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", want)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != want {
		t.Fatalf("Allow-Origin = %q, want %q (echoed origin, not wildcard)", got, want)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("Allow-Credentials missing")
	}
}

// TestCORS_CredentialsWithWildcardOrigin_NoOriginHeader confirms the fallback
// branch is only entered when an Origin header is present; with credentials +
// wildcard but no Origin, Allow-Origin stays "*".
func TestCORS_CredentialsWithWildcardOrigin_NoOriginHeader(t *testing.T) {
	h := CORS(CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil) // no Origin header
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q, want * when no Origin header", got)
	}
}

// TestCORS_MaxAge covers the cfg.MaxAge > 0 branch (middleware.go ~line 127):
// the Access-Control-Max-Age header must be set to the duration in seconds.
func TestCORS_MaxAge(t *testing.T) {
	const maxAge = 30 * time.Minute // 1800s
	h := CORS(CORSConfig{MaxAge: maxAge})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Max-Age"); got != "1800" {
		t.Fatalf("Access-Control-Max-Age = %q, want 1800", got)
	}
}

// TestCORS_PreflightWithMaxAge ensures the MaxAge branch is also taken on
// preflight (OPTIONS) requests, which is its primary intended use.
func TestCORS_PreflightWithMaxAge(t *testing.T) {
	h := CORS(CORSConfig{MaxAge: 5 * time.Minute})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight must not reach handler")
	}))
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 204 {
		t.Fatalf("preflight code = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "300" {
		t.Fatalf("Access-Control-Max-Age = %q, want 300", got)
	}
}

// TestJoinCSV_Empty covers the joinCSV len(ss)==0 branch directly. All
// production callers pass non-nil slices (CORSConfig nil-slices are normalized
// to defaults before joinCSV), so the empty branch is only reachable via a
// direct white-box call.
func TestJoinCSV_Empty(t *testing.T) {
	if got := joinCSV(nil); got != "" {
		t.Fatalf("joinCSV(nil) = %q, want empty", got)
	}
	if got := joinCSV([]string{}); got != "" {
		t.Fatalf("joinCSV([]string{}) = %q, want empty", got)
	}
}

// TestJoinCSV_SingleAndMultiple guards the non-empty path of joinCSV (already
// exercised indirectly by CORS, but pinned here for stability).
func TestJoinCSV_SingleAndMultiple(t *testing.T) {
	if got := joinCSV([]string{"GET"}); got != "GET" {
		t.Fatalf("joinCSV single = %q, want GET", got)
	}
	if got := joinCSV([]string{"GET", "POST", "PUT"}); got != "GET, POST, PUT" {
		t.Fatalf("joinCSV multiple = %q, want GET, POST, PUT", got)
	}
}
