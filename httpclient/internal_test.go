// This file is an internal test (package httpclient, not httpclient_test) so
// it can exercise the unexported helpers — shouldRetry, retryDelay and
// withDefaults — directly without exposing them in the public API.
package httpclient

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestShouldRetry_StatusCodes(t *testing.T) {
	cases := []struct {
		name  string
		code  int
		retry bool
	}{
		{"200 ok", 200, false},
		{"201 created", 201, false},
		{"204 no content", 204, false},
		{"301 redirect", 301, false},
		{"400 bad request", 400, false},
		{"401 unauthorized", 401, false},
		{"403 forbidden", 403, false},
		{"404 not found", 404, false},
		{"429 too many requests", 429, false},
		{"500 internal", 500, true},
		{"502 bad gateway", 502, true},
		{"503 unavailable", 503, true},
		{"504 timeout", 504, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tc.code}
			if got := shouldRetry(resp, nil); got != tc.retry {
				t.Fatalf("shouldRetry(%d) = %v, want %v", tc.code, got, tc.retry)
			}
		})
	}
}

func TestShouldRetry_Errors(t *testing.T) {
	if shouldRetry(nil, context.Canceled) {
		t.Fatal("context.Canceled must not be retryable")
	}
	// io.EOF / io.ErrUnexpectedEOF are retryable.
	if !shouldRetry(nil, io.EOF) {
		t.Fatal("io.EOF should be retryable")
	}
	if !shouldRetry(nil, io.ErrUnexpectedEOF) {
		t.Fatal("io.ErrUnexpectedEOF should be retryable")
	}
	// A timeout net.Error is retryable.
	timeoutErr := &timeoutError{}
	if !shouldRetry(nil, timeoutErr) {
		t.Fatal("timeout net.Error should be retryable")
	}
	// A connection refused is wrapped in net.OpError; retryable.
	opErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	if !shouldRetry(nil, opErr) {
		t.Fatal("net.OpError should be retryable")
	}
	// A bare non-network error is still treated as retryable by the fallback
	// (the assumption is that any transport-level error is transient).
	if !shouldRetry(nil, errors.New("broken pipe")) {
		t.Fatal("fallback should retry unknown errors")
	}
}

// timeoutError satisfies net.Error with Timeout() == true.
type timeoutError struct{}

func (*timeoutError) Error() string   { return "i/o timeout" }
func (*timeoutError) Timeout() bool   { return true }
func (*timeoutError) Temporary() bool { return true }

func TestRetryDelay_BoundedByMax(t *testing.T) {
	minW := 10 * time.Millisecond
	maxW := 100 * time.Millisecond
	for attempt := range 30 {
		d := retryDelay(attempt, minW, maxW)
		if d < 0 {
			t.Fatalf("attempt %d: delay %v < 0", attempt, d)
		}
		if d > maxW {
			t.Fatalf("attempt %d: delay %v > max %v", attempt, d, maxW)
		}
	}
}

func TestRetryDelay_GrowsExponentiallyBeforeCap(t *testing.T) {
	// With jitter factor in [0.5, 1.0), the first attempt (attempt=0) delay is
	// in [0.5*min, min). For min=1ms that's [0.5ms, 1ms). A later attempt whose
	// base exceeds that band proves exponential growth happened.
	minW := 100 * time.Millisecond
	maxW := 100 * time.Second
	// Take many samples of attempt 0 and attempt 10; the max-of-attempt-10
	// must exceed the max-of-attempt-0 with very high probability.
	var max0, max10 time.Duration
	for range 200 {
		if d := retryDelay(0, minW, maxW); d > max0 {
			max0 = d
		}
		if d := retryDelay(10, minW, maxW); d > max10 {
			max10 = d
		}
	}
	// attempt 0's cap is 0.5*1ms..1ms; attempt 10's base is min*2^10 = ~100s.
	if max10 <= max0 {
		t.Fatalf("expected exponential growth: max0=%v max10=%v", max0, max10)
	}
}

