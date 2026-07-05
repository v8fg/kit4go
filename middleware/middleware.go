// Package middleware provides composable HTTP middleware for net/http servers
// (compatible with httpserver and any standard http.Handler chain). Each
// middleware is a func(http.Handler) http.Handler — the standard Go pattern.
//
// Pure standard library. Ad-tech uses: rate-limit SSP endpoints, propagate
// request IDs for cross-service tracing, CORS for browser-side ad tags.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"
)

// --- Request ID ---

// ContextKey is the context key for the request ID.
type ContextKey struct{}

// HeaderRequestID is the standard request-ID header.
const HeaderRequestID = "X-Request-ID"

// RequestID generates a request ID (if absent) and propagates it via the
// X-Request-ID header and context. If the incoming request already carries the
// header, it is preserved (allows cross-service propagation).
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if id == "" {
			id = generateID()
			r.Header.Set(HeaderRequestID, id)
		}
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ContextKey{}, id)))
	})
}

// FromContext extracts the request ID from a context (set by RequestID middleware).
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ContextKey{}).(string)
	return id
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// --- Rate Limit ---

// AllowFunc is any function that reports whether a request is allowed (e.g.,
// limiter.Limiter.Allow, rate.Limiter.Allow). It is called once per request.
type AllowFunc func() bool

// RateLimit returns middleware that checks allow before each request. If allow
// returns false, the response is 429 Too Many Requests. Optionally set a custom
// Retry-After header via retryAfter (0 = omit).
func RateLimit(allow AllowFunc, retryAfter int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !allow() {
				if retryAfter > 0 {
					w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				}
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- CORS ---

// CORSConfig holds CORS policy. Zero value = permissive (all origins, standard
// methods/headers). Tighten with the fields below.
type CORSConfig struct {
	AllowOrigins     []string      // nil = "*" (all origins)
	AllowMethods     []string      // nil = GET,POST,PUT,DELETE,OPTIONS
	AllowHeaders     []string      // nil = Content-Type,Authorization
	AllowCredentials bool          // true sends Access-Control-Allow-Credentials
	MaxAge           time.Duration // preflight cache; 0 = no header
}

// CORS returns middleware that applies CORS headers. Preflight (OPTIONS) requests
// are answered directly; all other methods pass through with CORS headers added.
//
// NOTE: "*" + AllowCredentials is invalid per the CORS spec (browsers reject it).
// If AllowCredentials is true and AllowOrigins contains "*", the middleware
// echoes the request's Origin header instead of "*" (the spec-compliant fallback).
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	origins := cfg.AllowOrigins
	if origins == nil {
		origins = []string{"*"}
	}
	creds := cfg.AllowCredentials
	methods := cfg.AllowMethods
	if methods == nil {
		methods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions}
	}
	headers := cfg.AllowHeaders
	if headers == nil {
		headers = []string{"Content-Type", "Authorization"}
	}
	methodStr := joinCSV(methods)
	headerStr := joinCSV(headers)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowOrigin := matchOrigin(origin, origins)
			// Spec compliance: "*" + credentials is invalid. Echo the specific
			// origin instead so browsers accept it with credentials.
			if creds && allowOrigin == "*" && origin != "" {
				allowOrigin = origin
			}
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			w.Header().Set("Access-Control-Allow-Methods", methodStr)
			w.Header().Set("Access-Control-Allow-Headers", headerStr)
			if creds {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if cfg.MaxAge > 0 {
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(int(cfg.MaxAge.Seconds())))
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// matchOrigin returns the origin if it's in the allowlist, or "*" if the list
// contains "*". For a single-origin list, returns that origin directly (the
// common case).
func matchOrigin(requestOrigin string, allowed []string) string {
	for _, a := range allowed {
		if a == "*" {
			return "*"
		}
		if a == requestOrigin {
			return requestOrigin
		}
	}
	return "" // origin not allowed — browser will block
}

func joinCSV(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for i := 1; i < len(ss); i++ {
		result += ", " + ss[i]
	}
	return result
}
