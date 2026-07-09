package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

// --- CORS Vary: Origin regression (R16) ---

// TestCORS_VaryOrigin_AllowlistReflected confirms that when the origin is
// allowlist-matched (ACAO reflects the request origin rather than the literal
// "*"), the response carries "Vary: Origin" so a shared cache cannot serve a
// wrong-origin response.
func TestCORS_VaryOrigin_AllowlistReflected(t *testing.T) {
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

	if got := rec.Header().Get("Vary"); !strings.EqualFold(got, "Origin") {
		t.Fatalf("Vary = %q, want \"Origin\" when origin is reflected", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://ads.example.com" {
		t.Fatalf("Allow-Origin = %q, want https://ads.example.com", got)
	}
}

// TestCORS_VaryOrigin_CredentialsWildcardEcho confirms the credentials+wildcard
// echo fallback (ACAO becomes the reflected origin) also emits Vary: Origin.
func TestCORS_VaryOrigin_CredentialsWildcardEcho(t *testing.T) {
	h := CORS(CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Allow-Origin = %q, want reflected origin", got)
	}
	if got := rec.Header().Get("Vary"); !strings.EqualFold(got, "Origin") {
		t.Fatalf("Vary = %q, want \"Origin\" on credentials echo", got)
	}
}

// TestCORS_VaryOrigin_AbsentForLiteralWildcard confirms Vary: Origin is NOT
// emitted when ACAO is the literal "*" (origin-independent response → cacheable).
func TestCORS_VaryOrigin_AbsentForLiteralWildcard(t *testing.T) {
	h := CORS(CORSConfig{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://anywhere.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q, want *", got)
	}
	if rec.Header().Get("Vary") != "" {
		t.Fatalf("Vary = %q, want empty for origin-independent wildcard response", rec.Header().Get("Vary"))
	}
}

// --- RequestID validation regression (R16) ---

// TestRequestID_RejectsOversized confirms that a client X-Request-ID exceeding
// MaxRequestIDLen is dropped and a fresh ID is generated (and used in both
// context and response header), rather than echoing a megabyte-scale value.
func TestRequestID_RejectsOversized(t *testing.T) {
	huge := strings.Repeat("a", MaxRequestIDLen+1)
	var captured string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderRequestID, huge)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	respID := rec.Header().Get(HeaderRequestID)
	if respID == huge {
		t.Fatal("oversized client request ID must not be echoed in the response")
	}
	if len(respID) != 32 { // generateID = hex(16 bytes)
		t.Fatalf("response ID len = %d, want 32 (freshly generated)", len(respID))
	}
	if captured != respID {
		t.Fatalf("context ID %q != response ID %q (should be the fresh ID)", captured, respID)
	}
	if req.Header.Get(HeaderRequestID) != respID {
		t.Fatalf("request header ID %q != fresh ID %q", req.Header.Get(HeaderRequestID), respID)
	}
}

// TestRequestID_RejectsBadCharset confirms an ID within length bounds but with
// invalid characters is rejected and regenerated.
func TestRequestID_RejectsBadCharset(t *testing.T) {
	const bad = "trace;rm -rf" // contains spaces and semicolons
	var captured string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderRequestID, bad)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if captured == bad {
		t.Fatalf("bad-charset client ID must be dropped, got context ID %q", captured)
	}
	if rec.Header().Get(HeaderRequestID) == bad {
		t.Fatal("bad-charset client ID must not be echoed in the response")
	}
}

// TestRequestID_PreservesValidCharset confirms valid-format IDs at the length
// bound are preserved (covers all allowed chars: [A-Za-z0-9._-]).
func TestRequestID_PreservesValidCharset(t *testing.T) {
	const valid = "ABCdef012._-XYZabc789" // all charset classes, <= MaxRequestIDLen
	var captured string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderRequestID, valid)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if captured != valid {
		t.Fatalf("valid client ID not preserved: context = %q, want %q", captured, valid)
	}
	if rec.Header().Get(HeaderRequestID) != valid {
		t.Fatalf("valid client ID not echoed: response = %q, want %q", rec.Header().Get(HeaderRequestID), valid)
	}
}

// TestValidRequestID_Boundaries pins the edge cases of the validator: empty,
// exactly MaxRequestIDLen (accepted), one byte over (rejected).
func TestValidRequestID_Boundaries(t *testing.T) {
	if validRequestID("") {
		t.Fatal("empty ID should be invalid")
	}
	atMax := strings.Repeat("a", MaxRequestIDLen)
	if !validRequestID(atMax) {
		t.Fatal("ID at MaxRequestIDLen should be valid")
	}
	overMax := strings.Repeat("a", MaxRequestIDLen+1)
	if validRequestID(overMax) {
		t.Fatal("ID over MaxRequestIDLen should be invalid")
	}
}

// --- RateLimit nil guard regression (R16) ---

// TestRateLimit_NilAllowFuncPanics confirms RateLimit panics at construction
// (not at request time) with a clear message when allow is nil, matching the
// Must-style convention of other kit constructors.
func TestRateLimit_NilAllowFuncPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("RateLimit(nil, ...) should panic at construction")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "non-nil AllowFunc") {
			t.Fatalf("panic = %v, want a message mentioning non-nil AllowFunc", r)
		}
	}()
	_ = RateLimit(nil, 0)
}
