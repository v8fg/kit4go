package errcode

import (
	"errors"
	"fmt"
	"testing"
)

// sentinel is a plain error used to verify errors.Is reaches wrapped causes.
var sentinel = errors.New("sentinel root cause")

// TestCodeString exercises every declared code plus both out-of-range arms so
// the codeNames table is fully covered.
func TestCodeString(t *testing.T) {
	tests := []struct {
		code Code
		want string
	}{
		{OK, "ok"},
		{Canceled, "canceled"},
		{Unknown, "unknown"},
		{InvalidArgument, "invalid_argument"},
		{DeadlineExceeded, "deadline_exceeded"},
		{NotFound, "not_found"},
		{AlreadyExists, "already_exists"},
		{PermissionDenied, "permission_denied"},
		{ResourceExhausted, "resource_exhausted"},
		{FailedPrecondition, "failed_precondition"},
		{Aborted, "aborted"},
		{OutOfRange, "out_of_range"},
		{Unimplemented, "unimplemented"},
		{Internal, "internal"},
		{Unavailable, "unavailable"},
		{DataLoss, "data_loss"},
		{Unauthenticated, "unauthenticated"},
		// Out-of-range codes must not panic; both directions fall back to "unknown".
		{Code(-1), "unknown"},
		{Code(len(codeNames)), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.code.String(); got != tt.want {
			t.Errorf("Code(%d).String() = %q, want %q", tt.code, got, tt.want)
		}
	}
}

// TestErrorNew covers New, the non-nil Error() formatting, and a nil Unwrap.
func TestErrorNew(t *testing.T) {
	e := New(NotFound, "user missing")
	if e.Code != NotFound {
		t.Fatalf("Code = %v, want NotFound", e.Code)
	}
	if e.Message != "user missing" {
		t.Fatalf("Message = %q, want %q", e.Message, "user missing")
	}
	if got := e.Unwrap(); got != nil {
		t.Fatalf("Unwrap() = %v, want nil", got)
	}
	want := "errcode: not_found: user missing"
	if got := e.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

// TestErrorWrap exercises Wrap, the cause-bearing Error() formatting, and the
// Unwrap chain reaching the wrapped sentinel.
func TestErrorWrap(t *testing.T) {
	e := Wrap(Internal, sentinel, "db down")
	if e.Code != Internal {
		t.Fatalf("Code = %v, want Internal", e.Code)
	}
	if got := e.Unwrap(); got != sentinel {
		t.Fatalf("Unwrap() = %v, want sentinel", got)
	}
	want := "errcode: internal: db down: sentinel root cause"
	if got := e.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(e, sentinel) {
		t.Fatal("errors.Is(e, sentinel) = false, want true")
	}
}

// TestWrapNilCause verifies Wrap with a nil cause behaves like New.
func TestWrapNilCause(t *testing.T) {
	e := Wrap(InvalidArgument, nil, "bad input")
	if got := e.Unwrap(); got != nil {
		t.Fatalf("Unwrap() = %v, want nil for nil cause", got)
	}
	want := "errcode: invalid_argument: bad input"
	if got := e.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

// TestWithDetail covers the builder chain: details append in order and the
// Error's Code is left untouched.
func TestWithDetail(t *testing.T) {
	d1 := struct{ K string }{K: "request-id"}
	d2 := 42
	e := New(DeadlineExceeded, "slow").WithDetail(d1).WithDetail(d2)
	if len(e.Details) != 2 {
		t.Fatalf("len(Details) = %d, want 2", len(e.Details))
	}
	if e.Details[0] != d1 || e.Details[1] != d2 {
		t.Fatalf("Details = %#v, want [%#v %#v]", e.Details, d1, d2)
	}
	if e.Code != DeadlineExceeded {
		t.Fatalf("Code changed after WithDetail: %v", e.Code)
	}
}

// TestIsSameCode covers the same-code equality rule across distinct instances
// and message texts, plus the negative case (different code) and non-*Error
// target (delegates to identity).
func TestIsSameCode(t *testing.T) {
	a := New(NotFound, "user 1 missing")
	b := New(NotFound, "user 2 missing") // same code, different message
	if !errors.Is(a, b) {
		t.Fatal("errors.Is(a, b) = false for same code, want true")
	}

	other := New(Internal, "boom") // different code
	if errors.Is(a, other) {
		t.Fatal("errors.Is(a, other) = true for different codes, want false")
	}

	// Non-*Error target: standard identity must hold.
	plain := errors.New("plain")
	if errors.Is(a, plain) {
		t.Fatal("errors.Is(a, plain) = true, want false")
	}
	if !errors.Is(plain, plain) {
		t.Fatal("errors.Is(plain, plain) = false, want true (identity)")
	}

	// Same-code equality must also traverse a Wrap chain.
	wrapped := Wrap(PermissionDenied, sentinel, "no access")
	target := New(PermissionDenied, "")
	if !errors.Is(wrapped, target) {
		t.Fatal("errors.Is(wrapped, target) = false, want true (code match through chain)")
	}
}

// TestErrorsAs covers errors.As reaching an *Error nested under fmt.Errorf %w.
func TestErrorsAs(t *testing.T) {
	e := New(DataLoss, "lost row")
	outer := fmt.Errorf("handler: %w", e)
	var got *Error
	if !errors.As(outer, &got) {
		t.Fatal("errors.As failed to find *Error in chain")
	}
	if got.Code != DataLoss {
		t.Fatalf("retrieved Code = %v, want DataLoss", got.Code)
	}
	if got != e {
		t.Fatal("retrieved *Error is not the original instance")
	}
}

// TestCodeOf covers every branch: nil -> OK, plain error -> Unknown, an *Error
// -> its Code, and an *Error reachable through a chain -> the wrapped Code.
func TestCodeOf(t *testing.T) {
	if got := CodeOf(nil); got != OK {
		t.Fatalf("CodeOf(nil) = %v, want OK", got)
	}

	plain := errors.New("untyped")
	if got := CodeOf(plain); got != Unknown {
		t.Fatalf("CodeOf(plain) = %v, want Unknown", got)
	}

	if got := CodeOf(New(AlreadyExists, "dup")); got != AlreadyExists {
		t.Fatalf("CodeOf(*Error) = %v, want AlreadyExists", got)
	}

	wrapped := fmt.Errorf("outer: %w", Wrap(ResourceExhausted, nil, "quota"))
	if got := CodeOf(wrapped); got != ResourceExhausted {
		t.Fatalf("CodeOf(wrapped) = %v, want ResourceExhausted", got)
	}
}

// TestNilErrorMethodReceivers guards the nil-receiver paths of Error and Is so
// a nil *Error never panics when used as an error.
func TestNilErrorMethodReceivers(t *testing.T) {
	var nilErr *Error

	if got := nilErr.Error(); got != "errcode: ok" {
		t.Fatalf("nil Error() = %q, want %q", got, "errcode: ok")
	}
	if got := nilErr.Unwrap(); got != nil {
		t.Fatalf("nil Unwrap() = %v, want nil", got)
	}
	if !nilErr.Is(nil) {
		t.Fatal("nil.Is(nil) = false, want true")
	}
	if nilErr.Is(New(NotFound, "")) {
		t.Fatal("nil.Is(nonNil) = true, want false")
	}
}