func TestRetryDelay_DisabledWhenZero(t *testing.T) {
	if d := retryDelay(3, 0, 0); d != 0 {
		t.Fatalf("disabled delay = %v, want 0", d)
	}
	if d := retryDelay(3, 10*time.Millisecond, 0); d != 0 {
		t.Fatalf("zero max delay = %v, want 0", d)
	}
	if d := retryDelay(3, 0, 10*time.Millisecond); d != 0 {
		t.Fatalf("zero min delay = %v, want 0", d)
	}
}

func TestRetryDelay_MinMaxEqual(t *testing.T) {
	// When min == max the exponential immediately clamps to max; the jitter
	// factor still varies the output in [0.5*max, max).
	w := 50 * time.Millisecond
	for i := range 50 {
		d := retryDelay(i, w, w)
		if d > w {
			t.Fatalf("attempt %d: delay %v > equal min/max %v", i, d, w)
		}
	}
}

func TestWithDefaults_AppliesDefaults(t *testing.T) {
	o := ClientOptions{}.withDefaults()
	d := defaultClientOptions()
	if o.ConnectTimeout != d.ConnectTimeout {
		t.Fatalf("ConnectTimeout = %v, want default %v", o.ConnectTimeout, d.ConnectTimeout)
	}
	if o.RequestTimeout != d.RequestTimeout {
		t.Fatalf("RequestTimeout = %v, want default %v", o.RequestTimeout, d.RequestTimeout)
	}
	if o.MaxIdleConns != d.MaxIdleConns {
		t.Fatalf("MaxIdleConns = %v, want default %v", o.MaxIdleConns, d.MaxIdleConns)
	}
	if o.MaxIdlePerHost != d.MaxIdlePerHost {
		t.Fatalf("MaxIdlePerHost = %v, want default %v", o.MaxIdlePerHost, d.MaxIdlePerHost)
	}
	if o.RetryMax != d.RetryMax {
		t.Fatalf("RetryMax = %v, want default %v", o.RetryMax, d.RetryMax)
	}
	if !o.FollowRedirect {
		t.Fatal("FollowRedirect should default to true on a zero ClientOptions")
	}
}

func TestWithDefaults_PreservesExplicitOverrides(t *testing.T) {
	in := ClientOptions{
		RequestTimeout: 7 * time.Second,
		RetryMax:       5,
	}.WithNoRedirect()
	o := in.withDefaults()
	if o.RequestTimeout != 7*time.Second {
		t.Fatalf("RequestTimeout = %v, want 7s (preserved)", o.RequestTimeout)
	}
	if o.RetryMax != 5 {
		t.Fatalf("RetryMax = %v, want 5 (preserved)", o.RetryMax)
	}
	// ConnectTimeout was zero, so defaulted.
	if o.ConnectTimeout != defaultClientOptions().ConnectTimeout {
		t.Fatalf("ConnectTimeout = %v, want default", o.ConnectTimeout)
	}
	if o.FollowRedirect {
		t.Fatal("FollowRedirect should be false (explicitly disabled via WithNoRedirect)")
	}
}

func TestWithRedirect_Helpers(t *testing.T) {
	on := ClientOptions{}.WithRedirect()
	if !on.FollowRedirect || !on.FollowRedirectSet {
		t.Fatalf("WithRedirect: %+v", on)
	}
	off := ClientOptions{}.WithNoRedirect()
	if off.FollowRedirect || !off.FollowRedirectSet {
		t.Fatalf("WithNoRedirect: %+v", off)
	}
	// Package-level helpers mirror the methods.
	if !WithRedirect(ClientOptions{}).FollowRedirect {
		t.Fatal("package WithRedirect did not enable")
	}
	if WithNoRedirect(ClientOptions{}).FollowRedirect {
		t.Fatal("package WithNoRedirect did not disable")
	}
}
